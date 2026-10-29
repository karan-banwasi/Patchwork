package dotnet

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// DotnetManager implements the pm.Manager interface for global .NET tools.
type DotnetManager struct{}

func NewDotnetManager() *DotnetManager {
	return &DotnetManager{}
}

func (d *DotnetManager) Name() string {
	return "dotnet"
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

// GetAvailableUpdates lists global .NET tools and checks NuGet for newer stable versions.
func (d *DotnetManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := dotnetPath()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, exe, "tool", "list", "-g")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return []pm.PackageUpdate{}, nil
	}

	tools := parseDotnetToolList(stdout.String())
	if len(tools) == 0 {
		return []pm.PackageUpdate{}, nil
	}

	var updates []pm.PackageUpdate
	for _, tool := range tools {
		searchCmd := exec.CommandContext(ctx, exe, "tool", "search", tool.Id, "--take", "1")
		var sOut bytes.Buffer
		searchCmd.Stdout = &sOut
		if err := searchCmd.Run(); err != nil {
			continue
		}
		latest := parseDotnetToolSearchLatest(sOut.String(), tool.Id)
		if latest != "" && latest != tool.Version {
			tool.AvailableVersion = latest
			updates = append(updates, tool)
		}
	}
	return updates, nil
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
		if len(fields) >= 2 {
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
	exe, err := dotnetPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, "tool", "update", "-g", id)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (d *DotnetManager) UpgradeAll(ctx context.Context) error {
	exe, err := dotnetPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, "tool", "update", "-g", "--all")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
