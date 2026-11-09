package uv

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var updateGolden = flag.Bool("update", false, "update golden files with current parser output")

func TestGoldenFixtures(t *testing.T) {
	fixtureDir := filepath.Join("testdata")
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Skip("no testdata directory found")
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".input") {
			continue
		}
		baseName := strings.TrimSuffix(entry.Name(), ".input")
		t.Run(baseName, func(t *testing.T) {
			inputBytes, err := os.ReadFile(filepath.Join(fixtureDir, entry.Name()))
			if err != nil {
				t.Fatalf("failed to read fixture input: %v", err)
			}

			tools := parseUvToolList(string(inputBytes))
			goldenPath := filepath.Join(fixtureDir, baseName+".golden.json")

			if *updateGolden {
				data, err := json.MarshalIndent(tools, "", "  ")
				if err != nil {
					t.Fatalf("failed to marshal updates for golden file: %v", err)
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
				t.Fatalf("failed to unmarshal golden file %s: %v", goldenPath, err)
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
	}
}
