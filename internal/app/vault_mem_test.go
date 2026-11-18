package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/quantum-6/skillvault/internal/domain"
)

func setupVaultMemTest(t *testing.T) (*EntryService, *VaultMemService, func()) {
	t.Helper()
	store, entrySvc, _, _, _, _, contextSvc, _, _, _, cleanup := setupAppServices(t)
	memSvc := NewVaultMemService(store.Entries, store.Projects, entrySvc, contextSvc)
	return entrySvc, memSvc, cleanup
}

func TestVaultMemCalculateNormalizedHash(t *testing.T) {
	h1 := CalculateNormalizedHash("Use Tailwind v4 in frontend")
	h2 := CalculateNormalizedHash("  use tailwind v4 in frontend \n ")
	if h1 != h2 {
		t.Fatalf("expected identical hash for normalized content, got %s != %s", h1, h2)
	}

	if len(h1) != 64 {
		t.Fatalf("expected 64-char sha256 hex string, got len=%d", len(h1))
	}

	h3 := CalculateNormalizedHash("different content")
	if h1 == h3 {
		t.Fatalf("expected different hash for distinct content")
	}

	// Alias check
	if NormalizedMemoryHash("test") != CalculateNormalizedHash("test") {
		t.Fatalf("NormalizedMemoryHash should match CalculateNormalizedHash")
	}
}

func TestCalculateNormalizedHash(t *testing.T) {
	TestVaultMemCalculateNormalizedHash(t)
}

func TestVaultMemMapObservationType(t *testing.T) {
	cases := []struct {
		input       string
		expectedTyp domain.EntryType
		expectedTag string
	}{
		{"decision", domain.EntryTypeDecision, "decision"},
		{"architecture", domain.EntryTypeSkill, "architecture"},
		{"workflow", domain.EntryTypeWorkflowNote, "workflow"},
		{"workflow_note", domain.EntryTypeWorkflowNote, "workflow"},
		{"skill", domain.EntryTypeSkill, "skill"},
		{"bugfix", domain.EntryTypeReference, "bugfix"},
		{"discovery", domain.EntryTypeReference, "discovery"},
		{"pattern", domain.EntryTypeReference, "pattern"},
		{"config", domain.EntryTypeReference, "config"},
		{"preference", domain.EntryTypeFeedback, "feedback"},
		{"feedback", domain.EntryTypeFeedback, "feedback"},
		{"prompt", domain.EntryTypePrompt, "prompt"},
		{"custom_unknown", domain.EntryTypeReference, "custom_unknown"},
	}

	for _, tc := range cases {
		typ, tag := MapObservationType(tc.input)
		if typ != tc.expectedTyp {
			t.Errorf("input %q: expected type %s, got %s", tc.input, tc.expectedTyp, typ)
		}
		if tag != tc.expectedTag {
			t.Errorf("input %q: expected tag %s, got %s", tc.input, tc.expectedTag, tag)
		}
	}
}

func TestMapObservationType(t *testing.T) {
	TestVaultMemMapObservationType(t)
}

func TestVaultMemSaveDeduplication(t *testing.T) {
	entrySvc, _, cleanup := setupVaultMemTest(t)
	defer cleanup()

	ctx := context.Background()

	input := VaultMemSaveInput{
		Title:    "Architecture decision: use Tauri 2 sidecar",
		Content:  "We supervise the Go server as a local sidecar on localhost:8080.",
		Type:     "architecture",
		Project:  "q-bot",
		TopicKey: "odd/tasks/desktop-companion",
		Scope:    "project",
		Tags:     []string{"rust", "tauri"},
	}

	res1, dedup1, err := entrySvc.SaveVaultMem(ctx, input)
	if err != nil {
		t.Fatalf("initial save failed: %v", err)
	}
	if dedup1 {
		t.Errorf("expected initial save to NOT be deduplicated")
	}
	if res1.Entry.ID == "" {
		t.Fatalf("expected non-empty entry ID")
	}
	if res1.Entry.Type != domain.EntryTypeSkill {
		t.Errorf("expected architecture to map to %s, got %s", domain.EntryTypeSkill, res1.Entry.Type)
	}
	expectedHash := "hash:" + CalculateNormalizedHash(input.Content)
	if res1.Entry.ExternalRef != expectedHash {
		t.Errorf("expected ExternalRef %s, got %s", expectedHash, res1.Entry.ExternalRef)
	}

	// Verify tags
	tagNames := make(map[string]bool)
	for _, tag := range res1.Tags {
		tagNames[tag.Name] = true
	}
	for _, expectedTag := range []string{"mem", "architecture", "scope:project", "rust", "tauri"} {
		if !tagNames[expectedTag] {
			t.Errorf("expected tag %q in %v", expectedTag, tagNames)
		}
	}

	// Save identical content in the same project -> MUST be deduplicated
	res2, dedup2, err := entrySvc.SaveVaultMem(ctx, input)
	if err != nil {
		t.Fatalf("second save failed: %v", err)
	}
	if !dedup2 {
		t.Errorf("expected second save to be deduplicated")
	}
	if res2.Entry.ID != res1.Entry.ID {
		t.Errorf("expected same ID on deduplication, got %s vs %s", res2.Entry.ID, res1.Entry.ID)
	}

	// Save identical content in a DIFFERENT project -> NOT deduplicated
	diffProjInput := input
	diffProjInput.Project = "q-other-proj"
	res3, dedup3, err := entrySvc.SaveVaultMem(ctx, diffProjInput)
	if err != nil {
		t.Fatalf("different project save failed: %v", err)
	}
	if dedup3 {
		t.Errorf("expected save in different project to NOT be deduplicated")
	}
	if res3.Entry.ID == res1.Entry.ID {
		t.Errorf("expected different ID for different project")
	}

	// Save with auto-generated title (empty title)
	noTitleInput := VaultMemSaveInput{
		Content: "First line of content\nSecond line with details",
		Type:    "discovery",
		Project: "q-bot",
	}
	res4, _, err := entrySvc.SaveVaultMem(ctx, noTitleInput)
	if err != nil {
		t.Fatalf("auto-title save failed: %v", err)
	}
	if res4.Entry.Title != "First line of content" {
		t.Errorf("expected auto-generated title 'First line of content', got %q", res4.Entry.Title)
	}
}

func TestSaveVaultMemDeduplication(t *testing.T) {
	TestVaultMemSaveDeduplication(t)
}

func TestVaultMemSearch(t *testing.T) {
	entrySvc, _, cleanup := setupVaultMemTest(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Entry in q-bot with type decision
	_, _, err := entrySvc.SaveVaultMem(ctx, VaultMemSaveInput{
		Title:    "SQLite WAL mode",
		Content:  "Enabled modernc pragmas busy_timeout=5000 and journal_mode=WAL for SQLite concurrency.",
		Type:     "decision",
		Project:  "q-bot",
		TopicKey: "storage/sqlite",
	})
	if err != nil {
		t.Fatalf("save 1 failed: %v", err)
	}

	// 2. Entry in q-bot with type bugfix
	_, _, err = entrySvc.SaveVaultMem(ctx, VaultMemSaveInput{
		Title:    "Fix connection lock",
		Content:  "Fixed deadlock when transactions were opened in read-only mode during migration.",
		Type:     "bugfix",
		Project:  "q-bot",
		TopicKey: "storage/sqlite",
	})
	if err != nil {
		t.Fatalf("save 2 failed: %v", err)
	}

	// 3. Entry in q-bot with type workflow
	_, _, err = entrySvc.SaveVaultMem(ctx, VaultMemSaveInput{
		Title:    "Release checklist",
		Content:  "Tag git release, build multi-arch binaries, upload assets to GitHub release.",
		Type:     "workflow",
		Project:  "q-bot",
		TopicKey: "release/pipeline",
	})
	if err != nil {
		t.Fatalf("save 3 failed: %v", err)
	}

	// 4. Entry in another project
	_, _, err = entrySvc.SaveVaultMem(ctx, VaultMemSaveInput{
		Title:    "Postgres pooling",
		Content:  "Configured pgxpool with max 20 connections for PostgreSQL.",
		Type:     "decision",
		Project:  "web-service",
		TopicKey: "storage/postgres",
	})
	if err != nil {
		t.Fatalf("save 4 failed: %v", err)
	}

	// 5. Entry with content longer than 300 chars to test preview truncation
	longContent := strings.Repeat("Extremely important architectural note about system constraints. ", 10)
	_, _, err = entrySvc.SaveVaultMem(ctx, VaultMemSaveInput{
		Title:    "Long note",
		Content:  longContent,
		Type:     "discovery",
		Project:  "q-bot",
		TopicKey: "storage/sqlite",
	})
	if err != nil {
		t.Fatalf("save 5 failed: %v", err)
	}

	// Test 1: Project filter
	resProj, err := entrySvc.SearchVaultMem(ctx, VaultMemSearchInput{
		Project: "q-bot",
	})
	if err != nil {
		t.Fatalf("search by project failed: %v", err)
	}
	if len(resProj) != 4 {
		t.Errorf("expected 4 results in q-bot, got %d", len(resProj))
	}

	// Test 2: Topic filter
	resTopic, err := entrySvc.SearchVaultMem(ctx, VaultMemSearchInput{
		Project:  "q-bot",
		TopicKey: "storage/sqlite",
	})
	if err != nil {
		t.Fatalf("search by topic failed: %v", err)
	}
	if len(resTopic) != 3 {
		t.Errorf("expected 3 results for topic storage/sqlite, got %d", len(resTopic))
	}

	// Test 3: Type filter (workflow -> workflow_note)
	resType, err := entrySvc.SearchVaultMem(ctx, VaultMemSearchInput{
		Project: "q-bot",
		Type:    "workflow",
	})
	if err != nil {
		t.Fatalf("search by type failed: %v", err)
	}
	if len(resType) != 1 {
		t.Fatalf("expected 1 result for type workflow, got %d", len(resType))
	}
	if resType[0].Title != "Release checklist" {
		t.Errorf("unexpected item: %s", resType[0].Title)
	}

	// Test 4: Query search
	resQuery, err := entrySvc.SearchVaultMem(ctx, VaultMemSearchInput{
		Project: "q-bot",
		Query:   "deadlock",
	})
	if err != nil {
		t.Fatalf("search by query failed: %v", err)
	}
	if len(resQuery) == 0 {
		t.Fatalf("expected at least 1 match for query 'deadlock'")
	}
	if resQuery[0].Title != "Fix connection lock" {
		t.Errorf("expected 'Fix connection lock', got %s", resQuery[0].Title)
	}

	// Test 5: Preview truncation <= 300 chars
	resLong, err := entrySvc.SearchVaultMem(ctx, VaultMemSearchInput{
		Project: "q-bot",
		Query:   "architectural",
	})
	if err != nil {
		t.Fatalf("search long preview failed: %v", err)
	}
	if len(resLong) == 0 {
		t.Fatalf("expected match for 'architectural'")
	}
	if len(resLong[0].Preview) > 300 {
		t.Errorf("preview exceeded 300 chars: len=%d", len(resLong[0].Preview))
	}
}

func TestSearchVaultMem(t *testing.T) {
	TestVaultMemSearch(t)
}

func TestVaultMemTimeline(t *testing.T) {
	entrySvc, _, cleanup := setupVaultMemTest(t)
	defer cleanup()

	ctx := context.Background()

	r1, _, err := entrySvc.SaveVaultMem(ctx, VaultMemSaveInput{
		Title:   "Step 1: Inception",
		Content: "Initial observation",
		Project: "proj-timeline",
	})
	if err != nil {
		t.Fatalf("save 1 failed: %v", err)
	}

	time.Sleep(15 * time.Millisecond)

	r2, _, err := entrySvc.SaveVaultMem(ctx, VaultMemSaveInput{
		Title:   "Step 2: Execution",
		Content: "Intermediate observation",
		Project: "proj-timeline",
	})
	if err != nil {
		t.Fatalf("save 2 failed: %v", err)
	}

	time.Sleep(15 * time.Millisecond)

	r3, _, err := entrySvc.SaveVaultMem(ctx, VaultMemSaveInput{
		Title:   "Step 3: Verification",
		Content: "Final verification",
		Project: "proj-timeline",
	})
	if err != nil {
		t.Fatalf("save 3 failed: %v", err)
	}

	// Test GetVaultMemTimeline slice (before and after target)
	timelineSlice, err := entrySvc.GetVaultMemTimeline(ctx, r2.Entry.ID, 1, 1, "proj-timeline")
	if err != nil {
		t.Fatalf("GetVaultMemTimeline failed: %v", err)
	}
	if len(timelineSlice) != 2 {
		t.Fatalf("expected 2 items (1 before, 1 after), got %d", len(timelineSlice))
	}

	// Ordering verification: chronologically ascending
	if timelineSlice[0].ID != r1.Entry.ID {
		t.Errorf("expected first timeline entry to be %s (Step 1), got %s", r1.Entry.ID, timelineSlice[0].ID)
	}
	if timelineSlice[1].ID != r3.Entry.ID {
		t.Errorf("expected second timeline entry to be %s (Step 3), got %s", r3.Entry.ID, timelineSlice[1].ID)
	}
	if !timelineSlice[0].CreatedAt.Before(timelineSlice[1].CreatedAt) {
		t.Errorf("expected chronological ordering: %v before %v", timelineSlice[0].CreatedAt, timelineSlice[1].CreatedAt)
	}

	// Test GetVaultMemTimelineResult structured result
	structured, err := entrySvc.GetVaultMemTimelineResult(ctx, r2.Entry.ID, 1, 1, "proj-timeline")
	if err != nil {
		t.Fatalf("GetVaultMemTimelineResult failed: %v", err)
	}
	if structured.Target.ID != r2.Entry.ID {
		t.Errorf("expected target ID %s, got %s", r2.Entry.ID, structured.Target.ID)
	}
	if len(structured.Before) != 1 || structured.Before[0].ID != r1.Entry.ID {
		t.Errorf("expected before item %s, got %+v", r1.Entry.ID, structured.Before)
	}
	if len(structured.After) != 1 || structured.After[0].ID != r3.Entry.ID {
		t.Errorf("expected after item %s, got %+v", r3.Entry.ID, structured.After)
	}
}

func TestGetVaultMemTimeline(t *testing.T) {
	TestVaultMemTimeline(t)
}

func TestVaultMemServiceFacade(t *testing.T) {
	_, memSvc, cleanup := setupVaultMemTest(t)
	defer cleanup()

	ctx := context.Background()

	saveRes, err := memSvc.Save(ctx, VaultMemSaveInput{
		Title:   "Facade test",
		Content: "Testing the high-level VaultMemService facade",
		Type:    "discovery",
		Project: "facade-proj",
		Topic:   "system/facade",
	})
	if err != nil {
		t.Fatalf("facade save failed: %v", err)
	}
	if saveRes.Deduplicated {
		t.Errorf("expected first save not deduplicated")
	}

	// Test Get
	getRes, err := memSvc.Get(ctx, saveRes.ID)
	if err != nil {
		t.Fatalf("facade get failed: %v", err)
	}
	if getRes.Title != "Facade test" {
		t.Errorf("unexpected get title: %s", getRes.Title)
	}

	// Test Search
	searchRes, err := memSvc.Search(ctx, "facade", "facade-proj", "", "", 10)
	if err != nil {
		t.Fatalf("facade search failed: %v", err)
	}
	if len(searchRes) == 0 {
		t.Fatalf("expected search match")
	}

	// Test Context
	ctxPack, err := memSvc.Context(ctx, "facade-proj")
	if err != nil {
		t.Fatalf("facade context failed: %v", err)
	}
	if ctxPack == "" {
		t.Errorf("expected non-empty context string")
	}
}

func TestStripPrivateTags(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"no tags", "Public architecture decision", "Public architecture decision"},
		{"single private tag", "Config with <private>supersecretkey123</private> value", "Config with [REDACTED] value"},
		{"case insensitive", "Config with <PRIVATE>token-xyz</PRIVATE> here", "Config with [REDACTED] here"},
		{"multiline private block", "Header\n<private>line1\nline2\nsecret</private>\nFooter", "Header\n[REDACTED]\nFooter"},
		{"multiple tags", "<private>a</private> middle <private>b</private>", "[REDACTED] middle [REDACTED]"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StripPrivateTags(tc.input)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestSaveVaultMem_StripPrivateTags(t *testing.T) {
	entrySvc, _, cleanup := setupVaultMemTest(t)
	defer cleanup()

	ctx := context.Background()

	input := VaultMemSaveInput{
		Title:   "Database auth <private>token-999</private>",
		Content: "Connect to DB with secret <private>password-xyz\nsuper-secret</private> done.",
		Type:    "config",
		Project: "secret-test-proj",
	}

	res, _, err := entrySvc.SaveVaultMem(ctx, input)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if strings.Contains(res.Entry.Title, "token-999") {
		t.Errorf("expected title to redact private tag, got %q", res.Entry.Title)
	}
	if !strings.Contains(res.Entry.Title, "[REDACTED]") {
		t.Errorf("expected title to contain [REDACTED], got %q", res.Entry.Title)
	}

	if strings.Contains(res.Entry.BodyOptional, "password-xyz") {
		t.Errorf("expected body to redact private tag, got %q", res.Entry.BodyOptional)
	}
	if !strings.Contains(res.Entry.BodyOptional, "[REDACTED]") {
		t.Errorf("expected body to contain [REDACTED], got %q", res.Entry.BodyOptional)
	}

	if strings.Contains(res.Entry.Summary, "password-xyz") {
		t.Errorf("expected summary to redact private tag, got %q", res.Entry.Summary)
	}
}

func TestPromoteVaultMem(t *testing.T) {
	entrySvc, memSvc, cleanup := setupVaultMemTest(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Save an observation
	obsInput := VaultMemSaveInput{
		Title:    "Use SQLite WAL for concurrent readers",
		Content:  "Enabling journal_mode=WAL allows readers not to block writers.",
		Type:     "decision",
		Project:  "wal-proj",
		TopicKey: "storage/sqlite-wal",
	}

	saveRes, _, err := entrySvc.SaveVaultMem(ctx, obsInput)
	if err != nil {
		t.Fatalf("save observation failed: %v", err)
	}
	srcID := saveRes.Entry.ID

	// 2. Test MinDuplicateCount gate (currently duplicate_count = 0)
	skipRes, err := memSvc.Promote(ctx, PromoteVaultMemInput{
		ID:                srcID,
		TargetType:        "decision",
		MinDuplicateCount: 1,
	})
	if err != nil {
		t.Fatalf("promote with min count failed: %v", err)
	}
	if !skipRes.Skipped {
		t.Errorf("expected promote to be skipped when duplicate_count < 1")
	}

	// 3. Re-learn the fact so duplicate_count bumps
	_, deduped, err := entrySvc.SaveVaultMem(ctx, obsInput)
	if err != nil || !deduped {
		t.Fatalf("expected fact to be deduplicated and duplicate_count bumped, err=%v, deduped=%v", err, deduped)
	}

	// 4. Now promote with MinDuplicateCount = 1 (should succeed)
	promoteRes, err := memSvc.Promote(ctx, PromoteVaultMemInput{
		ID:                srcID,
		TargetType:        "decision",
		MinDuplicateCount: 1,
	})
	if err != nil {
		t.Fatalf("promote failed: %v", err)
	}
	if promoteRes.Skipped {
		t.Fatalf("expected promote to succeed, got skipped: %s", promoteRes.Reason)
	}
	if promoteRes.PromotedID == "" || promoteRes.PromotedID == srcID {
		t.Errorf("expected new promoted entry ID distinct from src, got %s", promoteRes.PromotedID)
	}
	if promoteRes.Type != "decision" {
		t.Errorf("expected type decision, got %s", promoteRes.Type)
	}

	// 5. Verify the promoted entry in store
	promoted, err := entrySvc.GetEntry(ctx, promoteRes.PromotedID)
	if err != nil {
		t.Fatalf("failed to fetch promoted entry: %v", err)
	}
	if promoted.Entry.Entry.Title != obsInput.Title {
		t.Errorf("expected title %q, got %q", obsInput.Title, promoted.Entry.Entry.Title)
	}
	if promoted.Entry.Entry.Type != domain.EntryTypeDecision {
		t.Errorf("expected entry type decision, got %s", promoted.Entry.Entry.Type)
	}

	hasProvenance := false
	for _, tag := range promoted.Entry.Tags {
		if tag.Name == "promoted-from:"+srcID {
			hasProvenance = true
		}
		if tag.Name == "mem" {
			t.Errorf("expected 'mem' tag to be stripped from promoted canonical entry")
		}
	}
	if !hasProvenance {
		t.Errorf("expected promoted entry to have promoted-from tag")
	}

	// 6. Verify source observation still exists and is untouched
	srcObs, err := entrySvc.GetEntry(ctx, srcID)
	if err != nil {
		t.Fatalf("source observation should still exist: %v", err)
	}
	if srcObs.Entry.Entry.Status != domain.StatusActive {
		t.Errorf("source observation should remain active, got %s", srcObs.Entry.Entry.Status)
	}
}
