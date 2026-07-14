package job

import (
	"testing"

	"github.com/jullury/akama/internal/storage"
)

func TestResolveGroupWorkspace(t *testing.T) {
	tests := []struct {
		name     string
		primary  *storage.Job
		jobs     []*storage.Job
		expected string
	}{
		{
			name:     "primary workspace",
			primary:  &storage.Job{WorkspacePath: "/workspaces/github/owner/repo/6/owner-repo"},
			jobs:     []*storage.Job{},
			expected: "/workspaces/github/owner/repo/6",
		},
		{
			name:     "fallback to first job",
			primary:  &storage.Job{},
			jobs:     []*storage.Job{{WorkspacePath: "/workspaces/github/owner/repo/6/owner-repo"}},
			expected: "/workspaces/github/owner/repo/6",
		},
		{
			name:     "primary wins over first job",
			primary:  &storage.Job{WorkspacePath: "/workspaces/github/owner/repo/6/owner-repo"},
			jobs:     []*storage.Job{{WorkspacePath: "/workspaces/github/other/repo/1/other-repo"}},
			expected: "/workspaces/github/owner/repo/6",
		},
		{
			name:     "all empty",
			primary:  &storage.Job{},
			jobs:     []*storage.Job{{}},
			expected: "",
		},
		{
			name:     "nil primary",
			primary:  nil,
			jobs:     []*storage.Job{{WorkspacePath: "/workspaces/github/owner/repo/6/owner-repo"}},
			expected: "/workspaces/github/owner/repo/6",
		},
		{
			name:     "empty jobs slice",
			primary:  &storage.Job{WorkspacePath: "/workspaces/github/owner/repo/6/owner-repo"},
			jobs:     []*storage.Job{},
			expected: "/workspaces/github/owner/repo/6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveGroupWorkspace(tt.primary, tt.jobs)
			if got != tt.expected {
				t.Fatalf("resolveGroupWorkspace() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestResolveGroupWorkspace_AvoidsDoublePath(t *testing.T) {
	// Regression: previously groupWorkspace was set to primary.WorkspacePath
	// directly, so the later loop joined it again and produced a doubled path
	// like /base/owner-repo/owner-repo that did not exist.
	primary := &storage.Job{WorkspacePath: "/workspaces/github/jullury/fimpiz/6/jullury-fimpiz"}
	groupWorkspace := resolveGroupWorkspace(primary, nil)
	want := "/workspaces/github/jullury/fimpiz/6"
	if groupWorkspace != want {
		t.Fatalf("expected shared workspace %q, got %q", want, groupWorkspace)
	}
}
