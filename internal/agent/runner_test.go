package agent

import (
	"testing"
)

func TestIsQuestion_DetectsMarker(t *testing.T) {
	text := "I have reviewed the code.\nINPUT_REQUIRED: What is the expected output format?"
	if !IsQuestion(text) {
		t.Fatal("expected IsQuestion to return true for text containing INPUT_REQUIRED")
	}
}

func TestIsQuestion_NoFalsePositive(t *testing.T) {
	text := "Did you check the file? I think there might be an issue."
	if IsQuestion(text) {
		t.Fatal("expected IsQuestion to return false for text without INPUT_REQUIRED marker")
	}
}

func TestIsQuestion_Empty(t *testing.T) {
	if IsQuestion("") {
		t.Fatal("expected IsQuestion to return false for empty string")
	}
}

func TestIsQuestion_EmptyQuestionAfterMarker(t *testing.T) {
	// Marker present but no question text — should NOT be treated as a question.
	text := "some output\nINPUT_REQUIRED:"
	if IsQuestion(text) {
		t.Fatal("expected IsQuestion to return false when marker has no question text")
	}
}

func TestExtractQuestion_ReturnsQuestion(t *testing.T) {
	text := "Some agent output.\nINPUT_REQUIRED: What is your name?"
	got := ExtractQuestion(text)
	want := "What is your name?"
	if got != want {
		t.Fatalf("ExtractQuestion returned %q, want %q", got, want)
	}
}

func TestExtractQuestion_MultilineAfterMarker(t *testing.T) {
	text := "Preamble\nINPUT_REQUIRED: Question\nMore text on next line"
	got := ExtractQuestion(text)
	want := "Question"
	if got != want {
		t.Fatalf("ExtractQuestion returned %q, want %q (should stop at first newline)", got, want)
	}
}

func TestExtractQuestion_NoMarker(t *testing.T) {
	text := "This is plain output with no marker."
	got := ExtractQuestion(text)
	if got != text {
		t.Fatalf("ExtractQuestion without marker should return full text, got %q", got)
	}
}

func TestBranchFromCommit(t *testing.T) {
	cases := []struct {
		commitMsg string
		fallback  string
		want      string
	}{
		{
			commitMsg: "feat: implement OWASP 2025 top 10",
			fallback:  "fix/issue-1",
			want:      "feat/implement-owasp-2025-top-10",
		},
		{
			commitMsg: "fix: resolve nil pointer dereference",
			fallback:  "fix/issue-2",
			want:      "fix/resolve-nil-pointer-dereference",
		},
		{
			commitMsg: "not-a-conventional-commit",
			fallback:  "fix/issue-3",
			want:      "fix/issue-3",
		},
		{
			commitMsg: "",
			fallback:  "fix/issue-4",
			want:      "fix/issue-4",
		},
		{
			// scope stripping: "feat(auth): add login" → "feat/add-login"
			commitMsg: "feat(auth): add login",
			fallback:  "fix/issue-5",
			want:      "feat/add-login",
		},
	}

	for _, tc := range cases {
		got := BranchFromCommit(tc.commitMsg, tc.fallback)
		if got != tc.want {
			t.Errorf("BranchFromCommit(%q, %q) = %q, want %q", tc.commitMsg, tc.fallback, got, tc.want)
		}
	}
}
