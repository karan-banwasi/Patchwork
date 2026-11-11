package vscode

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
	_ pm.Manager             = (*VSCodeManager)(nil)
	_ pm.Locator             = (*VSCodeManager)(nil)
	_ pm.BulkUpgrader        = (*VSCodeManager)(nil)
	_ pm.WarningFetcher      = (*VSCodeManager)(nil)
	_ pm.OfflineConfigurable = (*VSCodeManager)(nil)
	_ pm.NetworkDependent    = (*VSCodeManager)(nil)
	_ pm.IOConfigurable      = (*VSCodeManager)(nil)
)

// VSCodeManager implements the pm.Manager interface for Visual Studio Code extensions.
type VSCodeManager struct {
	runner  pm.Runner
	client  *http.Client
	offline bool
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewVSCodeManager() *VSCodeManager {
	return &VSCodeManager{
		runner: pm.ExecRunner{},
		client: httputil.NewClient(httputil.WithTimeout(10 * time.Second)),
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (v *VSCodeManager) Name() string {
	return "vscode"
}

func (v *VSCodeManager) Aliases() []string {
	return []string{"code", "vs-code"}
}

func (v *VSCodeManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	v.stdin = stdin
	if stdout != nil {
		v.stdout = stdout
	} else {
		v.stdout = io.Discard
	}
	if stderr != nil {
		v.stderr = stderr
	} else {
		v.stderr = io.Discard
	}
}

func (v *VSCodeManager) SetOffline(offline bool) {
	v.offline = offline
}

func (v *VSCodeManager) RequiresNetwork() bool {
	return true
}

func (v *VSCodeManager) SetHTTPClient(client *http.Client) {
	v.client = client
}

func (v *VSCodeManager) ValidateID(id string) error {
	return pm.ValidateVSCodeID(id)
}

func (v *VSCodeManager) Path() (string, error) {
	if v.exePath != "" {
		return v.exePath, nil
	}
	for _, name := range []string{"code.cmd", "code.exe", "code", "code-insiders.cmd", "code-insiders", "codium"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("vscode (code) executable not found")
}

func (v *VSCodeManager) Version(ctx context.Context) (string, error) {
	exe, err := v.Path()
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

func (v *VSCodeManager) IsAvailable(ctx context.Context) bool {
	_, err := v.Path()
	return err == nil
}

func (v *VSCodeManager) UpgradePackage(ctx context.Context, id string) error {
	return v.UpgradePackages(ctx, []string{id})
}

func (v *VSCodeManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := v.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := v.Path()
	if err != nil {
		return err
	}
	runner := v.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := v.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := v.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	var args []string
	for _, id := range ids {
		args = append(args, "--install-extension", id, "--force")
	}

	if err := runner.Run(ctx, exe, args, v.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("code install extension failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

var vscodeLineRegex = regexp.MustCompile(`^([a-zA-Z0-9.\-_]+\.[a-zA-Z0-9.\-_]+)@([0-9][^\s]*)$`)

func parseVSCodeListExtensions(output string) ([]pm.PackageUpdate, error) {
	var extensions []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		m := vscodeLineRegex.FindStringSubmatch(line)
		if len(m) == 3 {
			id := strings.ToLower(m[1])
			ver := m[2]
			if pm.ValidateVSCodeID(id) == nil {
				extensions = append(extensions, pm.PackageUpdate{
					Name:        id,
					Id:          id,
					Version:     ver,
					Source:      "marketplace",
					ManagerName: "vscode",
				})
			}
		}
	}

	sort.Slice(extensions, func(i, j int) bool {
		return strings.ToLower(extensions[i].Name) < strings.ToLower(extensions[j].Name)
	})

	return extensions, nil
}

type marketplaceQueryRequest struct {
	Filters []marketplaceFilter `json:"filters"`
	Flags   int                 `json:"flags"`
}

type marketplaceFilter struct {
	Criteria []marketplaceCriteria `json:"criteria"`
}

type marketplaceCriteria struct {
	FilterType int    `json:"filterType"`
	Value      string `json:"value"`
}

type marketplaceResponse struct {
	Results []struct {
		Extensions []struct {
			Versions []struct {
				Version string `json:"version"`
			} `json:"versions"`
		} `json:"extensions"`
	} `json:"results"`
}

func fetchMarketplaceLatestVersion(ctx context.Context, client *http.Client, extID string) (string, error) {
	endpoint := "https://marketplace.visualstudio.com/_apis/public/gallery/extensionquery"
	query := marketplaceQueryRequest{
		Filters: []marketplaceFilter{
			{
				Criteria: []marketplaceCriteria{
					{FilterType: 7, Value: extID},
				},
			},
		},
		Flags: 103,
	}

	jsonBytes, err := json.Marshal(query)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(jsonBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json;api-version=3.0-preview.1")
	req.Header.Set("User-Agent", "Patchwork-CLI")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var data marketplaceResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("invalid response json: %w", err)
	}

	if len(data.Results) > 0 && len(data.Results[0].Extensions) > 0 {
		ext := data.Results[0].Extensions[0]
		if len(ext.Versions) > 0 {
			return strings.TrimSpace(ext.Versions[0].Version), nil
		}
	}

	return "", fmt.Errorf("extension not found in marketplace")
}

func (v *VSCodeManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]pm.PackageUpdate, []error, error) {
	exe, err := v.Path()
	if err != nil {
		return nil, nil, err
	}
	runner := v.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"--list-extensions", "--show-versions"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, nil, fmt.Errorf("code list extensions failed: %w (%s)", err, errMsg)
		}
		return nil, nil, fmt.Errorf("code list extensions failed: %w", err)
	}

	installed, err := parseVSCodeListExtensions(stdout.String())
	if err != nil {
		return nil, nil, err
	}
	if len(installed) == 0 {
		return []pm.PackageUpdate{}, nil, nil
	}

	if v.offline {
		return []pm.PackageUpdate{}, []error{
			fmt.Errorf("[%s] offline mode: skipping VS Code Marketplace remote checks", v.Name()),
		}, nil
	}

	client := v.client
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

	for _, ext := range installed {
		wg.Add(1)
		go func(pu pm.PackageUpdate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			latest, err := fetchMarketplaceLatestVersion(ctx, client, pu.Id)
			if err != nil {
				mu.Lock()
				lookupErrs = append(lookupErrs, fmt.Errorf("%s: %w", pu.Id, err))
				mu.Unlock()
				return
			}

			if latest != "" && version.IsUpgrade("vscode", pu.Version, latest) {
				pu.AvailableVersion = latest
				mu.Lock()
				updates = append(updates, pu)
				mu.Unlock()
			}
		}(ext)
	}
	wg.Wait()

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	if len(lookupErrs) > 0 {
		if len(lookupErrs) == len(installed) {
			return nil, nil, fmt.Errorf("failed to check any extensions (0 of %d checked): %v", len(installed), lookupErrs[0])
		}
		warning := fmt.Errorf("[%s] %d of %d extensions could not be checked; network error: %v", v.Name(), len(lookupErrs), len(installed), lookupErrs[0])
		return updates, []error{warning}, nil
	}

	return updates, nil, nil
}

func (v *VSCodeManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	updates, _, err := v.GetAvailableUpdatesWithWarnings(ctx)
	return updates, err
}
