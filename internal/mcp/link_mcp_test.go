package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestLinkMCPTools(t *testing.T) {
	reg, _, cleanup := setupMCPServices(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Test add_link
	addRes, err := reg.Call(ctx, "add_link", map[string]interface{}{
		"url":     "https://github.com/charmbracelet/bubbletea",
		"alias":   "btea",
		"title":   "Bubble Tea",
		"summary": "A powerful TUI framework for Go",
		"tags":    []interface{}{"go", "tui", "charm"},
	})
	if err != nil {
		t.Fatalf("add_link failed: %v", err)
	}
	if addRes.IsError {
		t.Fatalf("add_link returned error: %s", addRes.Content[0].Text)
	}

	var addData map[string]interface{}
	if err := json.Unmarshal([]byte(addRes.Content[0].Text), &addData); err != nil {
		t.Fatalf("unmarshal add_link result: %v", err)
	}
	if addData["url"] != "https://github.com/charmbracelet/bubbletea" {
		t.Errorf("expected url in add_link, got %v", addData["url"])
	}

	// 2. Test get_link with valid alias
	getRes, err := reg.Call(ctx, "get_link", map[string]interface{}{
		"alias": "btea",
	})
	if err != nil {
		t.Fatalf("get_link failed: %v", err)
	}
	if getRes.IsError {
		t.Fatalf("get_link returned error: %s", getRes.Content[0].Text)
	}

	var getData map[string]interface{}
	if err := json.Unmarshal([]byte(getRes.Content[0].Text), &getData); err != nil {
		t.Fatalf("unmarshal get_link result: %v", err)
	}
	if getData["url"] != "https://github.com/charmbracelet/bubbletea" {
		t.Errorf("expected url %s, got %v", "https://github.com/charmbracelet/bubbletea", getData["url"])
	}
	if getData["title"] != "Bubble Tea" {
		t.Errorf("expected title 'Bubble Tea', got %v", getData["title"])
	}

	// 3. Test get_link with unknown alias
	getMissing, err := reg.Call(ctx, "get_link", map[string]interface{}{
		"alias": "nonexistent",
	})
	if err != nil {
		t.Fatalf("get_link nonexistent call error: %v", err)
	}
	if !getMissing.IsError {
		t.Errorf("expected error for nonexistent alias, got success: %v", getMissing.Content[0].Text)
	}

	// 4. Test search_links
	searchRes, err := reg.Call(ctx, "search_links", map[string]interface{}{
		"query": "bubbletea",
	})
	if err != nil {
		t.Fatalf("search_links failed: %v", err)
	}
	if searchRes.IsError {
		t.Fatalf("search_links returned error: %s", searchRes.Content[0].Text)
	}

	var searchData struct {
		Count   int `json:"count"`
		Results []struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(searchRes.Content[0].Text), &searchData); err != nil {
		t.Fatalf("unmarshal search_links: %v", err)
	}
	if searchData.Count < 1 {
		t.Errorf("expected at least 1 search result, got %d", searchData.Count)
	}

	// 5. Test list_links
	listRes, err := reg.Call(ctx, "list_links", map[string]interface{}{})
	if err != nil {
		t.Fatalf("list_links failed: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("list_links returned error: %s", listRes.Content[0].Text)
	}

	var listData struct {
		Count int `json:"count"`
		Links []struct {
			URL string `json:"url"`
		} `json:"links"`
	}
	if err := json.Unmarshal([]byte(listRes.Content[0].Text), &listData); err != nil {
		t.Fatalf("unmarshal list_links: %v", err)
	}
	if listData.Count < 1 {
		t.Errorf("expected at least 1 link in list, got %d", listData.Count)
	}
}
