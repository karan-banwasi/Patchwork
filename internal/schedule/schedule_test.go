package schedule

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recordedCall struct {
	name  string
	args  []string
	stdin string
	xml   string // captured if /XML file existed during call
}

type mockRunner struct {
	calls []recordedCall
	fn    func(ctx context.Context, name string, args []string, stdin string, stdout, stderr io.Writer) error
}

func (m *mockRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	var inStr string
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		inStr = string(b)
	}
	call := recordedCall{
		name:  name,
		args:  append([]string(nil), args...),
		stdin: inStr,
	}
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "/XML" {
			if b, err := os.ReadFile(args[i+1]); err == nil {
				call.xml = string(b)
			}
		}
	}
	m.calls = append(m.calls, call)
	if m.fn != nil {
		return m.fn(ctx, name, args, inStr, stdout, stderr)
	}
	return nil
}

func TestStatePathAndPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "nested", "schedule.json")
	t.Setenv("PATCHWORK_SCHEDULE_STATE", customPath)

	if got := StatePath(); got != customPath {
		t.Fatalf("expected StatePath %q, got %q", customPath, got)
	}

	// Missing file returns zero state without error
	st, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState on missing file failed: %v", err)
	}
	if st.Enabled || st.Interval != "" || st.LastCount != 0 {
		t.Fatalf("expected zero state on missing file, got %+v", st)
	}

	now := time.Date(2026, 5, 1, 12, 30, 0, 0, time.UTC)
	want := &State{
		Enabled:   true,
		Interval:  "6h",
		Notify:    true,
		LastRun:   now,
		LastCount: 4,
		UpdatedAt: now,
	}
	if err := want.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if got.Enabled != want.Enabled || got.Interval != want.Interval || got.Notify != want.Notify || got.LastCount != want.LastCount || !got.LastRun.Equal(want.LastRun) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("loaded state mismatch: got %+v, want %+v", got, want)
	}

	// Corrupt JSON returns error
	if err := os.WriteFile(customPath, []byte("{bad json"), 0600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	if _, err := LoadState(); err == nil {
		t.Fatalf("expected error loading corrupt schedule state")
	}
}

func TestStatePathDefault(t *testing.T) {
	t.Setenv("PATCHWORK_SCHEDULE_STATE", "")
	p := StatePath()
	if !strings.HasSuffix(p, filepath.Join("Patchwork", "schedule.json")) && !strings.HasSuffix(p, "patchwork-schedule.json") {
		t.Errorf("unexpected default StatePath: %s", p)
	}
}

func TestParseInterval(t *testing.T) {
	tests := []struct {
		input   string
		wantDur time.Duration
		wantStr string
		wantErr bool
	}{
		{"", 6 * time.Hour, "6h", false},
		{"  ", 6 * time.Hour, "6h", false},
		{"hourly", time.Hour, "hourly", false},
		{"HOURLY", time.Hour, "hourly", false},
		{"daily", 24 * time.Hour, "daily", false},
		{"weekly", 168 * time.Hour, "weekly", false},
		{"15m", 15 * time.Minute, "15m", false},
		{"30m", 30 * time.Minute, "30m", false},
		{"2h30m", 150 * time.Minute, "2h30m", false},
		{"12h", 12 * time.Hour, "12h", false},
		{"14m", 0, "", true},
		{"14m59s", 0, "", true},
		{"0s", 0, "", true},
		{"-1h", 0, "", true},
		{"monthly", 0, "", true},
	}

	for _, tc := range tests {
		dur, norm, err := ParseInterval(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseInterval(%q): expected error, got (%s, %q)", tc.input, dur, norm)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseInterval(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if dur != tc.wantDur || norm != tc.wantStr {
			t.Errorf("ParseInterval(%q) = (%s, %q), want (%s, %q)", tc.input, dur, norm, tc.wantDur, tc.wantStr)
		}
	}
}

func TestGenerationHelpers(t *testing.T) {
	// Windows XML
	xmlOut, err := generateWindowsTaskXML(`C:\Program Files\Patchwork\pw.exe`, 6*time.Hour, true, time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("generateWindowsTaskXML failed: %v", err)
	}
	if err := xml.Unmarshal([]byte(xmlOut), new(any)); err != nil {
		t.Fatalf("generated Windows task XML is not valid XML: %v\n%s", err, xmlOut)
	}
	if !strings.Contains(xmlOut, "<Interval>PT6H</Interval>") {
		t.Errorf("expected PT6H interval in Windows XML:\n%s", xmlOut)
	}
	if !strings.Contains(xmlOut, `<Command>C:\Program Files\Patchwork\pw.exe</Command>`) {
		t.Errorf("expected Command in Windows XML:\n%s", xmlOut)
	}
	if !strings.Contains(xmlOut, `<Arguments>schedule run --quiet --notify</Arguments>`) {
		t.Errorf("expected Arguments in Windows XML:\n%s", xmlOut)
	}

	// macOS plist
	plistOut, err := GenerateLaunchdPlist("/usr/local/bin/pw & <test>", 24*time.Hour, false)
	if err != nil {
		t.Fatalf("GenerateLaunchdPlist failed: %v", err)
	}
	if !strings.Contains(plistOut, "<string>io.patchwork.check</string>") {
		t.Errorf("expected label in plist:\n%s", plistOut)
	}
	if !strings.Contains(plistOut, "<integer>86400</integer>") {
		t.Errorf("expected 86400s StartInterval in plist:\n%s", plistOut)
	}
	if !strings.Contains(plistOut, "<string>--notify=false</string>") {
		t.Errorf("expected --notify=false in plist:\n%s", plistOut)
	}
	if !strings.Contains(plistOut, "<string>/usr/local/bin/pw &amp; &lt;test&gt;</string>") {
		t.Errorf("expected XML-escaped binary path in plist:\n%s", plistOut)
	}

	// Linux systemd
	svc, err := GenerateSystemdService("/opt/my app/pw", true)
	if err != nil {
		t.Fatalf("GenerateSystemdService failed: %v", err)
	}
	if !strings.Contains(svc, `ExecStart="/opt/my app/pw" schedule run --quiet --notify`) {
		t.Errorf("unexpected systemd service content:\n%s", svc)
	}
	timer, err := GenerateSystemdTimer(15 * time.Minute)
	if err != nil {
		t.Fatalf("GenerateSystemdTimer failed: %v", err)
	}
	if !strings.Contains(timer, "OnUnitActiveSec=15m") {
		t.Errorf("unexpected systemd timer content:\n%s", timer)
	}

	// Crontab
	cronLine, err := GenerateCrontabEntry("/opt/my app/pw", 6*time.Hour, true)
	if err != nil {
		t.Fatalf("GenerateCrontabEntry failed: %v", err)
	}
	if !strings.HasPrefix(cronLine, "0 */6 * * * '/opt/my app/pw' schedule run --quiet --notify # patchwork-check") {
		t.Errorf("unexpected crontab entry: %q", cronLine)
	}
}

func TestSchedulerWindows(t *testing.T) {
	registered := false
	runner := &mockRunner{
		fn: func(ctx context.Context, name string, args []string, stdin string, stdout, stderr io.Writer) error {
			if name != "schtasks" {
				return errors.New("unexpected binary")
			}
			switch args[0] {
			case "/Create":
				registered = true
			case "/Query":
				if !registered {
					return errors.New("ERROR: The system cannot find the file specified.")
				}
			case "/Delete":
				registered = false
			}
			return nil
		},
	}

	sched := &Scheduler{
		Runner:     runner,
		GOOS:       "windows",
		BinaryPath: `C:\Tools\pw.exe`,
		Now:        func() time.Time { return time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC) },
	}

	ctx := context.Background()
	if reg, err := sched.IsRegistered(ctx); err != nil || reg {
		t.Fatalf("expected unregistered initially, got reg=%v err=%v", reg, err)
	}

	runner.calls = nil
	if err := sched.Enable(ctx, 6*time.Hour, true); err != nil {
		t.Fatalf("Enable windows failed: %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0].args[0] != "/Create" {
		t.Fatalf("expected schtasks /Create call, got %+v", runner.calls)
	}
	if !strings.Contains(runner.calls[0].xml, "<Interval>PT6H</Interval>") {
		t.Errorf("expected PT6H in schtasks XML, got:\n%s", runner.calls[0].xml)
	}

	if reg, err := sched.IsRegistered(ctx); err != nil || !reg {
		t.Fatalf("expected registered after Enable, got reg=%v err=%v", reg, err)
	}

	if err := sched.Disable(ctx); err != nil {
		t.Fatalf("Disable windows failed: %v", err)
	}
	if reg, err := sched.IsRegistered(ctx); err != nil || reg {
		t.Fatalf("expected unregistered after Disable, got reg=%v err=%v", reg, err)
	}
	// Idempotent Disable when already unregistered
	if err := sched.Disable(ctx); err != nil {
		t.Fatalf("second Disable windows should succeed cleanly: %v", err)
	}
}

func TestSchedulerDarwin(t *testing.T) {
	home := t.TempDir()
	loaded := false
	runner := &mockRunner{
		fn: func(ctx context.Context, name string, args []string, stdin string, stdout, stderr io.Writer) error {
			if name != "launchctl" {
				return errors.New("unexpected command")
			}
			switch args[0] {
			case "load":
				loaded = true
			case "unload":
				loaded = false
			case "list":
				if !loaded {
					return errors.New("could not find service")
				}
			}
			return nil
		},
	}

	sched := &Scheduler{
		Runner:     runner,
		GOOS:       "darwin",
		HomeDir:    home,
		BinaryPath: "/usr/local/bin/pw",
	}

	ctx := context.Background()
	if err := sched.Enable(ctx, 24*time.Hour, true); err != nil {
		t.Fatalf("Enable darwin failed: %v", err)
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", "io.patchwork.check.plist")
	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("expected plist at %s: %v", plistPath, err)
	}
	if !strings.Contains(string(data), "<integer>86400</integer>") {
		t.Errorf("expected 86400s interval in plist:\n%s", string(data))
	}

	if reg, err := sched.IsRegistered(ctx); err != nil || !reg {
		t.Fatalf("expected registered on darwin, got reg=%v err=%v", reg, err)
	}

	if err := sched.Disable(ctx); err != nil {
		t.Fatalf("Disable darwin failed: %v", err)
	}
	if _, err := os.Stat(plistPath); !os.IsNotExist(err) {
		t.Errorf("expected plist removed after Disable, stat err=%v", err)
	}
	if reg, err := sched.IsRegistered(ctx); err != nil || reg {
		t.Fatalf("expected unregistered after Disable, got reg=%v err=%v", reg, err)
	}
}

func TestSchedulerLinuxSystemd(t *testing.T) {
	home := t.TempDir()
	enabled := false
	runner := &mockRunner{
		fn: func(ctx context.Context, name string, args []string, stdin string, stdout, stderr io.Writer) error {
			if name == "crontab" {
				_, _ = stderr.Write([]byte("no crontab for user\n"))
				return errors.New("exit 1")
			}
			if name != "systemctl" {
				return errors.New("unexpected command")
			}
			if len(args) >= 2 && args[0] == "--user" {
				switch args[1] {
				case "show-environment", "daemon-reload":
					return nil
				case "enable":
					enabled = true
					return nil
				case "disable":
					enabled = false
					return nil
				case "is-enabled":
					if !enabled {
						return errors.New("disabled")
					}
					return nil
				}
			}
			return nil
		},
	}

	sched := &Scheduler{
		Runner:     runner,
		GOOS:       "linux",
		HomeDir:    home,
		BinaryPath: "/usr/bin/pw",
	}

	ctx := context.Background()
	if err := sched.Enable(ctx, time.Hour, false); err != nil {
		t.Fatalf("Enable linux systemd failed: %v", err)
	}

	unitDir := filepath.Join(home, ".config", "systemd", "user")
	svcBytes, err := os.ReadFile(filepath.Join(unitDir, "patchwork-check.service"))
	if err != nil {
		t.Fatalf("expected service unit: %v", err)
	}
	if !strings.Contains(string(svcBytes), "ExecStart=/usr/bin/pw schedule run --quiet --notify=false") {
		t.Errorf("unexpected service unit:\n%s", string(svcBytes))
	}
	timerBytes, err := os.ReadFile(filepath.Join(unitDir, "patchwork-check.timer"))
	if err != nil {
		t.Fatalf("expected timer unit: %v", err)
	}
	if !strings.Contains(string(timerBytes), "OnUnitActiveSec=1h") {
		t.Errorf("unexpected timer unit:\n%s", string(timerBytes))
	}

	if reg, err := sched.IsRegistered(ctx); err != nil || !reg {
		t.Fatalf("expected registered via systemd, got reg=%v err=%v", reg, err)
	}

	if err := sched.Disable(ctx); err != nil {
		t.Fatalf("Disable linux systemd failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unitDir, "patchwork-check.timer")); !os.IsNotExist(err) {
		t.Errorf("expected timer unit removed after Disable")
	}
	if reg, err := sched.IsRegistered(ctx); err != nil || reg {
		t.Fatalf("expected unregistered after Disable, got reg=%v err=%v", reg, err)
	}
}

func TestSchedulerLinuxCrontabFallback(t *testing.T) {
	home := t.TempDir()
	crontabContent := "0 3 * * * /usr/local/bin/backup.sh\n"
	runner := &mockRunner{
		fn: func(ctx context.Context, name string, args []string, stdin string, stdout, stderr io.Writer) error {
			if name == "systemctl" {
				return errors.New("System has not been booted with systemd as init system")
			}
			if name == "crontab" {
				if len(args) == 1 && args[0] == "-l" {
					_, _ = stdout.Write([]byte(crontabContent))
					return nil
				}
				if len(args) == 1 && args[0] == "-" {
					crontabContent = stdin
					return nil
				}
			}
			return errors.New("unexpected command")
		},
	}

	sched := &Scheduler{
		Runner:     runner,
		GOOS:       "linux",
		HomeDir:    home,
		BinaryPath: "/usr/local/bin/pw",
	}

	ctx := context.Background()
	if err := sched.Enable(ctx, 30*time.Minute, true); err != nil {
		t.Fatalf("Enable linux crontab failed: %v", err)
	}
	if !strings.Contains(crontabContent, "/usr/local/bin/backup.sh") {
		t.Errorf("existing crontab line was lost:\n%s", crontabContent)
	}
	if !strings.Contains(crontabContent, "*/30 * * * * /usr/local/bin/pw schedule run --quiet --notify # patchwork-check") {
		t.Errorf("expected patchwork crontab line:\n%s", crontabContent)
	}

	if reg, err := sched.IsRegistered(ctx); err != nil || !reg {
		t.Fatalf("expected registered via crontab, got reg=%v err=%v", reg, err)
	}

	if err := sched.Disable(ctx); err != nil {
		t.Fatalf("Disable linux crontab failed: %v", err)
	}
	if strings.Contains(crontabContent, "patchwork-check") {
		t.Errorf("expected patchwork crontab line removed:\n%s", crontabContent)
	}
	if !strings.Contains(crontabContent, "/usr/local/bin/backup.sh") {
		t.Errorf("existing crontab line should remain after Disable:\n%s", crontabContent)
	}
	if reg, err := sched.IsRegistered(ctx); err != nil || reg {
		t.Fatalf("expected unregistered after Disable, got reg=%v err=%v", reg, err)
	}
}

func TestSendNotificationAndEscaping(t *testing.T) {
	ctx := context.Background()
	evilTitle := "Patchwork'; Start-Process calc # \"$(whoami)\"\r\n"
	evilMsg := "Updates: `foo` 'bar' \"baz\" \\ --expire-time=0 $(id)\x00\nnext"

	// Windows
	winRunner := &mockRunner{}
	if err := SendNotification(ctx, winRunner, "windows", evilTitle, evilMsg); err != nil {
		t.Fatalf("windows SendNotification failed: %v", err)
	}
	if len(winRunner.calls) != 1 || winRunner.calls[0].name != "powershell" {
		t.Fatalf("expected powershell call, got %+v", winRunner.calls)
	}
	script := winRunner.calls[0].args[len(winRunner.calls[0].args)-1]
	// Single quotes inside PowerShell literal must be doubled ('') and no raw newline/NUL allowed.
	if strings.Contains(script, "\n") || strings.Contains(script, "\r") || strings.Contains(script, "\x00") {
		t.Errorf("powershell script contains unstripped control chars: %q", script)
	}
	if !strings.Contains(script, "$t = 'Patchwork''; Start-Process calc # \"$(whoami)\"'") {
		t.Errorf("expected safely single-quoted title in PowerShell script: %s", script)
	}
	if !strings.Contains(script, "''bar''") {
		t.Errorf("expected doubled single quotes for message in PowerShell script: %s", script)
	}

	// macOS
	macRunner := &mockRunner{}
	if err := SendNotification(ctx, macRunner, "darwin", evilTitle, evilMsg); err != nil {
		t.Fatalf("darwin SendNotification failed: %v", err)
	}
	if len(macRunner.calls) != 1 || macRunner.calls[0].name != "osascript" {
		t.Fatalf("expected osascript call, got %+v", macRunner.calls)
	}
	asScript := macRunner.calls[0].args[1]
	if strings.Contains(asScript, "\n") || strings.Contains(asScript, "\x00") {
		t.Errorf("osascript contains control chars: %q", asScript)
	}
	if !strings.Contains(asScript, `\"baz\" \\`) {
		t.Errorf("expected escaped quotes and backslash in osascript: %s", asScript)
	}

	// Linux
	linuxRunner := &mockRunner{}
	if err := SendNotification(ctx, linuxRunner, "linux", "--malicious-flag", evilMsg); err != nil {
		t.Fatalf("linux SendNotification failed: %v", err)
	}
	if len(linuxRunner.calls) != 1 || linuxRunner.calls[0].name != "notify-send" {
		t.Fatalf("expected notify-send call, got %+v", linuxRunner.calls)
	}
	lArgs := linuxRunner.calls[0].args
	if len(lArgs) != 4 || lArgs[1] != "--" || lArgs[2] != "--malicious-flag" {
		t.Fatalf("expected '--' option terminator before title/message, got %v", lArgs)
	}

	// Empty message and unsupported OS
	if err := SendNotification(ctx, linuxRunner, "linux", "Title", "   \n "); err == nil {
		t.Errorf("expected error for empty message")
	}
	if err := SendNotification(ctx, linuxRunner, "plan9", "Title", "Msg"); err == nil {
		t.Errorf("expected error for unsupported OS")
	}
}
