package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/quantum-6/skillvault/internal/domain"
)

func TestSemanticRelationsMigrationPreservesExistingEdges(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDB(t.TempDir() + "/legacy.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name() >= "014" {
			continue
		}
		script, err := migrationsFS.ReadFile("migrations/" + file.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(script)); err != nil {
			t.Fatalf("legacy schema %s: %v", file.Name(), err)
		}
	}
	entries := &sqliteEntryStore{db: db}
	for _, id := range []string{"a", "b"} {
		if err := entries.Save(ctx, domain.Entry{ID: id, Slug: id, Title: id, Type: domain.EntryTypeReference}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO entry_links(from_entry_id,to_entry_id,relation_type,label,active,created_at) VALUES('a','b','references','preserved',0,'2020-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	var label, created string
	var active bool
	if err := db.QueryRow(`SELECT label,active,CAST(created_at AS TEXT) FROM entry_links WHERE relation_type='references'`).Scan(&label, &active, &created); err != nil {
		t.Fatal(err)
	}
	if label != "preserved" || active || created != "2020-01-01 00:00:00" {
		t.Fatal("migration changed existing edge")
	}
	links := &sqliteEntryLinkStore{db: db}
	for _, kind := range []domain.RelationType{domain.RelationReferences, domain.RelationSupersedes, domain.RelationRelatedTo, domain.RelationPartOf, domain.RelationDerivedFrom, domain.RelationImplements, domain.RelationUses, domain.RelationExtends, domain.RelationHandoffOf, domain.RelationGeneratedFrom, domain.RelationDependsOn, domain.RelationContradicts, domain.RelationCites} {
		if err := links.Save(ctx, domain.EntryLink{FromEntryID: "a", ToEntryID: "b", RelationType: kind}); err != nil {
			t.Fatalf("public relation %s rejected: %v", kind, err)
		}
	}
	if err := links.Save(ctx, domain.EntryLink{FromEntryID: "a", ToEntryID: "b", RelationType: "invalid"}); err == nil {
		t.Fatal("CHECK constraint lost")
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("migration replay: %v", err)
	}
}

func TestAtomicMigrationRollsBackDDLAndVersion(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	script := `CREATE TABLE partial_state(id TEXT); INSERT INTO schema_migrations VALUES(99,'test'); INSERT INTO missing_table VALUES(1);`
	if err := applyAtomicMigration(db, script); err == nil {
		t.Fatal("broken migration accepted")
	}
	var count int
	for _, query := range []string{`SELECT count(*) FROM sqlite_master WHERE name='partial_state'`, `SELECT count(*) FROM schema_migrations WHERE version=99`} {
		if err := db.QueryRow(query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial migration remained: %v count=%d", err, count)
		}
	}
	// The failed transaction must release the connection so retry can succeed.
	if err := applyAtomicMigration(db, strings.ReplaceAll(script, "INSERT INTO missing_table VALUES(1);", "")); err != nil {
		t.Fatal(err)
	}
}
