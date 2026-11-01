package uv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// UvManager implements the pm.Manager interface for Python CLI tools managed by `uv tool`.
type UvManager struct {
	runner pm.Runner
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func NewUvManager() *UvManager {
	return &UvManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (u *UvManager) Name() string {
	return "uv"
}

func (u *UvManager) Aliases() []string {
	return []string{"uv-tool"}
}

func (u *UvManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	u.stdin = stdin
	if stdout != nil {
		u.stdout = stdout
	} else {
		u.stdout = io.Discard
	}
	if stderr != nil {
		u.stderr = stderr
	} else {
		u.stderr = io.Discard
	}
}

func (u *UvManager) ValidateID(id string) error {
	return pm.ValidateUvID(id)
}

func (u *UvManager) Path() (string, error) {
	return uvPath()
}

func (u *UvManager) Version(ctx context.Context) (string, error) {
	exe, err := uvPath()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown", err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "uv "), nil
}

func (u *UvManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := uvPath()
	if err != nil {
		return nil
	}
	fmt.Fprintln(out, "--- Pruning uv cache ---")
	runner := u.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	return runner.Run(ctx, exe, []string{"cache", "prune"}, nil, out, errOut)
}

func (u *UvManager) IsAvailable(ctx context.Context) bool {
	_, err := uvPath()
	return err == nil
}

func uvPath() (string, error) {
	for _, name := range []string{"uv.exe", "uv"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("uv executable not found")
}

var uvToolLineRegex = regexp.MustCompile(`^([A-Za-z0-9_.\-]+)\s+v([0-9][^\s]*)$`)

func parseUvToolList(output string) []pm.PackageUpdate {
	var tools []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") {
			continue
		}
		m := uvToolLineRegex.FindStringSubmatch(trimmed)
		if len(m) == 3 && pm.ValidateUvID(m[1]) == nil {
			tools = append(tools, pm.PackageUpdate{
				Name:        m[1],
				Id:          m[1],
				Version:     m[2],
				Source:      "pypi",
				ManagerName: "uv",
			})
		}
	}
	return tools
}

type pypiResponse struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
}

func fetchPyPILatestVersion(ctx context.Context, client *http.Client, pkgName string) string {
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", pkgName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Patchwork-CLI")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var data pypiResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.Info.Version)
}

func (u *UvManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := uvPath()
	if err != nil {
		return nil, err
	}

	runner := u.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"tool", "list"}, nil, &stdout, io.Discard); err != nil {
		return []pm.PackageUpdate{}, nil
	}

	tools := parseUvToolList(stdout.String())
	if len(tools) == 0 {
		return []pm.PackageUpdate{}, nil
	}

	client := &http.Client{Timeout: 8 * time.Second}
	var (
		updates []pm.PackageUpdate
		mu      sync.Mutex
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 6)
	)

	for _, tool := range tools {
		wg.Add(1)
		go func(t pm.PackageUpdate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			latest := fetchPyPILatestVersion(ctx, client, t.Id)
			if latest != "" && latest != t.Version {
				t.AvailableVersion = latest
				mu.Lock()
				updates = append(updates, t)
				mu.Unlock()
			}
		}(tool)
	}
	wg.Wait()

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})
	return updates, nil
}

func (u *UvManager) UpgradePackage(ctx context.Context, id string) error {
	if err := u.ValidateID(id); err != nil {
		return err
	}
	exe, err := uvPath()
	if err != nil {
		return err
	}
	runner := u.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := u.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := u.stderr
	if stderr == nil {
		stderr = io.Discard
	}
	if err := runner.Run(ctx, exe, []string{"tool", "upgrade", id}, u.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("uv tool upgrade failed for %s: %w", id, err)
	}
	return nil
}
