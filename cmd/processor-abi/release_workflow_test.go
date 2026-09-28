package main

import (
	"os"
	"strings"
	"testing"
)

func TestProcessorReleaseOpensAVerifiedBindingsUpdate(t *testing.T) {
	contents, err := os.ReadFile("../../.github/workflows/release-processor.yml")
	if err != nil {
		t.Fatalf("unable to read processor release workflow: %v", err)
	}
	workflow := string(contents)

	required := []string{
		"update-bindings:",
		"needs: [publish]",
		"ref: ${{ needs.targets.outputs.tag }}",
		"git rev-parse \"refs/tags/$tag^{commit}\"",
		"repository: louiss0/mace-bindings",
		"tools/native/processor.json",
		"sha256sum processor-manifest.json",
		"node tools/native/stage-release.mjs",
		"gh pr create",
	}
	for _, fragment := range required {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("release-processor.yml must contain %q", fragment)
		}
	}
}

func TestBindingsUpdateKeepsMuslOutOfThePublishedMatrix(t *testing.T) {
	targets := loadReleaseTargets(t)
	for _, target := range targets.Targets {
		if target.Libc == "musl" {
			t.Fatalf("bindings automation must not receive unresolved musl target %q", target.Target)
		}
	}
}
