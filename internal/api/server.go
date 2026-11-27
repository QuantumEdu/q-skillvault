package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/quantum-6/skillvault/internal/app"
	"github.com/quantum-6/skillvault/internal/domain"
	"github.com/quantum-6/skillvault/internal/security"
)

// Server is the HTTP API server.
type Server struct {
	entrySvc    *app.EntryService
	artifactSvc *app.ArtifactService
	contextSvc  *app.ContextService
	projectSvc  *app.ProjectService
	sessionSvc  *app.SessionService
	workflowSvc *app.WorkflowService
	exportSvc   *app.VaultExportService
	importSvc   *app.VaultImportService
	host        string
	port        int
	apiKey      string
	srv         *http.Server
}

// NewServer creates a new HTTP API server.
func NewServer(
	host string, port int,
	entrySvc *app.EntryService,
	artifactSvc *app.ArtifactService,
	contextSvc *app.ContextService,
	projectSvc *app.ProjectService,
	sessionSvc *app.SessionService,
	workflowSvc *app.WorkflowService,
	exportSvc *app.VaultExportService,
	importSvc *app.VaultImportService,
) *Server {
	return &Server{
		entrySvc:    entrySvc,
		artifactSvc: artifactSvc,
		contextSvc:  contextSvc,
		projectSvc:  projectSvc,
		sessionSvc:  sessionSvc,
		workflowSvc: workflowSvc,
		exportSvc:   exportSvc,
		importSvc:   importSvc,
		host:        host,
		port:        port,
	}
}

// WithAPIKey sets the API key for bearer token authentication.
// All vault endpoints require Authorization: Bearer <key>.
func (s *Server) WithAPIKey(key string) *Server {
	s.apiKey = key
	return s
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	expected := sha256.Sum256([]byte(s.apiKey))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		actual := sha256.Sum256([]byte(token))
		if s.apiKey == "" || !bearer || subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "invalid or missing API key")
			return
		}
		if r.Method == http.MethodPost {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "application/json required")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		}
		next.ServeHTTP(w, r)
	})
}

// Start begins listening. Returns error if unable to start.
func (s *Server) Start() error {
	if strings.TrimSpace(s.apiKey) == "" {
		return fmt.Errorf("HTTP API key required")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/entries", s.handleEntries)
	mux.HandleFunc("/entries/", s.handleEntryByID)
	mux.HandleFunc("/artifacts", s.handleArtifacts)
	mux.HandleFunc("/context", s.handleContext)
	mux.HandleFunc("/projects", s.handleProjects)
	mux.HandleFunc("/sessions/wrap", s.handleSessionWrap)
	mux.HandleFunc("/workflows", s.handleWorkflows)
	mux.HandleFunc("/workflows/", s.handleWorkflowByID)
	mux.HandleFunc("/export", s.handleExport)
	mux.HandleFunc("/import", s.handleImport)

	s.srv = &http.Server{
		Addr:              net.JoinHostPort(s.host, strconv.Itoa(s.port)),
		Handler:           security.LoopbackHTTP(s.authMiddleware(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully shuts down the server, draining active connections.
func (s *Server) Stop() error {
	if s.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.srv.Shutdown(ctx)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// --- Handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleEntries(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	switch r.Method {
	case http.MethodPost:
		var input app.SaveEntryInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		result, err := s.entrySvc.SaveEntry(ctx, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, result)

	case http.MethodGet:
		q := r.URL.Query()
		query := q.Get("q")
		entryType := q.Get("type")
		project := q.Get("project")
		limitStr := q.Get("limit")

		limit := 50
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				limit = n
			}
		}

		var typePtr *string
		if entryType != "" {
			typePtr = &entryType
		}
		var projectPtr *string
		if project != "" {
			projectPtr = &project
		}

		results, err := s.entrySvc.SearchEntries(ctx, query, domain.SearchQuery{
			ProjectID:       projectPtr,
			Type:            typePtr,
			IncludeArchived: true,
			Limit:           limit,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, results)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleEntryByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/entries/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "entry id required")
		return
	}

	ctx := context.Background()

	switch r.Method {
	case http.MethodGet:
		result, err := s.entrySvc.GetEntry(ctx, id)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)

	case http.MethodDelete:
		if err := s.entrySvc.ArchiveEntry(ctx, id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "archived", "id": id})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := context.Background()

	var input app.SaveArtifactInput
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	artifact, err := s.artifactSvc.SaveArtifact(ctx, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, artifact)
}

func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := context.Background()

	var input struct {
		Mode     string `json:"mode"`
		Project  string `json:"project"`
		MaxChars int    `json:"max_chars"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	pack, err := s.contextSvc.GetContext(ctx, app.ContextInput{
		Mode:     input.Mode,
		Project:  input.Project,
		MaxChars: input.MaxChars,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, pack)
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	switch r.Method {
	case http.MethodPost:
		var input app.SaveProjectInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		proj, err := s.projectSvc.SaveProject(ctx, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, proj)

	case http.MethodGet:
		projects, err := s.projectSvc.ListProjects(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, projects)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSessionWrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := context.Background()

	var input app.SessionWrapInput
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	output, err := s.sessionSvc.SessionWrap(ctx, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, output)
}

func (s *Server) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := context.Background()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	var input app.SaveWorkflowInput
	if err := json.Unmarshal(body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	wf, err := s.workflowSvc.SaveWorkflow(ctx, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, wf)
}

func (s *Server) handleWorkflowByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/workflows/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "workflow id required")
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := context.Background()

	steps, err := s.workflowSvc.RenderWorkflow(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, steps)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := context.Background()

	var input struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if input.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	if err := s.exportSvc.Export(ctx, input.Path); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "exported", "path": input.Path})
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := context.Background()

	var input struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if input.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	if err := s.importSvc.Import(ctx, input.Path); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "imported"})
}
