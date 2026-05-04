package run

import "testing"

func TestWorkspaceChangeStatuses(t *testing.T) {
	tests := map[WorkspaceChangeStatus]string{
		WorkspaceChangeAdded:    "added",
		WorkspaceChangeModified: "modified",
		WorkspaceChangeDeleted:  "deleted",
	}

	for status, want := range tests {
		if string(status) != want {
			t.Fatalf("status = %q, want %q", status, want)
		}
	}
}
