package dotnet

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var updateGolden = flag.Bool("update", false, "update golden files with current parser output")

func TestGoldenFixtures(t *testing.T) {
	fixtureDir := filepath.Join("testdata")

	t.Run("tool_list", func(t *testing.T) {
		inputBytes, err := os.ReadFile(filepath.Join(fixtureDir, "tool_list.input"))
		if err != nil {
			t.Fatalf("failed to read fixture input: %v", err)
		}

		tools := parseDotnetToolList(string(inputBytes))
		goldenPath := filepath.Join(fixtureDir, "tool_list.golden.json")

		if *updateGolden {
			data, err := json.MarshalIndent(tools, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal golden json: %v", err)
			}
			if err := os.WriteFile(goldenPath, append(data, '\n'), 0644); err != nil {
				t.Fatalf("failed to write golden file: %v", err)
			}
			return
		}

		goldenBytes, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("golden file not found: %s (run with -update to generate)", goldenPath)
		}

		var expected []pm.PackageUpdate
		if err := json.Unmarshal(goldenBytes, &expected); err != nil {
			t.Fatalf("failed to unmarshal golden file: %v", err)
		}

		if len(tools) != len(expected) {
			t.Fatalf("count mismatch: got %d, expected %d", len(tools), len(expected))
		}
		for i := range tools {
			if tools[i].Id != expected[i].Id || tools[i].Version != expected[i].Version {
				t.Errorf("mismatch at [%d]: got %+v, expected %+v", i, tools[i], expected[i])
			}
		}
	})

	t.Run("tool_search", func(t *testing.T) {
		inputBytes, err := os.ReadFile(filepath.Join(fixtureDir, "tool_search.input"))
		if err != nil {
			t.Fatalf("failed to read fixture input: %v", err)
		}

		latest := parseDotnetToolSearchLatest(string(inputBytes), "dotnet-ef")
		goldenPath := filepath.Join(fixtureDir, "tool_search.golden.json")

		if *updateGolden {
			data, err := json.MarshalIndent(map[string]string{"latest": latest}, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal golden json: %v", err)
			}
			if err := os.WriteFile(goldenPath, append(data, '\n'), 0644); err != nil {
				t.Fatalf("failed to write golden file: %v", err)
			}
			return
		}

		goldenBytes, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("golden file not found: %s (run with -update to generate)", goldenPath)
		}

		var expected map[string]string
		if err := json.Unmarshal(goldenBytes, &expected); err != nil {
			t.Fatalf("failed to unmarshal golden file: %v", err)
		}

		if latest != expected["latest"] {
			t.Errorf("mismatch: got %q, expected %q", latest, expected["latest"])
		}
	})
}
