package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
)

// SchemaVersion is the current machine-readable profile schema version.
const SchemaVersion = "1.0.0"

// ProfilePackage represents a single package entry in an exported or imported profile.
type ProfilePackage struct {
	Manager string `json:"manager"`
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// Profile represents a portable machine package profile including installed packages and pins.
type Profile struct {
	SchemaVersion string            `json:"schema_version"`
	CreatedAt     time.Time         `json:"created_at"`
	Packages      []ProfilePackage  `json:"packages"`
	Pins          []config.PinEntry `json:"pins,omitempty"`
}

// FormatPatchfile serializes a Profile into the Brewfile-style Patchfile DSL.
func FormatPatchfile(p Profile) string {
	var b strings.Builder
	b.WriteString("# Patchwork machine profile\n")
	if !p.CreatedAt.IsZero() {
		fmt.Fprintf(&b, "# Generated: %s\n", p.CreatedAt.UTC().Format(time.RFC3339))
	}

	for _, pkg := range p.Packages {
		mgr := strings.ToLower(strings.TrimSpace(pkg.Manager))
		if mgr == "" || pkg.ID == "" {
			continue
		}
		b.WriteString(mgr)
		b.WriteByte(' ')
		b.WriteString(strconv.Quote(pkg.ID))
		if pkg.Version != "" {
			b.WriteString(" version=")
			b.WriteString(strconv.Quote(pkg.Version))
		}
		b.WriteByte('\n')
	}

	if len(p.Pins) > 0 && len(p.Packages) > 0 {
		b.WriteByte('\n')
	}

	for _, pin := range p.Pins {
		target := pin.ID
		if pin.Manager != "" {
			target = strings.ToLower(strings.TrimSpace(pin.Manager)) + ":" + pin.ID
		}
		if target == "" {
			continue
		}
		b.WriteString("pin ")
		b.WriteString(strconv.Quote(target))
		if pin.Version != "" {
			b.WriteString(" version=")
			b.WriteString(strconv.Quote(pin.Version))
		}
		if pin.Reason != "" {
			b.WriteString(" reason=")
			b.WriteString(strconv.Quote(pin.Reason))
		}
		if pin.ExpiresAt != nil {
			b.WriteString(" expires=")
			b.WriteString(strconv.Quote(pin.ExpiresAt.UTC().Format(time.RFC3339)))
		}
		b.WriteByte('\n')
	}

	return b.String()
}

// MarshalJSON serializes a Profile as indented JSON without HTML escaping.
func MarshalJSON(p Profile) ([]byte, error) {
	if p.SchemaVersion == "" {
		p.SchemaVersion = SchemaVersion
	}
	if p.Packages == nil {
		p.Packages = []ProfilePackage{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ParsePatchfile parses either JSON (when trimmed input starts with '{') or the Patchfile DSL.
func ParsePatchfile(data []byte) (Profile, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return Profile{
			SchemaVersion: SchemaVersion,
			Packages:      []ProfilePackage{},
		}, nil
	}
	if trimmed[0] == '{' {
		return parseProfileJSON(trimmed)
	}
	return parseProfileDSL(string(data))
}

func parseProfileJSON(data []byte) (Profile, error) {
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return Profile{}, fmt.Errorf("invalid profile JSON: %w", err)
	}
	if p.SchemaVersion != "" && p.SchemaVersion != SchemaVersion {
		return Profile{}, fmt.Errorf("unsupported profile schema_version %q (expected %q)", p.SchemaVersion, SchemaVersion)
	}
	if p.SchemaVersion == "" {
		p.SchemaVersion = SchemaVersion
	}
	for i := range p.Packages {
		p.Packages[i].Manager = strings.ToLower(strings.TrimSpace(p.Packages[i].Manager))
		p.Packages[i].ID = strings.TrimSpace(p.Packages[i].ID)
		if p.Packages[i].Manager == "" || p.Packages[i].ID == "" {
			return Profile{}, fmt.Errorf("profile package at index %d is missing manager or id", i)
		}
	}
	for i := range p.Pins {
		normalizeParsedPin(&p.Pins[i])
		if p.Pins[i].ID == "" {
			return Profile{}, fmt.Errorf("profile pin at index %d is missing id", i)
		}
	}
	if p.Packages == nil {
		p.Packages = []ProfilePackage{}
	}
	return p, nil
}

func normalizeParsedPin(pin *config.PinEntry) {
	pin.Manager = strings.ToLower(strings.TrimSpace(pin.Manager))
	pin.ID = strings.TrimSpace(pin.ID)
	if pin.Manager == "" {
		if idx := strings.IndexByte(pin.ID, ':'); idx > 0 && idx < len(pin.ID)-1 {
			pin.Manager = strings.ToLower(strings.TrimSpace(pin.ID[:idx]))
			pin.ID = strings.TrimSpace(pin.ID[idx+1:])
		}
	}
}

func parseProfileDSL(input string) (Profile, error) {
	p := Profile{
		SchemaVersion: SchemaVersion,
		Packages:      []ProfilePackage{},
	}

	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	for idx, rawLine := range lines {
		lineNum := idx + 1
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if err := parseDSLLine(trimmed, lineNum, &p); err != nil {
			return Profile{}, err
		}
	}

	return p, nil
}

func parseDSLLine(line string, lineNum int, p *Profile) error {
	pos := 0
	n := len(line)

	skipSpaces := func() {
		for pos < n && (line[pos] == ' ' || line[pos] == '\t') {
			pos++
		}
	}

	readWord := func() string {
		start := pos
		for pos < n && line[pos] != ' ' && line[pos] != '\t' && line[pos] != '=' && line[pos] != '#' {
			pos++
		}
		return line[start:pos]
	}

	readValue := func() (string, error) {
		if pos >= n {
			return "", fmt.Errorf("line %d: expected value", lineNum)
		}
		if line[pos] == '"' {
			return readQuotedString(line, &pos, lineNum)
		}
		start := pos
		for pos < n && line[pos] != ' ' && line[pos] != '\t' && line[pos] != '#' {
			pos++
		}
		val := line[start:pos]
		if val == "" {
			return "", fmt.Errorf("line %d: empty value", lineNum)
		}
		return val, nil
	}

	skipSpaces()
	if pos >= n || line[pos] == '#' {
		return nil
	}

	directive := strings.ToLower(readWord())
	if directive == "" {
		return fmt.Errorf("line %d: expected directive or manager name", lineNum)
	}

	skipSpaces()
	if pos >= n || line[pos] == '#' {
		return fmt.Errorf("line %d: expected package or pin identifier after %q", lineNum, directive)
	}

	target, err := readValue()
	if err != nil {
		return err
	}
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("line %d: empty identifier for %q", lineNum, directive)
	}

	attrs := make(map[string]string)
	for {
		skipSpaces()
		if pos >= n || line[pos] == '#' {
			break
		}
		key := strings.ToLower(readWord())
		if key == "" {
			return fmt.Errorf("line %d: unexpected character %q", lineNum, line[pos])
		}
		if pos >= n || line[pos] != '=' {
			return fmt.Errorf("line %d: expected '=' after attribute %q", lineNum, key)
		}
		pos++ // consume '='
		val, err := readValue()
		if err != nil {
			return err
		}
		attrs[key] = val
	}

	if directive == "pin" {
		pin := config.PinEntry{
			ID:      target,
			Version: attrs["version"],
			Reason:  attrs["reason"],
		}
		for k := range attrs {
			switch k {
			case "version", "reason", "expires", "expires_at":
			default:
				return fmt.Errorf("line %d: unknown pin attribute %q", lineNum, k)
			}
		}
		expStr := attrs["expires"]
		if expStr == "" {
			expStr = attrs["expires_at"]
		}
		if expStr != "" {
			exp, err := parseProfileExpiry(expStr)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNum, err)
			}
			pin.ExpiresAt = exp
		}
		normalizeParsedPin(&pin)
		if pin.ID == "" {
			return fmt.Errorf("line %d: pin identifier cannot be empty", lineNum)
		}
		p.Pins = append(p.Pins, pin)
		return nil
	}

	for k := range attrs {
		switch k {
		case "version", "name":
		default:
			return fmt.Errorf("line %d: unknown package attribute %q", lineNum, k)
		}
	}

	p.Packages = append(p.Packages, ProfilePackage{
		Manager: directive,
		ID:      target,
		Name:    attrs["name"],
		Version: attrs["version"],
	})
	return nil
}

func readQuotedString(line string, pos *int, lineNum int) (string, error) {
	start := *pos
	if start >= len(line) || line[start] != '"' {
		return "", fmt.Errorf("line %d: expected '\"'", lineNum)
	}
	i := start + 1
	escaped := false
	for i < len(line) {
		c := line[i]
		if escaped {
			escaped = false
			i++
			continue
		}
		if c == '\\' {
			escaped = true
			i++
			continue
		}
		if c == '"' {
			raw := line[start : i+1]
			unquoted, err := strconv.Unquote(raw)
			if err != nil {
				return "", fmt.Errorf("line %d: invalid quoted string: %w", lineNum, err)
			}
			*pos = i + 1
			return unquoted, nil
		}
		i++
	}
	return "", fmt.Errorf("line %d: unterminated quoted string", lineNum)
}

func parseProfileExpiry(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		utc := t.UTC()
		return &utc, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		utc := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.UTC)
		return &utc, nil
	}
	return nil, fmt.Errorf("invalid expires timestamp %q (expected RFC3339 or YYYY-MM-DD)", s)
}

// WriteAtomic writes data to path atomically via a temporary file in the same directory.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, ".patchwork-profile-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	closed = true

	_ = os.Chmod(tmpName, perm)
	return os.Rename(tmpName, path)
}
