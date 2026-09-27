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
	Zigarch  string `json:"zigarch"`
	Runner   string `json:"runner"`
	Filename string `json:"filename"`
}

type releaseTargets struct {
	ABIMajor int             `json:"abiMajor"`
	Library  string          `json:"library"`
	Targets  []releaseTarget `json:"targets"`
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
		"linux-amd64-musl",
		"linux-arm64-glibc",
		"linux-arm64-musl",
		"windows-amd64",
		"windows-arm64",
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

// Zig names architectures differently from Go, so a musl target must carry the
// spelling its cross compiler expects.
func TestMuslTargetsCarryTheZigArchitectureName(t *testing.T) {
	expected := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}

	for _, target := range loadReleaseTargets(t).Targets {
		if target.Libc != "musl" {
			continue
		}
		if target.Zigarch != expected[target.GOARCH] {
			t.Errorf("target %q zig architecture = %q, want %q", target.Target, target.Zigarch, expected[target.GOARCH])
		}
	}
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
		// A musl or system build of the host platform must still be listed so the
		// release cannot silently publish fewer variants than the bindings expect.
		if target.Libc == "musl" || target.Libc == "system" {
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
