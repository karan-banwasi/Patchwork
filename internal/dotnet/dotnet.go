package dotnet

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

var (
	_ pm.Manager             = (*DotnetManager)(nil)
	_ pm.WarningFetcher      = (*DotnetManager)(nil)
	_ pm.OfflineConfigurable = (*DotnetManager)(nil)
	_ pm.NetworkDependent    = (*DotnetManager)(nil)
)

// DotnetManager implements the pm.Manager interface for global .NET tools.
type DotnetManager struct {
	runner  pm.Runner
	offline bool
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewDotnetManager() *DotnetManager {
	return &DotnetManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (d *DotnetManager) SetOffline(offline bool) {
	d.offline = offline
}

func (d *DotnetManager) RequiresNetwork() bool {
	return true
}

func (d *DotnetManager) Name() string {
	return "dotnet"
}

func (d *DotnetManager) Aliases() []string {
	return []string{"dotnet-tool"}
}

func (d *DotnetManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	d.stdin = stdin
	if stdout != nil {
		d.stdout = stdout
	} else {
		d.stdout = io.Discard
	}
	if stderr != nil {
		d.stderr = stderr
	} else {
		d.stderr = io.Discard
	}
}

func (d *DotnetManager) ValidateID(id string) error {
	return pm.ValidateDotnetID(id)
}

func (d *DotnetManager) Path() (string, error) {
	return dotnetPath()
}

func (d *DotnetManager) Version(ctx context.Context) (string, error) {
	exe, err := dotnetPath()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (d *DotnetManager) IsAvailable(ctx context.Context) bool {
	_, err := dotnetPath()
	return err == nil
}

func dotnetPath() (string, error) {
	for _, name := range []string{"dotnet.exe", "dotnet"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("dotnet executable not found")
}

// GetAvailableUpdatesWithWarnings lists global .NET tools, searches NuGet, and collects warnings.
func (d *DotnetManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]pm.PackageUpdate, []error, error) {
	exe, err := dotnetPath()
	if err != nil {
		return nil, nil, err
	}

	runner := d.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"tool", "list", "-g"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, nil, fmt.Errorf("failed to list global dotnet tools: %w: %s", err, errMsg)
		}
		return nil, nil, fmt.Errorf("failed to list global dotnet tools: %w", err)
	}

	tools := parseDotnetToolList(stdout.String())
	if len(tools) == 0 {
		return []pm.PackageUpdate{}, nil, nil
	}

	if d.offline {
		return []pm.PackageUpdate{}, []error{
			fmt.Errorf("[%s] offline mode: skipping NuGet tool search", d.Name()),
		}, nil
	}

	var (
		updates    []pm.PackageUpdate
		lookupErrs []error
		mu         sync.Mutex
		wg         sync.WaitGroup
		sem        = make(chan struct{}, 4)
	)

	for _, tool := range tools {
		wg.Add(1)
		go func(t pm.PackageUpdate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var sOut, sErr bytes.Buffer
			if err := runner.Run(ctx, exe, []string{"tool", "search", t.Id, "--take", "5"}, nil, &sOut, &sErr); err != nil {
				errMsg := strings.TrimSpace(sErr.String())
				mu.Lock()
				if errMsg != "" {
					lookupErrs = append(lookupErrs, fmt.Errorf("%s: %w: %s", t.Id, err, errMsg))
				} else {
					lookupErrs = append(lookupErrs, fmt.Errorf("%s: %w", t.Id, err))
				}
				mu.Unlock()
				return
			}
			latest := parseDotnetToolSearchLatest(sOut.String(), t.Id)
			if latest != "" && version.IsUpgrade("dotnet", t.Version, latest) {
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
			return nil, nil, fmt.Errorf("failed to check any dotnet tools (0 of %d checked): %v", len(tools), lookupErrs[0])
		}
		warning := fmt.Errorf("[%s] %d of %d tools could not be checked; lookup error: %v", d.Name(), len(lookupErrs), len(tools), lookupErrs[0])
		return updates, []error{warning}, nil
	}

	return updates, nil, nil
}

// GetAvailableUpdates lists global .NET tools and checks NuGet for newer stable versions.
func (d *DotnetManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	updates, _, err := d.GetAvailableUpdatesWithWarnings(ctx)
	return updates, err
}

func parseDotnetToolList(output string) []pm.PackageUpdate {
	var tools []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	headerFound := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "----") {
			headerFound = true
			continue
		}
		if !headerFound {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && pm.ValidateDotnetID(fields[0]) == nil {
			tools = append(tools, pm.PackageUpdate{
				Name:        fields[0],
				Id:          fields[0],
				Version:     fields[1],
				Source:      "dotnet",
				ManagerName: "dotnet",
			})
		}
	}
	return tools
}

func parseDotnetToolSearchLatest(output, targetID string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "---") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && strings.EqualFold(fields[0], targetID) {
			return fields[1]
		}
	}
	return ""
}

func (d *DotnetManager) UpgradePackage(ctx context.Context, id string) error {
	if err := d.ValidateID(id); err != nil {
		return err
	}
	exe, err := dotnetPath()
	if err != nil {
		return err
	}
	runner := d.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := d.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := d.stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return runner.Run(ctx, exe, []string{"tool", "update", "-g", id}, d.stdin, stdout, stderr)
}
