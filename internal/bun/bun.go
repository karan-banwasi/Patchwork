package bun

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
	_ pm.Manager             = (*BunManager)(nil)
	_ pm.Locator             = (*BunManager)(nil)
	_ pm.Cleaner             = (*BunManager)(nil)
	_ pm.BulkUpgrader        = (*BunManager)(nil)
	_ pm.WarningFetcher      = (*BunManager)(nil)
	_ pm.OfflineConfigurable = (*BunManager)(nil)
	_ pm.NetworkDependent    = (*BunManager)(nil)
	_ pm.IOConfigurable      = (*BunManager)(nil)
)

// BunManager implements the pm.Manager interface for bun global packages.
type BunManager struct {
	runner  pm.Runner
	client  *http.Client
	offline bool
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewBunManager() *BunManager {
	return &BunManager{
		runner: pm.ExecRunner{},
		client: httputil.NewClient(httputil.WithTimeout(10 * time.Second)),
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (b *BunManager) Name() string {
	return "bun"
}

func (b *BunManager) Aliases() []string {
	return []string{"bun-global"}
}

func (b *BunManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	b.stdin = stdin
	if stdout != nil {
		b.stdout = stdout
	} else {
		b.stdout = io.Discard
	}
	if stderr != nil {
		b.stderr = stderr
	} else {
		b.stderr = io.Discard
	}
}

func (b *BunManager) SetOffline(offline bool) {
	b.offline = offline
}

func (b *BunManager) RequiresNetwork() bool {
	return true
}

func (b *BunManager) SetHTTPClient(client *http.Client) {
	b.client = client
}

func (b *BunManager) ValidateID(id string) error {
	return pm.ValidateNpmID(id)
}

func (b *BunManager) Path() (string, error) {
	if b.exePath != "" {
		return b.exePath, nil
	}
	for _, name := range []string{"bun.exe", "bun"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("bun executable not found")
}

func (b *BunManager) Version(ctx context.Context) (string, error) {
	exe, err := b.Path()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}
	return "unknown", nil
}

func (b *BunManager) IsAvailable(ctx context.Context) bool {
	_, err := b.Path()
	return err == nil
}

func (b *BunManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := b.Path()
	if err != nil {
		return nil
	}
	runner := b.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	fmt.Fprintln(out, "--- Removing bun package cache ---")
	return runner.Run(ctx, exe, []string{"pm", "cache", "rm"}, b.stdin, out, errOut)
}

func (b *BunManager) UpgradePackage(ctx context.Context, id string) error {
	return b.UpgradePackages(ctx, []string{id})
}

func (b *BunManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := b.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := b.Path()
	if err != nil {
		return err
	}
	runner := b.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := b.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := b.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	var installTargets []string
	for _, id := range ids {
		installTargets = append(installTargets, id+"@latest")
	}

	args := append([]string{"add", "-g"}, installTargets...)
	if err := runner.Run(ctx, exe, args, b.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("bun add -g failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

var bunLineRegex = regexp.MustCompile(`(?:[├└]──\s+|^\s*)([a-zA-Z0-9@_./\-]+)@([0-9][^\s]*)`)

func parseBunGlobalList(output string) ([]pm.PackageUpdate, error) {
	var pkgs []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r\n")
		if line == "" || strings.HasPrefix(line, "/") || strings.Contains(line, "global") {
			continue
		}

		m := bunLineRegex.FindStringSubmatch(line)
		if len(m) == 3 {
			id := m[1]
			ver := m[2]
			if pm.ValidateNpmID(id) == nil {
				pkgs = append(pkgs, pm.PackageUpdate{
					Name:        id,
					Id:          id,
					Version:     ver,
					Source:      "npm",
					ManagerName: "bun",
				})
			}
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].Name) < strings.ToLower(pkgs[j].Name)
	})

	return pkgs, nil
}

type npmLatestResponse struct {
	Version string `json:"version"`
}

func fetchNpmLatestVersion(ctx context.Context, client *http.Client, pkgName string) (string, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/%s/latest", pkgName)
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
	var data npmLatestResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("invalid json: %w", err)
	}
	return strings.TrimSpace(data.Version), nil
}

func (b *BunManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]pm.PackageUpdate, []error, error) {
	exe, err := b.Path()
	if err != nil {
		return nil, nil, err
	}
	runner := b.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"pm", "ls", "-g"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, nil, fmt.Errorf("bun pm ls -g failed: %w (%s)", err, errMsg)
		}
		return nil, nil, fmt.Errorf("bun pm ls -g failed: %w", err)
	}

	installed, err := parseBunGlobalList(stdout.String())
	if err != nil {
		return nil, nil, err
	}
	if len(installed) == 0 {
		return []pm.PackageUpdate{}, nil, nil
	}

	if b.offline {
		return []pm.PackageUpdate{}, []error{
			fmt.Errorf("[%s] offline mode: skipping npm registry remote checks", b.Name()),
		}, nil
	}

	client := b.client
	if client == nil {
		client = httputil.NewClient(httputil.WithTimeout(10 * time.Second))
	}

	var (
		updates    []pm.PackageUpdate
		lookupErrs []error
		mu         sync.Mutex
		wg         sync.WaitGroup
		sem        = make(chan struct{}, 4)
	)

	for _, pkg := range installed {
		wg.Add(1)
		go func(pu pm.PackageUpdate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			latest, err := fetchNpmLatestVersion(ctx, client, pu.Id)
			if err != nil {
				mu.Lock()
				lookupErrs = append(lookupErrs, fmt.Errorf("%s: %w", pu.Id, err))
				mu.Unlock()
				return
			}

			if latest != "" && version.IsUpgrade("bun", pu.Version, latest) {
				pu.AvailableVersion = latest
				mu.Lock()
				updates = append(updates, pu)
				mu.Unlock()
			}
		}(pkg)
	}
	wg.Wait()

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	if len(lookupErrs) > 0 {
		if len(lookupErrs) == len(installed) {
			return nil, nil, fmt.Errorf("failed to check any packages (0 of %d checked): %v", len(installed), lookupErrs[0])
		}
		warning := fmt.Errorf("[%s] %d of %d packages could not be checked; network error: %v", b.Name(), len(lookupErrs), len(installed), lookupErrs[0])
		return updates, []error{warning}, nil
	}

	return updates, nil, nil
}

func (b *BunManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	updates, _, err := b.GetAvailableUpdatesWithWarnings(ctx)
	return updates, err
}
