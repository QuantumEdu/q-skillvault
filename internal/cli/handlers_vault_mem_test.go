package cli

import (
	"reflect"
	"testing"
)

func TestParseMemSaveFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    memSaveFlags
		wantErr bool
	}{
		{
			name: "positional content only",
			args: []string{"skillvault", "mem", "save", "Implemented", "feature", "x"},
			want: memSaveFlags{
				Content: "Implemented feature x",
				Type:    "decision",
				Scope:   "project",
			},
		},
		{
			name: "explicit flags",
			args: []string{"skillvault", "mem", "save", "--content", "Fixed N+1 query", "--title", "N+1 Fix", "--type", "bugfix", "--project", "myproj", "--topic", "perf", "--tags", "sql,perf", "--json"},
			want: memSaveFlags{
				Content:  "Fixed N+1 query",
				Title:    "N+1 Fix",
				Type:     "bugfix",
				Project:  "myproj",
				TopicKey: "perf",
				Scope:    "project",
				Tags:     "sql,perf",
				JSON:     true,
			},
		},
		{
			name:    "missing content",
			args:    []string{"skillvault", "mem", "save"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMemSaveFlags(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMemSaveFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseMemSaveFlags() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseMemSearchFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want memSearchFlags
	}{
		{
			name: "positional query",
			args: []string{"skillvault", "mem", "search", "sqlite", "wal"},
			want: memSearchFlags{
				Query: "sqlite wal",
				Limit: 20,
			},
		},
		{
			name: "with filters and json",
			args: []string{"skillvault", "mem", "search", "--query", "auth", "--project", "p1", "--type", "decision", "--limit", "5", "--json"},
			want: memSearchFlags{
				Query:   "auth",
				Project: "p1",
				Type:    "decision",
				Limit:   5,
				JSON:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMemSearchFlags(tt.args)
			if err != nil {
				t.Fatalf("parseMemSearchFlags() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseMemSearchFlags() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseMemGetFlags(t *testing.T) {
	t.Run("valid id positional", func(t *testing.T) {
		got, err := parseMemGetFlags([]string{"skillvault", "mem", "get", "obs-123"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "obs-123" || got.JSON {
			t.Errorf("unexpected result: %+v", got)
		}
	})

	t.Run("missing id", func(t *testing.T) {
		_, err := parseMemGetFlags([]string{"skillvault", "mem", "get"})
		if err == nil {
			t.Errorf("expected error for missing id")
		}
	})
}

func TestParseMemTimelineFlags(t *testing.T) {
	t.Run("valid timeline args", func(t *testing.T) {
		got, err := parseMemTimelineFlags([]string{"skillvault", "mem", "timeline", "obs-123", "--before", "5", "--after", "2", "--project", "p1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "obs-123" || got.Before != 5 || got.After != 2 || got.Project != "p1" {
			t.Errorf("unexpected result: %+v", got)
		}
	})
}

func TestParseMemContextFlags(t *testing.T) {
	t.Run("valid project arg", func(t *testing.T) {
		got, err := parseMemContextFlags([]string{"skillvault", "mem", "context", "p1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Project != "p1" || got.MaxChars != 8000 {
			t.Errorf("unexpected result: %+v", got)
		}
	})

	t.Run("missing project", func(t *testing.T) {
		_, err := parseMemContextFlags([]string{"skillvault", "mem", "context"})
		if err == nil {
			t.Errorf("expected error for missing project")
		}
	})
}
