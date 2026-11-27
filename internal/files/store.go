package files

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultBaseDir = ".skillvault"

type ArtifactFileService struct {
	basePath string
}

func NewArtifactFileService(basePath string) (*ArtifactFileService, error) {
	if basePath == "" {
		basePath = os.Getenv("SKILLVAULT_HOME")
	}
	if basePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		basePath = filepath.Join(home, defaultBaseDir)
	}
	return &ArtifactFileService{basePath: basePath}, nil
}

func extFromMIME(mimeType string) string {
	switch mimeType {
	case "text/markdown":
		return ".md"
	case "application/json":
		return ".json"
	case "text/plain":
		return ".txt"
	case "text/html":
		return ".html"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}

func DetectMIME(content []byte) string {
	if len(content) == 0 {
		return ""
	}
	if len(content) >= 4 && content[0] == '%' && content[1] == 'P' && content[2] == 'D' && content[3] == 'F' {
		return "application/pdf"
	}
	if len(content) >= 6 && content[0] == '<' && content[1] == 'h' && content[2] == 't' && content[3] == 'm' {
		return "text/html"
	}
	if content[0] == '{' || content[0] == '[' {
		return "application/json"
	}
	if content[0] == '#' {
		return "text/markdown"
	}
	return "text/plain"
}

func (s *ArtifactFileService) WriteArtifact(slug string, content []byte, mimeType string) (string, string, int64, error) {
	now := time.Now()
	year := now.Format("2006")
	month := now.Format("01")

	if mimeType == "" {
		mimeType = DetectMIME(content)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	ext := extFromMIME(mimeType)
	relPath := filepath.Join("objects", year, month, slug+ext)
	if slug == "" || !filepath.IsLocal(slug) || strings.ContainsAny(slug, `/\`) {
		return "", "", 0, fmt.Errorf("invalid artifact file name")
	}
	root, objectPath, err := s.openObjectPath(relPath)
	if err != nil {
		return "", "", 0, err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(objectPath), 0700); err != nil {
		return "", "", 0, fmt.Errorf("create artifact directory: %w", err)
	}
	file, err := root.OpenFile(objectPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", "", 0, fmt.Errorf("create artifact: %w", err)
	}
	_, writeErr := file.Write(content)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = root.Remove(objectPath)
		return "", "", 0, fmt.Errorf("write artifact: %w", errors.Join(writeErr, closeErr))
	}
	return filepath.ToSlash(relPath), HashContent(content), int64(len(content)), nil
}

func (s *ArtifactFileService) ReadArtifact(filePath string) ([]byte, error) {
	root, name, err := s.openObjectPath(filePath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(name)
}

func (s *ArtifactFileService) DeleteArtifact(filePath string) error {
	root, name, err := s.openObjectPath(filePath)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(name)
}

func (s *ArtifactFileService) openObjectPath(path string) (*os.Root, string, error) {
	if filepath.IsAbs(path) {
		relative, err := filepath.Rel(s.basePath, path)
		if err != nil {
			return nil, "", err
		}
		path = relative
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	name, ok := strings.CutPrefix(clean, "objects/")
	if !ok || !filepath.IsLocal(filepath.FromSlash(name)) || strings.Contains(name, `\`) {
		return nil, "", fmt.Errorf("artifact path must be inside objects")
	}
	vault, err := os.OpenRoot(s.basePath)
	if err != nil {
		return nil, "", err
	}
	defer vault.Close()
	if err := vault.MkdirAll("objects", 0700); err != nil {
		return nil, "", err
	}
	info, err := vault.Lstat("objects")
	if err != nil {
		return nil, "", err
	}
	if !info.IsDir() {
		return nil, "", fmt.Errorf("objects must be a directory inside the vault")
	}
	objects, err := vault.OpenRoot("objects")
	return objects, filepath.FromSlash(name), err
}

func HashContent(content []byte) string {
	h := sha256.Sum256(content)
	return fmt.Sprintf("%x", h)
}
