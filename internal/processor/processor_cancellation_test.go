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

	"github.com/louiss0/mace/internal/parser/ast"
)

func TestCancelledOperationReturnsBeforeEvaluating(t *testing.T) {
	operation, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewWithContext(operation, nil).Process("[output = 'data']\n{ enabled: true, }")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestValueEvaluationStopsWhenTheOperationIsCancelled(t *testing.T) {
	operation, cancel := context.WithCancel(context.Background())
	cancel()
	environment := newValueEnvironment()
	environment.operation = operation

	_, err := evaluateExpression(
		ast.IntLiteral{Lexeme: "1"},
		environment,
		Value{},
		newSymbolTable(),
		newTypeRegistry(),
		newSchemaRegistry(),
		nil,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected value evaluation to stop, got %v", err)
	}
}

func TestOperationWithoutADeadlineStillGetsTheDefaultEvaluationTimeout(t *testing.T) {
	instance := NewWithContext(context.Background(), nil)

	operation, cancel := instance.operationContext()
	defer cancel()

	deadline, ok := operation.Deadline()
	if !ok {
		t.Fatal("expected an operation without a deadline to receive the default timeout")
	}
	if remaining := time.Until(deadline); remaining > DefaultEvaluationTimeout {
		t.Fatalf("default deadline is %v away, want at most %v", remaining, DefaultEvaluationTimeout)
	}
}

func TestCallerDeadlineShorterThanTheDefaultIsPreserved(t *testing.T) {
	operation, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	callerDeadline, _ := operation.Deadline()

	derived, cancelDerived := NewWithContext(operation, nil).operationContext()
	defer cancelDerived()

	derivedDeadline, ok := derived.Deadline()
	if !ok {
		t.Fatal("expected the caller deadline to be preserved")
	}
	if remaining := time.Until(derivedDeadline); remaining > 50*time.Millisecond {
		t.Fatalf("derived deadline is %v away, want at most the caller deadline", remaining)
	}
	if !derivedDeadline.After(callerDeadline) && !derivedDeadline.Equal(callerDeadline) {
		t.Fatalf("derived deadline %v does not extend the caller deadline %v", derivedDeadline, callerDeadline)
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

func TestDeadlineCancelsStalledRemoteSchemaFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-time.After(time.Second):
			_, _ = writer.Write([]byte("[output = 'schema']\n{ Age: int, }"))
		}
	}))
	defer server.Close()

	workspace := t.TempDir()
	operation, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	input := "[output = 'data', schema_file = '" + server.URL + "/schema.mace']\n{ age: 42, }"
	started := time.Now()
	_, err := NewWithContext(operation, nil).ProcessInScope(input, workspace, workspace)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a deadline error, got %v", err)
	}
	if time.Since(started) >= 500*time.Millisecond {
		t.Fatalf("schema-file request exceeded its deadline")
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
