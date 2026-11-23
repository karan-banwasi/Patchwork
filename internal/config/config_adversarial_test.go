package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConfig_Adversarial_AtomicWriteConcurrency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Pre-seed config
	initial := &Config{PinnedPackages: []string{"initial.pkg"}}
	if err := initial.SaveTo(path); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	const numWriters = 30
	const numReaders = 30
	const iterations = 40

	var stopReaders int32
	var readErrors int64
	var corruptReads int64
	var validReads int64

	var wg sync.WaitGroup

	// Readers continuously read and unmarshal
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(readerId int) {
			defer wg.Done()
			for atomic.LoadInt32(&stopReaders) == 0 {
				cfg, err := LoadFrom(path)
				if err != nil {
					// On Windows, if a rename is in progress, sharing violation could occur,
					// but it must NEVER return a partially written corrupted JSON!
					atomic.AddInt64(&readErrors, 1)
					continue
				}
				if cfg == nil || len(cfg.PinnedPackages) == 0 {
					atomic.AddInt64(&corruptReads, 1)
					t.Errorf("reader %d saw empty or nil config: %+v", readerId, cfg)
				} else {
					atomic.AddInt64(&validReads, 1)
				}
			}
		}(r)
	}

	// Writers concurrently save new configurations
	var writeErrors int64
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writerId int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				cfg := &Config{
					PinnedPackages: []string{
						fmt.Sprintf("pkg.writer.%d.iter.%d", writerId, i),
						"common.pkg",
					},
				}
				if err := cfg.SaveTo(path); err != nil {
					atomic.AddInt64(&writeErrors, 1)
				}
			}
		}(w)
	}

	// Wait for all writers to complete
	// We wait for writers first, then stop readers

	// Let writers and readers run
	// To cleanly wait for writers:
	var writerWg sync.WaitGroup
	for w := 0; w < numWriters; w++ {
		writerWg.Add(1)
		go func(writerId int) {
			defer writerWg.Done()
			for i := 0; i < iterations; i++ {
				cfg := &Config{
					PinnedPackages: []string{
						fmt.Sprintf("writer-%d-%d", writerId, i),
						"constant.pkg",
					},
				}
				_ = cfg.SaveTo(path)
			}
		}(w)
	}
	writerWg.Wait()
	atomic.StoreInt32(&stopReaders, 1)
	wg.Wait()

	t.Logf("Concurrency stats: validReads=%d, readErrors=%d, corruptReads=%d, writeErrors=%d",
		atomic.LoadInt64(&validReads),
		atomic.LoadInt64(&readErrors),
		atomic.LoadInt64(&corruptReads),
		atomic.LoadInt64(&writeErrors))

	if atomic.LoadInt64(&corruptReads) > 0 {
		t.Fatalf("FAIL: readers encountered corrupted/truncated config: %d times", atomic.LoadInt64(&corruptReads))
	}

	// Verify final state is valid and loadable
	finalCfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("final config is unreadable: %v", err)
	}
	if finalCfg == nil || len(finalCfg.PinnedPackages) == 0 {
		t.Fatalf("final config is empty or nil: %+v", finalCfg)
	}

	// Verify no tmp files were leaked
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "config-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("found leaked temporary file after concurrent writes: %s", e.Name())
		}
	}
}

func TestConfig_Adversarial_CorruptInputs(t *testing.T) {
	testCases := []struct {
		name    string
		content []byte
	}{
		{"empty file", []byte("")},
		{"truncated array", []byte(`{"pinned_packages": ["git"`)},
		{"truncated object", []byte(`{"pinned_packages": `)},
		{"raw garbage", []byte{0x00, 0xFF, 0xFE, 0x12, 0x34}},
		{"invalid type for pinned_packages", []byte(`{"pinned_packages": 12345}`)},
		{"invalid type for accept_agreements", []byte(`{"accept_agreements": "yes"}`)},
		{"malformed JSON syntax", []byte(`{pinned_packages: [foo]}`)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "config.json")
			if err := os.WriteFile(path, tc.content, 0600); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			cfg, err := LoadFrom(path)
			if err == nil {
				t.Fatalf("expected LoadFrom to fail for %s, but returned nil error and cfg: %+v", tc.name, cfg)
			}
			if cfg != nil {
				t.Errorf("expected nil config on error for %s, got: %+v", tc.name, cfg)
			}
		})
	}
}
