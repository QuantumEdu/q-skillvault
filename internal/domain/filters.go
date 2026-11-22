package domain

type SearchQuery struct {
	Query           string
	ProjectID       *string
	SeriesID        *string
	Type            *string
	Purpose         *string
	Tags            []string
	IncludeArchived bool
	// IncludeSuperseded returns revisions replaced by a newer memory. Off by
	// default: a superseded fact is history, not an answer.
	IncludeSuperseded bool
	Limit             int
	Vector            bool
}

type VaultExport struct {
	SchemaVersion int       `json:"schema_version"`
	AppVersion    string    `json:"app_version"`
	ExportedAt    string    `json:"exported_at"`
	Source        string    `json:"source"`
	Data          VaultData `json:"data"`
}

type VaultData struct {
	Projects         []Project         `json:"projects"`
	Entries          []Entry           `json:"entries"`
	EntryTags        []EntryTag        `json:"entry_tags"`
	Tags             []Tag             `json:"tags"`
	Series           []Series          `json:"series"`
	SeriesEntries    []SeriesEntry     `json:"series_entries"`
	Workflows        []Workflow        `json:"workflows"`
	WorkflowSteps    []WorkflowStep    `json:"workflow_steps"`
	WorkflowRuns     []WorkflowRun     `json:"workflow_runs"`
	WorkflowRunSteps []WorkflowRunStep `json:"workflow_run_steps"`
	Artifacts        []Artifact        `json:"artifacts"`
	EntryLinks       []EntryLink       `json:"entry_links"`
	// Schema v4 preserves memory mechanics independently of the public Entry DTO.
	// An omitted map in a legacy snapshot must not reset destination quality.
	MemoryQuality map[string]MemoryQualityRecord `json:"memory_quality,omitempty"`
	EntryVersions []EntryVersion                 `json:"entry_versions,omitempty"`
}

// MemoryQualityRecord is the portable state of one memory, keyed by entry ID.
type MemoryQualityRecord struct {
	NormalizedHash string `json:"normalized_hash"`
	TopicKey       string `json:"topic_key"`
	RevisionCount  int    `json:"revision_count"`
	DuplicateCount int    `json:"duplicate_count"`
	SupersededBy   string `json:"superseded_by,omitempty"`
	Pinned         bool   `json:"pinned"`
	ReviewAfter    string `json:"review_after,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
}

// VaultPackExport wraps a VaultExport with pack metadata for sharing
// curated vault content between instances. The JSON structure uses a
// top-level "pack" key containing metadata, with "data" holding the
// standard VaultExport payload.
type VaultPackExport struct {
	Pack PackMetadata `json:"pack"`
	Data VaultExport  `json:"data"`
}

// PackMetadata holds identifying information for a skill pack export.
type PackMetadata struct {
	PackID      string `json:"pack_id"`
	Author      string `json:"author"`
	Version     string `json:"version"`
	Description string `json:"description"`
	ExportedAt  string `json:"exported_at"`
}
