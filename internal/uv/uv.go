package uv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// UvManager implements the pm.Manager interface for Python CLI tools managed by `uv tool`.
type UvManager struct{}

func NewUvManager() *UvManager {
	return &UvManager{}
}

func (u *UvManager) Name() string {
	return "uv"
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
		if len(m) == 3 {
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

	cmd := exec.CommandContext(ctx, exe, "tool", "list")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
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
	exe, err := uvPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, "tool", "upgrade", id)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("uv tool upgrade failed for %s: %w", id, err)
	}
	return nil
}

func (u *UvManager) UpgradeAll(ctx context.Context) error {
	updates, err := u.GetAvailableUpdates(ctx)
	if err != nil {
		return err
	}
	for _, tool := range updates {
		if err := u.UpgradePackage(ctx, tool.Id); err != nil {
			return err
		}
	}
	return nil
}
