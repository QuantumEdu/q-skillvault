package app

import (
	"testing"

	"github.com/quantum-6/skillvault/internal/domain"
)

func TestPrefixPreservesMemoryOwnershipAndHistory(t *testing.T) {
	data := domain.VaultData{
		Entries:       []domain.Entry{{ID: "old"}, {ID: "new"}},
		MemoryQuality: map[string]domain.MemoryQualityRecord{"old": {RevisionCount: 1, SupersededBy: "new"}, "new": {RevisionCount: 2, Pinned: true}},
		EntryVersions: []domain.EntryVersion{{VersionID: "v1", EntryID: "new", VersionNumber: 1}},
	}
	applyPrefix("lab/", &data)
	if _, ok := data.MemoryQuality["old"]; ok {
		t.Fatal("unprefixed quality retained")
	}
	if data.MemoryQuality["lab/old"].SupersededBy != "lab/new" || !data.MemoryQuality["lab/new"].Pinned {
		t.Fatal("quality ownership lost")
	}
	if data.EntryVersions[0].VersionID != "lab/v1" || data.EntryVersions[0].EntryID != "lab/new" {
		t.Fatal("history not namespaced")
	}
}
