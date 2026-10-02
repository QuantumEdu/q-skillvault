package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestInstructionMCPTools(t *testing.T) {
	reg, _, cleanup := setupMCPServices(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Test add_instruction
	addRes, err := reg.Call(ctx, "add_instruction", map[string]interface{}{
		"command": "ffmpeg -i {{input}} -vcodec libx264 -crf {{crf}} {{output}}",
		"alias":   "ff-compress",
		"title":   "Compress Video H264",
		"summary": "Balance size and quality with ffmpeg",
		"tags":    []interface{}{"video", "ffmpeg"},
	})
	if err != nil {
		t.Fatalf("add_instruction failed: %v", err)
	}
	if addRes.IsError {
		t.Fatalf("add_instruction returned error: %s", addRes.Content[0].Text)
	}

	var addData map[string]interface{}
	if err := json.Unmarshal([]byte(addRes.Content[0].Text), &addData); err != nil {
		t.Fatalf("unmarshal add_instruction result: %v", err)
	}
	if addData["alias"] != "ff-compress" {
		t.Errorf("expected alias ff-compress, got %v", addData["alias"])
	}

	// 2. Test get_instruction without variables (returns template and missing_vars)
	getResRaw, err := reg.Call(ctx, "get_instruction", map[string]interface{}{
		"alias": "ff-compress",
	})
	if err != nil {
		t.Fatalf("get_instruction raw failed: %v", err)
	}
	if getResRaw.IsError {
		t.Fatalf("get_instruction raw returned error: %s", getResRaw.Content[0].Text)
	}

	var getRawData struct {
		Command     string   `json:"command"`
		MissingVars []string `json:"missing_vars"`
		Variables   []string `json:"variables"`
	}
	if err := json.Unmarshal([]byte(getResRaw.Content[0].Text), &getRawData); err != nil {
		t.Fatalf("unmarshal get_instruction raw: %v", err)
	}
	if len(getRawData.MissingVars) != 3 {
		t.Errorf("expected 3 missing vars, got %v", getRawData.MissingVars)
	}

	// 3. Test get_instruction with variables interpolation
	getResResolved, err := reg.Call(ctx, "get_instruction", map[string]interface{}{
		"alias": "ff-compress",
		"vars": map[string]interface{}{
			"input":  "clip.mov",
			"crf":    "28",
			"output": "clip.mp4",
		},
	})
	if err != nil {
		t.Fatalf("get_instruction resolved failed: %v", err)
	}
	if getResResolved.IsError {
		t.Fatalf("get_instruction resolved returned error: %s", getResResolved.Content[0].Text)
	}

	var getResolvedData struct {
		Command     string   `json:"command"`
		MissingVars []string `json:"missing_vars"`
	}
	if err := json.Unmarshal([]byte(getResResolved.Content[0].Text), &getResolvedData); err != nil {
		t.Fatalf("unmarshal get_instruction resolved: %v", err)
	}
	expectedCmd := "ffmpeg -i clip.mov -vcodec libx264 -crf 28 clip.mp4"
	if getResolvedData.Command != expectedCmd {
		t.Errorf("expected %q, got %q", expectedCmd, getResolvedData.Command)
	}
	if len(getResolvedData.MissingVars) != 0 {
		t.Errorf("expected 0 missing vars, got %v", getResolvedData.MissingVars)
	}

	// 4. Test search_instructions
	searchRes, err := reg.Call(ctx, "search_instructions", map[string]interface{}{
		"query": "ffmpeg",
	})
	if err != nil {
		t.Fatalf("search_instructions failed: %v", err)
	}
	if searchRes.IsError {
		t.Fatalf("search_instructions returned error: %s", searchRes.Content[0].Text)
	}

	var searchData struct {
		Count   int `json:"count"`
		Results []struct {
			Alias   string `json:"alias"`
			Command string `json:"command"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(searchRes.Content[0].Text), &searchData); err != nil {
		t.Fatalf("unmarshal search_instructions: %v", err)
	}
	if searchData.Count < 1 {
		t.Errorf("expected at least 1 search result, got %d", searchData.Count)
	}

	// 5. Test list_instructions
	listRes, err := reg.Call(ctx, "list_instructions", map[string]interface{}{})
	if err != nil {
		t.Fatalf("list_instructions failed: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("list_instructions returned error: %s", listRes.Content[0].Text)
	}

	var listData struct {
		Count        int `json:"count"`
		Instructions []struct {
			Alias string `json:"alias"`
		} `json:"instructions"`
	}
	if err := json.Unmarshal([]byte(listRes.Content[0].Text), &listData); err != nil {
		t.Fatalf("unmarshal list_instructions: %v", err)
	}
	if listData.Count < 1 {
		t.Errorf("expected at least 1 instruction in list, got %d", listData.Count)
	}
}
