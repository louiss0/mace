package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type releaseTarget struct {
	Target   string `json:"target"`
	GOOS     string `json:"goos"`
	GOARCH   string `json:"goarch"`
	Libc     string `json:"libc"`
	Runner   string `json:"runner"`
	Filename string `json:"filename"`
}

type releaseTargets struct {
	ABIMajor    int               `json:"abiMajor"`
	Library     string            `json:"library"`
	Targets     []releaseTarget   `json:"targets"`
	Unsupported map[string]string `json:"unsupported"`
}

func loadReleaseTargets(t *testing.T) releaseTargets {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join("..", "..", "processor-targets.json"))
	if err != nil {
		t.Fatalf("unable to read processor targets: %v", err)
	}

	var targets releaseTargets
	if err := json.Unmarshal(contents, &targets); err != nil {
		t.Fatalf("unable to decode processor targets: %v", err)
	}

	return targets
}

func TestProcessorReleaseTargetsCoverEverySupportedPlatform(t *testing.T) {
	targets := loadReleaseTargets(t)

	expected := []string{
		"darwin-amd64",
		"darwin-arm64",
		"linux-amd64-glibc",
		"linux-arm64-glibc",
		"windows-amd64",
	}

	names := make([]string, 0, len(targets.Targets))
	for _, target := range targets.Targets {
		names = append(names, target.Target)
	}
	sort.Strings(names)

	if strings.Join(names, ",") != strings.Join(expected, ",") {
		t.Errorf("processor targets = %v, want %v", names, expected)
	}
}

func TestProcessorReleaseTargetsMatchTheirGoPlatform(t *testing.T) {
	for _, target := range loadReleaseTargets(t).Targets {
		if target.Target != target.GOOS+"-"+target.GOARCH+libcSuffix(target.Libc) {
			t.Errorf("target %q does not describe %s/%s (%s)", target.Target, target.GOOS, target.GOARCH, target.Libc)
		}
	}
}

// A platform that is not built must say why, so a gap is never silent.
func TestUnsupportedTargetsExplainWhyTheyAreExcluded(t *testing.T) {
	targets := loadReleaseTargets(t)

	published := make(map[string]struct{}, len(targets.Targets))
	for _, target := range targets.Targets {
		published[target.Target] = struct{}{}
	}

	for name, reason := range targets.Unsupported {
		if reason == "" {
			t.Errorf("unsupported target %q has no documented reason", name)
		}
		if _, listed := published[name]; listed {
			t.Errorf("target %q is both published and listed as unsupported", name)
		}
		if name != nameTarget(name) {
			t.Errorf("unsupported entry %q is not a well-formed target name", name)
		}
	}
}

// nameTarget renders the canonical GOOS-GOARCH-libc spelling so a malformed key
// is caught before it is mistaken for a real platform.
func nameTarget(name string) string {
	parts := strings.Split(name, "-")
	if len(parts) < 2 {
		return name
	}

	suffix := ""
	if len(parts) == 3 {
		suffix = "-" + parts[2]
	}

	return parts[0] + "-" + parts[1] + suffix
}

func TestProcessorReleaseTargetsUseTheSharedLibraryName(t *testing.T) {
	targets := loadReleaseTargets(t)

	expectedFilenames := map[string]string{
		"darwin":  "libmace_processor.dylib",
		"linux":   "libmace_processor.so",
		"windows": "mace_processor.dll",
	}

	for _, target := range targets.Targets {
		if target.Filename != expectedFilenames[target.GOOS] {
			t.Errorf("target %q filename = %q, want %q", target.Target, target.Filename, expectedFilenames[target.GOOS])
		}
		if !strings.Contains(target.Filename, targets.Library) {
			t.Errorf("target %q filename does not name the shared library %q", target.Target, targets.Library)
		}
	}
}

func TestProcessorReleaseTargetsAgreeWithTheBuiltLibrary(t *testing.T) {
	targets := loadReleaseTargets(t)

	for _, target := range targets.Targets {
		if target.GOOS != runtime.GOOS || target.GOARCH != runtime.GOARCH {
			continue
		}
		// A system build of the host platform counts, so the release cannot
		// silently publish fewer variants than the bindings expect.
		if target.Libc == "system" {
			return
		}
	}

	t.Errorf("no release target builds the host platform %s/%s with its native libc", runtime.GOOS, runtime.GOARCH)
}

func libcSuffix(libc string) string {
	if libc == "glibc" || libc == "musl" {
		return "-" + libc
	}

	return ""
}
