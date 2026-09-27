package processor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParentDirectoryImportInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	child := filepath.Join(workspace, "nested")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	declarations := "|===|\nalias Age: int;\n|===|\n[output = 'schema']\n{ Age: Age, }"
	if err := os.WriteFile(filepath.Join(workspace, "types.mace"), []byte(declarations), 0600); err != nil {
		t.Fatal(err)
	}
	input := "|===|\nfrom '../types.mace' import Age;\n|===|\n[output = 'data']\n{ age: 42, }"
	path := filepath.Join(child, "config.mace")
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := New().ProcessFileInDir(path, workspace)
	if err != nil {
		t.Fatalf("expected a parent import inside the workspace to work: %v", err)
	}
	if result.Output["age"].Int != 42 {
		t.Fatalf("expected age 42, got %#v", result.Output["age"])
	}
}
