package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/karan-banwasi/patchwork/internal/httputil"
	"github.com/karan-banwasi/patchwork/internal/pm"
)

type slowHangingNetworkManager struct {
	name         string
	offlineCalls atomic.Int32
	refreshCalls atomic.Int32
	fetchCalls   atomic.Int32
	upgradeCalls atomic.Int32
	offline      bool
}

func (m *slowHangingNetworkManager) Name() string                         { return m.name }
func (m *slowHangingNetworkManager) Aliases() []string                    { return nil }
func (m *slowHangingNetworkManager) IsAvailable(ctx context.Context) bool { return true }
func (m *slowHangingNetworkManager) RequiresNetwork() bool                { return true }
func (m *slowHangingNetworkManager) SetOffline(offline bool) {
	m.offline = offline
	m.offlineCalls.Add(1)
}
func (m *slowHangingNetworkManager) Refresh(ctx context.Context) error {
	m.refreshCalls.Add(1)
	if !m.offline {
		// Would simulate hanging if offline wasn't honored
		time.Sleep(10 * time.Second)
	}
	return nil
}
func (m *slowHangingNetworkManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	m.fetchCalls.Add(1)
	if !m.offline {
		time.Sleep(10 * time.Second)
	}
	if m.offline {
		return nil, httputil.ErrOffline
	}
	return []pm.PackageUpdate{
		{Id: "net.pkg", Name: "NetPkg", Version: "1.0.0", AvailableVersion: "2.0.0", ManagerName: m.name},
	}, nil
}
func (m *slowHangingNetworkManager) UpgradePackage(ctx context.Context, id string) error {
	m.upgradeCalls.Add(1)
	if m.offline {
		return httputil.ErrOffline
	}
	return nil
}
func (m *slowHangingNetworkManager) ValidateID(id string) error { return nil }

func TestAdversarialOfflineMode(t *testing.T) {
	t.Run("check --offline fast skips network managers without hanging", func(t *testing.T) {
		localMgr := &mockCheckManager{
			name: "localpm",
			updates: []pm.PackageUpdate{
				{Id: "local.tool", Name: "LocalTool", Version: "1.0.0", AvailableVersion: "2.0.0", ManagerName: "localpm"},
			},
		}
		netMgr := &slowHangingNetworkManager{name: "netpm"}

		reg := pm.NewRegistry(localMgr, netMgr)
		registryOverride = reg
		t.Cleanup(func() { registryOverride = nil })

		var stdout, stderr bytes.Buffer
		cmd := NewRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"check", "--offline"})

		start := time.Now()
		err := cmd.Execute()
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("expected check --offline to succeed, got: %v", err)
		}

		if elapsed > 2*time.Second {
			t.Fatalf("check --offline took too long (%s), expected instant skip without hangs", elapsed)
		}

		if netMgr.fetchCalls.Load() != 0 {
			t.Errorf("expected 0 fetch calls to netpm, got %d", netMgr.fetchCalls.Load())
		}
		if netMgr.refreshCalls.Load() != 0 {
			t.Errorf("expected 0 refresh calls to netpm, got %d", netMgr.refreshCalls.Load())
		}

		errOut := stderr.String()
		if !strings.Contains(errOut, "[netpm] offline mode: skipping remote package update checks") {
			t.Errorf("expected offline skip warning in stderr, got: %s", errOut)
		}

		stdOut := stdout.String()
		if !strings.Contains(stdOut, "local.tool") {
			t.Errorf("expected local.tool in stdout, got: %s", stdOut)
		}
	})

	t.Run("check --offline --json outputs valid schema without network managers", func(t *testing.T) {
		localMgr := &mockCheckManager{
			name: "localpm",
			updates: []pm.PackageUpdate{
				{Id: "local.tool", Name: "LocalTool", Version: "1.0.0", AvailableVersion: "2.0.0", ManagerName: "localpm"},
			},
		}
		netMgr := &slowHangingNetworkManager{name: "netpm"}

		reg := pm.NewRegistry(localMgr, netMgr)
		registryOverride = reg
		t.Cleanup(func() { registryOverride = nil })

		var stdout, stderr bytes.Buffer
		cmd := NewRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"check", "--offline", "--json"})

		start := time.Now()
		err := cmd.Execute()
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("expected check --offline --json to succeed, got: %v", err)
		}

		if elapsed > 2*time.Second {
			t.Fatalf("check --offline --json took too long (%s)", elapsed)
		}

		var parsed struct {
			SchemaVersion string `json:"schema_version"`
			Count         int    `json:"count"`
			Updates       []struct {
				ID      string `json:"id"`
				Manager string `json:"manager"`
			} `json:"updates"`
		}

		if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
			t.Fatalf("failed to parse json output: %v (output: %s)", err, stdout.String())
		}

		if parsed.SchemaVersion != "1.0.0" {
			t.Errorf("expected schema_version '1.0.0', got %q", parsed.SchemaVersion)
		}
		if parsed.Count != 1 || len(parsed.Updates) != 1 || parsed.Updates[0].ID != "local.tool" {
			t.Errorf("unexpected updates in json: %+v", parsed)
		}
	})

	t.Run("upgrade --offline --all executes local upgrades and skips network managers", func(t *testing.T) {
		localMgr := &mockUpgradeManager{
			name: "localpm",
			updates: []pm.PackageUpdate{
				{Id: "local.tool", Name: "LocalTool", Version: "1.0.0", AvailableVersion: "2.0.0", ManagerName: "localpm"},
			},
		}
		netMgr := &slowHangingNetworkManager{name: "netpm"}

		reg := pm.NewRegistry(localMgr, netMgr)
		registryOverride = reg
		t.Cleanup(func() { registryOverride = nil })

		var stdout, stderr bytes.Buffer
		cmd := NewRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"upgrade", "--offline", "--all"})

		start := time.Now()
		err := cmd.Execute()
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("expected upgrade --offline --all to succeed, got: %v", err)
		}

		if elapsed > 2*time.Second {
			t.Fatalf("upgrade --offline --all took too long (%s)", elapsed)
		}

		if netMgr.fetchCalls.Load() != 0 {
			t.Errorf("expected 0 fetch calls to netpm, got %d", netMgr.fetchCalls.Load())
		}
		if len(localMgr.upgradedIDs) != 1 || localMgr.upgradedIDs[0] != "local.tool" {
			t.Errorf("expected local.tool to be upgraded, got: %v", localMgr.upgradedIDs)
		}

		stdOut := stdout.String()
		if !strings.Contains(stdOut, "=== Upgrade Summary ===") {
			t.Errorf("expected upgrade summary in stdout, got: %s", stdOut)
		}
		if !strings.Contains(stdOut, "Total: 1 | Succeeded: 1 | Failed: 0 | Skipped: 0") {
			t.Errorf("expected summary with 1 succeeded, got: %s", stdOut)
		}
	})

	t.Run("upgrade --offline explicit package with network manager fast fails", func(t *testing.T) {
		netMgr := &slowHangingNetworkManager{name: "netpm"}
		reg := pm.NewRegistry(netMgr)
		registryOverride = reg
		t.Cleanup(func() { registryOverride = nil })

		var stdout, stderr bytes.Buffer
		cmd := NewRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"upgrade", "--offline", "net.pkg"})

		start := time.Now()
		err := cmd.Execute()
		elapsed := time.Since(start)

		if elapsed > 2*time.Second {
			t.Fatalf("upgrade --offline explicit package took too long (%s)", elapsed)
		}

		// When net.pkg upgrade fails due to ErrOffline, the command should return an error or ExitUpgradeFailure
		if err == nil && !strings.Contains(stderr.String(), "Error upgrading net.pkg") {
			t.Fatalf("expected upgrade failure or error for offline network package, err: %v, stderr: %s", err, stderr.String())
		}
	})
}
