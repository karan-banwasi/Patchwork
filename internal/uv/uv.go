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

	"github.com/karan-banwasi/patchwork/internal/httputil"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

var (
	_ pm.Manager             = (*UvManager)(nil)
	_ pm.WarningFetcher      = (*UvManager)(nil)
	_ pm.OfflineConfigurable = (*UvManager)(nil)
	_ pm.NetworkDependent    = (*UvManager)(nil)
)

// UvManager implements the pm.Manager interface for Python CLI tools managed by `uv tool`.
type UvManager struct {
	runner  pm.Runner
	client  *http.Client
	offline bool
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewUvManager() *UvManager {
	return &UvManager{
		runner: pm.ExecRunner{},
		client: httputil.NewClient(httputil.WithTimeout(10 * time.Second)),
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (u *UvManager) SetOffline(offline bool) {
	u.offline = offline
}

func (u *UvManager) RequiresNetwork() bool {
	return true
}

func (u *UvManager) SetHTTPClient(client *http.Client) {
	u.client = client
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

func fetchPyPILatestVersion(ctx context.Context, client *http.Client, pkgName string) (string, error) {
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", pkgName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Patchwork-CLI")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var data pypiResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("invalid response: %w", err)
	}
	return strings.TrimSpace(data.Info.Version), nil
}

// GetAvailableUpdatesWithWarnings queries PyPI and collects partial failure warnings.
func (u *UvManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]pm.PackageUpdate, []error, error) {
	exe, err := uvPath()
	if err != nil {
		return nil, nil, err
	}

	runner := u.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"tool", "list"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, nil, fmt.Errorf("failed to list uv tools: %w: %s", err, errMsg)
		}
		return nil, nil, fmt.Errorf("failed to list uv tools: %w", err)
	}

	tools := parseUvToolList(stdout.String())
	if len(tools) == 0 {
		return []pm.PackageUpdate{}, nil, nil
	}

	if u.offline {
		return []pm.PackageUpdate{}, []error{
			fmt.Errorf("[%s] offline mode: skipping PyPI remote checks", u.Name()),
		}, nil
	}

	client := u.client
	if client == nil {
		client = httputil.NewClient(httputil.WithTimeout(10 * time.Second))
	}
	var (
		updates    []pm.PackageUpdate
		lookupErrs []error
		mu         sync.Mutex
		wg         sync.WaitGroup
		sem        = make(chan struct{}, 6)
	)

	for _, tool := range tools {
		wg.Add(1)
		go func(t pm.PackageUpdate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			latest, err := fetchPyPILatestVersion(ctx, client, t.Id)
			if err != nil {
				mu.Lock()
				lookupErrs = append(lookupErrs, fmt.Errorf("%s: %w", t.Id, err))
				mu.Unlock()
				return
			}
			if latest != "" && version.IsUpgrade("uv", t.Version, latest) {
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

	if len(lookupErrs) > 0 {
		if len(lookupErrs) == len(tools) {
			return nil, nil, fmt.Errorf("failed to check any uv tools (0 of %d checked): %v", len(tools), lookupErrs[0])
		}
		warning := fmt.Errorf("[%s] %d of %d tools could not be checked; network error: %v", u.Name(), len(lookupErrs), len(tools), lookupErrs[0])
		return updates, []error{warning}, nil
	}

	return updates, nil, nil
}

func (u *UvManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	updates, _, err := u.GetAvailableUpdatesWithWarnings(ctx)
	return updates, err
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
