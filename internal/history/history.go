package history

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SchemaVersion is the current schema version for history JSONL entries.
const SchemaVersion = "1.0.0"

var fileMu sync.Mutex

// Item represents the outcome of a single package within an upgrade run.
type Item struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	Manager     string `json:"manager"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
	Status      string `json:"status"`
	DurationMs  int64  `json:"duration_ms"`
	Error       string `json:"error,omitempty"`
}

// Entry represents a single recorded `pw upgrade` execution in history.jsonl.
type Entry struct {
	SchemaVersion string    `json:"schema_version"`
	RunID         string    `json:"run_id"`
	Timestamp     time.Time `json:"timestamp"`
	ManagerFilter string    `json:"manager_filter,omitempty"`
	Items         []Item    `json:"items"`
	Succeeded     int       `json:"succeeded"`
	Failed        int       `json:"failed"`
	Skipped       int       `json:"skipped"`
}

// HistoryPath returns the path to the upgrade history JSONL file.
// It honors the PATCHWORK_HISTORY environment variable override and otherwise
// defaults to <os.UserConfigDir()>/Patchwork/history.jsonl.
func HistoryPath() string {
	if custom := strings.TrimSpace(os.Getenv("PATCHWORK_HISTORY")); custom != "" {
		return custom
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "Patchwork", "history.jsonl")
	}
	return filepath.Join(os.TempDir(), "patchwork-history.jsonl")
}

// NewRunID generates a unique, chronologically sortable run identifier.
func NewRunID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102T150405Z"), time.Now().UnixNano()%100000)
	}
	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102T150405Z"), hex.EncodeToString(b[:]))
}

// Append writes entry as a single JSON line to HistoryPath(), creating the parent
// directory with 0700 permissions and the file with 0600 permissions if needed.
func Append(entry Entry) error {
	if entry.SchemaVersion == "" {
		entry.SchemaVersion = SchemaVersion
	}
	if entry.RunID == "" {
		entry.RunID = NewRunID()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal history entry: %w", err)
	}
	line := append(data, '\n')

	path := HistoryPath()
	dir := filepath.Dir(path)

	fileMu.Lock()
	defer fileMu.Unlock()

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create history directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	if err := os.Chmod(path, 0600); err != nil {
		// Non-fatal on Windows
		_ = err
	}

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("write history entry: %w", err)
	}
	return nil
}

// LoadAll reads all valid history entries from HistoryPath() in chronological order.
// Returns an empty slice and nil error if the history file does not exist.
// Blank lines and malformed JSON lines are skipped gracefully.
func LoadAll() ([]Entry, error) {
	path := HistoryPath()

	fileMu.Lock()
	defer fileMu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	// Allow lines up to 4 MiB for large bulk upgrade runs.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			// Skip malformed lines gracefully so one corrupt record does not discard history.
			continue
		}
		if e.RunID == "" && e.Timestamp.IsZero() && len(e.Items) == 0 {
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read history file: %w", err)
	}
	if entries == nil {
		entries = []Entry{}
	}
	return entries, nil
}

// Clear deletes the history file if it exists.
func Clear() error {
	path := HistoryPath()

	fileMu.Lock()
	defer fileMu.Unlock()

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear history file: %w", err)
	}
	return nil
}

// FindLatestForPackage searches history from newest to oldest for the most recent
// successful upgrade of id (matched case-insensitively against Item.ID or Item.Name).
// When manager is non-empty, only items from that manager are considered.
// Returns (nil, nil, nil) if no matching successful upgrade is found.
func FindLatestForPackage(manager, id string) (*Item, *Entry, error) {
	targetID := strings.TrimSpace(id)
	targetMgr := strings.TrimSpace(manager)
	if targetID == "" {
		return nil, nil, nil
	}

	entries, err := LoadAll()
	if err != nil {
		return nil, nil, err
	}

	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		// Prefer exact ID match before Name fallback within the same run.
		for _, matchByName := range []bool{false, true} {
			for j := range entry.Items {
				item := entry.Items[j]
				if !strings.EqualFold(item.Status, "Succeeded") {
					continue
				}
				if targetMgr != "" && !strings.EqualFold(item.Manager, targetMgr) {
					continue
				}
				matched := strings.EqualFold(item.ID, targetID)
				if !matched && matchByName && item.Name != "" {
					matched = strings.EqualFold(item.Name, targetID)
				}
				if matched {
					itemCopy := item
					entryCopy := entry
					return &itemCopy, &entryCopy, nil
				}
			}
		}
	}

	return nil, nil, nil
}

// FindEntry looks up a history entry by its RunID (case-insensitive).
// Returns (nil, nil) if no matching entry exists.
func FindEntry(runID string) (*Entry, error) {
	target := strings.TrimSpace(runID)
	if target == "" {
		return nil, nil
	}

	entries, err := LoadAll()
	if err != nil {
		return nil, err
	}

	for i := len(entries) - 1; i >= 0; i-- {
		if strings.EqualFold(entries[i].RunID, target) {
			entryCopy := entries[i]
			return &entryCopy, nil
		}
	}

	return nil, nil
}
