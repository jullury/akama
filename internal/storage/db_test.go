package storage

import (
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrate_Idempotent(t *testing.T) {
	db1, err := Open(":memory:")
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	defer db1.Close()

	db2, err := Open(":memory:")
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer db2.Close()
}

func TestJobCRUD(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	j := &Job{
		ChatID:     12345,
		IssueID:    "42",
		IssueTitle: "Fix the bug",
		IssueBody:  "There is a bug in the code.",
		IssueURL:   "https://github.com/example/repo/issues/42",
		RepoURL:    "https://github.com/example/repo",
		Provider:   "github",
		GitToken:   "ghp_test_token",
		Agent:      "claude",
		AgentModel: "claude-sonnet-4-6",
	}

	id, err := CreateJob(db, j)
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}
	if id == 0 {
		t.Fatal("CreateJob returned id = 0")
	}

	got, err := GetJob(db, id)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if got.ChatID != j.ChatID {
		t.Errorf("ChatID: got %d, want %d", got.ChatID, j.ChatID)
	}
	if got.IssueTitle != j.IssueTitle {
		t.Errorf("IssueTitle: got %q, want %q", got.IssueTitle, j.IssueTitle)
	}
	if got.Status != "pending" {
		t.Errorf("Status: got %q, want 'pending'", got.Status)
	}

	// SetJobRunning
	if err := SetJobRunning(db, id, "/tmp/workspace"); err != nil {
		t.Fatalf("SetJobRunning failed: %v", err)
	}
	got, err = GetJob(db, id)
	if err != nil {
		t.Fatalf("GetJob after SetJobRunning failed: %v", err)
	}
	if got.Status != "running" {
		t.Errorf("Status after SetJobRunning: got %q, want 'running'", got.Status)
	}
	if got.WorkspacePath != "/tmp/workspace" {
		t.Errorf("WorkspacePath: got %q, want '/tmp/workspace'", got.WorkspacePath)
	}

	// SetJobFailed
	if err := SetJobFailed(db, id, "something went wrong"); err != nil {
		t.Fatalf("SetJobFailed failed: %v", err)
	}
	got, err = GetJob(db, id)
	if err != nil {
		t.Fatalf("GetJob after SetJobFailed failed: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("Status after SetJobFailed: got %q, want 'failed'", got.Status)
	}
	if got.ErrorMsg != "something went wrong" {
		t.Errorf("ErrorMsg: got %q, want 'something went wrong'", got.ErrorMsg)
	}
}

func TestConnectionCRUD(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	var chatID int64 = 99999

	err = SaveConnection(db, chatID, "github", "https://github.com/example/repo", "ghp_token", "main")
	if err != nil {
		t.Fatalf("SaveConnection failed: %v", err)
	}

	conns, err := FindConnectionsByChat(db, chatID)
	if err != nil {
		t.Fatalf("FindConnectionsByChat failed: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	if conns[0].Provider != "github" {
		t.Errorf("Provider: got %q, want 'github'", conns[0].Provider)
	}
	if conns[0].RepoURL != "https://github.com/example/repo" {
		t.Errorf("RepoURL: got %q, want 'https://github.com/example/repo'", conns[0].RepoURL)
	}

	connID := int(conns[0].ID)
	err = DeleteConnection(db, connID)
	if err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}

	conns, err = FindConnectionsByChat(db, chatID)
	if err != nil {
		t.Fatalf("FindConnectionsByChat after delete failed: %v", err)
	}
	if len(conns) != 0 {
		t.Fatalf("expected 0 connections after delete, got %d", len(conns))
	}
}

func TestConversationState(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	var chatID int64 = 77777
	platform := "telegram"

	// Before setting state, GetConversation should return idle
	conv, err := GetConversation(db, chatID, platform)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if conv.State != "idle" {
		t.Errorf("initial state: got %q, want 'idle'", conv.State)
	}

	// SetConversationState
	data := map[string]interface{}{"job_id": float64(42)}
	err = SetConversationState(db, chatID, platform, "await_agent_input", data)
	if err != nil {
		t.Fatalf("SetConversationState failed: %v", err)
	}

	conv, err = GetConversation(db, chatID, platform)
	if err != nil {
		t.Fatalf("GetConversation after set failed: %v", err)
	}
	if conv.State != "await_agent_input" {
		t.Errorf("state after set: got %q, want 'await_agent_input'", conv.State)
	}
	if conv.Data["job_id"] != float64(42) {
		t.Errorf("data job_id: got %v, want 42", conv.Data["job_id"])
	}

	// ResetConversationState
	err = ResetConversationState(db, chatID)
	if err != nil {
		t.Fatalf("ResetConversationState failed: %v", err)
	}

	conv, err = GetConversation(db, chatID, platform)
	if err != nil {
		t.Fatalf("GetConversation after reset failed: %v", err)
	}
	if conv.State != "idle" {
		t.Errorf("state after reset: got %q, want 'idle'", conv.State)
	}
}
