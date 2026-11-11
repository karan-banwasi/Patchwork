package pipx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/karan-banwasi/patchwork/internal/httputil"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

var (
	_ pm.Manager             = (*PipxManager)(nil)
	_ pm.Locator             = (*PipxManager)(nil)
	_ pm.WarningFetcher      = (*PipxManager)(nil)
	_ pm.OfflineConfigurable = (*PipxManager)(nil)
	_ pm.NetworkDependent    = (*PipxManager)(nil)
	_ pm.IOConfigurable      = (*PipxManager)(nil)
)

// PipxManager implements the pm.Manager interface for Python application binaries installed via pipx.
type PipxManager struct {
	runner  pm.Runner
	client  *http.Client
	offline bool
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewPipxManager() *PipxManager {
	return &PipxManager{
		runner: pm.ExecRunner{},
		client: httputil.NewClient(httputil.WithTimeout(10 * time.Second)),
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (p *PipxManager) Name() string {
	return "pipx"
}

func (p *PipxManager) Aliases() []string {
	return []string{"pipx-app"}
}

func (p *PipxManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	p.stdin = stdin
	if stdout != nil {
		p.stdout = stdout
	} else {
		p.stdout = io.Discard
	}
	if stderr != nil {
		p.stderr = stderr
	} else {
		p.stderr = io.Discard
	}
}

func (p *PipxManager) SetOffline(offline bool) {
	p.offline = offline
}

func (p *PipxManager) RequiresNetwork() bool {
	return true
}

func (p *PipxManager) SetHTTPClient(client *http.Client) {
	p.client = client
}

func (p *PipxManager) ValidateID(id string) error {
	return pm.ValidatePipxID(id)
}

func (p *PipxManager) Path() (string, error) {
	if p.exePath != "" {
		return p.exePath, nil
	}
	for _, name := range []string{"pipx.exe", "pipx"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("pipx executable not found")
}

func (p *PipxManager) Version(ctx context.Context) (string, error) {
	exe, err := p.Path()
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

func (p *PipxManager) IsAvailable(ctx context.Context) bool {
	_, err := p.Path()
	return err == nil
}

type pipxListOutput struct {
	PipxSpecVersion string                      `json:"pipx_spec_version"`
	Venvs           map[string]pipxVenvMetadata `json:"venvs"`
}

type pipxVenvMetadata struct {
	Metadata struct {
		MainPackage struct {
			Package        string `json:"package"`
			PackageVersion string `json:"package_version"`
		} `json:"main_package"`
	} `json:"metadata"`
}

func parsePipxListJSON(output string) ([]pm.PackageUpdate, error) {
	output = strings.TrimSpace(output)
	if output == "" || output == "{}" {
		return []pm.PackageUpdate{}, nil
	}

	var root pipxListOutput
	if err := json.Unmarshal([]byte(output), &root); err != nil {
		return nil, fmt.Errorf("failed to parse pipx list JSON: %w", err)
	}

	var packages []pm.PackageUpdate
	for venvName, venv := range root.Venvs {
		pkgName := venv.Metadata.MainPackage.Package
		if pkgName == "" {
			pkgName = venvName
		}
		ver := venv.Metadata.MainPackage.PackageVersion
		if pm.ValidatePipxID(pkgName) == nil && ver != "" {
			packages = append(packages, pm.PackageUpdate{
				Name:        pkgName,
				Id:          pkgName,
				Version:     ver,
				Source:      "pypi",
				ManagerName: "pipx",
			})
		}
	}

	sort.Slice(packages, func(i, j int) bool {
		return strings.ToLower(packages[i].Name) < strings.ToLower(packages[j].Name)
	})

	return packages, nil
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
		return "", fmt.Errorf("invalid json: %w", err)
	}
	return strings.TrimSpace(data.Info.Version), nil
}

func (p *PipxManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]pm.PackageUpdate, []error, error) {
	exe, err := p.Path()
	if err != nil {
		return nil, nil, err
	}
	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"list", "--json"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, nil, fmt.Errorf("pipx list failed: %w (%s)", err, errMsg)
		}
		return nil, nil, fmt.Errorf("pipx list failed: %w", err)
	}

	installed, err := parsePipxListJSON(stdout.String())
	if err != nil {
		return nil, nil, err
	}
	if len(installed) == 0 {
		return []pm.PackageUpdate{}, nil, nil
	}

	if p.offline {
		return []pm.PackageUpdate{}, []error{
			fmt.Errorf("[%s] offline mode: skipping PyPI remote checks", p.Name()),
		}, nil
	}

	client := p.client
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

			latest, err := fetchPyPILatestVersion(ctx, client, pu.Id)
			if err != nil {
				mu.Lock()
				lookupErrs = append(lookupErrs, fmt.Errorf("%s: %w", pu.Id, err))
				mu.Unlock()
				return
			}

			if latest != "" && version.IsUpgrade("pipx", pu.Version, latest) {
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
		warning := fmt.Errorf("[%s] %d of %d packages could not be checked; network error: %v", p.Name(), len(lookupErrs), len(installed), lookupErrs[0])
		return updates, []error{warning}, nil
	}

	return updates, nil, nil
}

func (p *PipxManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	updates, _, err := p.GetAvailableUpdatesWithWarnings(ctx)
	return updates, err
}

func (p *PipxManager) UpgradePackage(ctx context.Context, id string) error {
	if err := p.ValidateID(id); err != nil {
		return err
	}
	exe, err := p.Path()
	if err != nil {
		return err
	}
	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := p.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := p.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	if err := runner.Run(ctx, exe, []string{"upgrade", id}, p.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("pipx upgrade failed for %s: %w", id, err)
	}
	return nil
}
