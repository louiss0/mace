package processor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCancelledOperationReturnsBeforeEvaluating(t *testing.T) {
	operation, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewWithContext(operation, nil).Process("[output = 'data']\n{ enabled: true, }")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestDeadlineCancelsStalledRemoteImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()

	workspace := t.TempDir()
	operation, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	input := "|===|\nfrom '" + server.URL + "/schema.mace' import Age;\n|===|\n[output = 'data']\n{ age: 42, }"
	_, err := NewWithContext(operation, nil).ProcessInScope(input, workspace, workspace)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a deadline error, got %v", err)
	}
}

func TestEntryFileOutsideWorkspaceIsRejected(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	path := filepath.Join(outside, "config.mace")
	if err := os.WriteFile(path, []byte("[output = 'data']\n{ enabled: true, }"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := New().ProcessFileInDir(path, workspace)
	if err == nil {
		t.Fatal("expected an entry file outside the workspace to be rejected")
	}
}
