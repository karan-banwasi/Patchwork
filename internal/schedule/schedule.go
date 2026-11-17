package schedule

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

const (
	// DefaultInterval is the default check interval used when none is specified.
	DefaultInterval = "6h"

	// MinInterval is the minimum allowed check interval.
	MinInterval = 15 * time.Minute

	windowsTaskName    = "PatchworkCheck"
	launchdLabel       = "io.patchwork.check"
	systemdServiceName = "patchwork-check.service"
	systemdTimerName   = "patchwork-check.timer"
	crontabMarker      = "# patchwork-check"
)

// State represents the persisted state of Patchwork's scheduled background checks.
type State struct {
	Enabled   bool      `json:"enabled"`
	Interval  string    `json:"interval"`
	Notify    bool      `json:"notify"`
	LastRun   time.Time `json:"last_run,omitempty"`
	LastCount int       `json:"last_count"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// StatePath returns the path to the schedule state JSON file.
// Honors the PATCHWORK_SCHEDULE_STATE environment variable override,
// defaulting to <os.UserConfigDir()>/Patchwork/schedule.json.
func StatePath() string {
	if custom := strings.TrimSpace(os.Getenv("PATCHWORK_SCHEDULE_STATE")); custom != "" {
		return custom
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "Patchwork", "schedule.json")
	}
	return filepath.Join(os.TempDir(), "patchwork-schedule.json")
}

// LoadState reads the schedule state from StatePath().
// Returns a zero-value State and nil error if the file does not exist.
func LoadState() (*State, error) {
	return LoadStateFrom(StatePath())
}

// LoadStateFrom reads the schedule state from the given path.
// Returns a zero-value State and nil error if the file does not exist.
func LoadStateFrom(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{}, nil
		}
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("invalid schedule state %s: %w", path, err)
	}
	return &st, nil
}

// Save atomically persists the schedule state to StatePath() with 0700/0600 permissions.
func (s *State) Save() error {
	return s.SaveTo(StatePath())
}

// SaveTo atomically persists the schedule state to the given path with 0700/0600 permissions.
func (s *State) SaveTo(path string) error {
	if s == nil {
		return errors.New("schedule state is nil")
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, data, 0600)
}

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "schedule-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	closed = true

	_ = os.Chmod(tmpName, perm)
	return os.Rename(tmpName, path)
}

// ParseInterval parses an interval specification ("hourly", "daily", "weekly",
// empty string for default "6h", or a Go duration string >= 15m).
// Returns the parsed duration, normalized interval label, and any validation error.
func ParseInterval(s string) (time.Duration, string, error) {
	norm := strings.ToLower(strings.TrimSpace(s))
	switch norm {
	case "":
		return 6 * time.Hour, DefaultInterval, nil
	case "hourly":
		return time.Hour, "hourly", nil
	case "daily":
		return 24 * time.Hour, "daily", nil
	case "weekly":
		return 7 * 24 * time.Hour, "weekly", nil
	}

	d, err := time.ParseDuration(norm)
	if err != nil {
		return 0, "", fmt.Errorf("invalid interval %q: expected hourly, daily, weekly, or duration >= %s", s, MinInterval)
	}
	if d < MinInterval {
		return 0, "", fmt.Errorf("interval %q must be at least %s", s, MinInterval)
	}
	return d, norm, nil
}

// Scheduler manages user-level OS scheduler registration across Windows, macOS, and Linux.
type Scheduler struct {
	Runner     pm.Runner
	GOOS       string
	HomeDir    string
	ConfigDir  string
	BinaryPath string
	Now        func() time.Time
}

func (s *Scheduler) runner() pm.Runner {
	if s != nil && s.Runner != nil {
		return s.Runner
	}
	return pm.ExecRunner{}
}

func (s *Scheduler) goos() string {
	if s != nil && s.GOOS != "" {
		return s.GOOS
	}
	return runtime.GOOS
}

func (s *Scheduler) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Scheduler) homeDir() (string, error) {
	if s != nil && s.HomeDir != "" {
		return s.HomeDir, nil
	}
	return os.UserHomeDir()
}

func (s *Scheduler) binaryPath(goos string) string {
	if s != nil && s.BinaryPath != "" {
		return s.BinaryPath
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		return exe
	}
	if goos == "windows" {
		return "pw.exe"
	}
	return "pw"
}

func (s *Scheduler) launchdPlistPath() (string, error) {
	home, err := s.homeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

func (s *Scheduler) systemdUserDir() (string, error) {
	if s != nil && s.ConfigDir != "" {
		return filepath.Join(s.ConfigDir, "systemd", "user"), nil
	}
	if s != nil && s.HomeDir != "" {
		return filepath.Join(s.HomeDir, ".config", "systemd", "user"), nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "systemd", "user"), nil
	}
	home, err := s.homeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

// Enable registers the periodic background check with the host OS user-level scheduler.
func Enable(ctx context.Context, runner pm.Runner, goos string, interval time.Duration, notify bool) error {
	return (&Scheduler{Runner: runner, GOOS: goos}).Enable(ctx, interval, notify)
}

// Disable unregisters the periodic background check from the host OS user-level scheduler.
func Disable(ctx context.Context, runner pm.Runner, goos string) error {
	return (&Scheduler{Runner: runner, GOOS: goos}).Disable(ctx)
}

// IsRegistered reports whether the periodic background check is currently registered with the OS scheduler.
func IsRegistered(ctx context.Context, runner pm.Runner, goos string) (bool, error) {
	return (&Scheduler{Runner: runner, GOOS: goos}).IsRegistered(ctx)
}

// Enable registers the periodic background check for the configured OS.
func (s *Scheduler) Enable(ctx context.Context, interval time.Duration, notify bool) error {
	if interval < MinInterval {
		return fmt.Errorf("interval %s must be at least %s", interval, MinInterval)
	}
	goos := s.goos()
	bin := s.binaryPath(goos)

	switch goos {
	case "windows":
		return s.enableWindows(ctx, bin, interval, notify)
	case "darwin":
		return s.enableDarwin(ctx, bin, interval, notify)
	case "linux":
		return s.enableLinux(ctx, bin, interval, notify)
	default:
		return fmt.Errorf("scheduled background checks are not supported on %s", goos)
	}
}

// Disable removes the periodic background check for the configured OS.
func (s *Scheduler) Disable(ctx context.Context) error {
	goos := s.goos()
	switch goos {
	case "windows":
		return s.disableWindows(ctx)
	case "darwin":
		return s.disableDarwin(ctx)
	case "linux":
		return s.disableLinux(ctx)
	default:
		return fmt.Errorf("scheduled background checks are not supported on %s", goos)
	}
}

// IsRegistered reports whether the periodic background check is active in the configured OS scheduler.
func (s *Scheduler) IsRegistered(ctx context.Context) (bool, error) {
	goos := s.goos()
	switch goos {
	case "windows":
		return s.isRegisteredWindows(ctx)
	case "darwin":
		return s.isRegisteredDarwin(ctx)
	case "linux":
		return s.isRegisteredLinux(ctx)
	default:
		return false, fmt.Errorf("scheduled background checks are not supported on %s", goos)
	}
}

func (s *Scheduler) enableWindows(ctx context.Context, bin string, interval time.Duration, notify bool) error {
	xmlContent, err := generateWindowsTaskXML(bin, interval, notify, s.now())
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "patchwork-task-*.xml")
	if err != nil {
		return fmt.Errorf("failed to create temporary task XML: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.WriteString(xmlContent); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write task XML: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close task XML: %w", err)
	}

	var stderr bytes.Buffer
	args := []string{"/Create", "/TN", windowsTaskName, "/XML", tmpPath, "/F"}
	if err := s.runner().Run(ctx, "schtasks", args, nil, &bytes.Buffer{}, &stderr); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("schtasks /Create failed: %w (%s)", err, msg)
		}
		return fmt.Errorf("schtasks /Create failed: %w", err)
	}
	return nil
}

func (s *Scheduler) disableWindows(ctx context.Context) error {
	reg, err := s.isRegisteredWindows(ctx)
	if err == nil && !reg {
		return nil
	}
	var stderr bytes.Buffer
	args := []string{"/Delete", "/TN", windowsTaskName, "/F"}
	if err := s.runner().Run(ctx, "schtasks", args, nil, &bytes.Buffer{}, &stderr); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(strings.ToLower(msg), "cannot find") || strings.Contains(strings.ToLower(msg), "does not exist") {
			return nil
		}
		if msg != "" {
			return fmt.Errorf("schtasks /Delete failed: %w (%s)", err, msg)
		}
		return fmt.Errorf("schtasks /Delete failed: %w", err)
	}
	return nil
}

func (s *Scheduler) isRegisteredWindows(ctx context.Context) (bool, error) {
	var stdout, stderr bytes.Buffer
	err := s.runner().Run(ctx, "schtasks", []string{"/Query", "/TN", windowsTaskName}, nil, &stdout, &stderr)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (s *Scheduler) enableDarwin(ctx context.Context, bin string, interval time.Duration, notify bool) error {
	plistPath, err := s.launchdPlistPath()
	if err != nil {
		return err
	}
	plistContent, err := GenerateLaunchdPlist(bin, interval, notify)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}
	if err := writeAtomic(plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("failed to write plist %s: %w", plistPath, err)
	}

	// Best-effort unload before loading updated plist
	_ = s.runner().Run(ctx, "launchctl", []string{"unload", plistPath}, nil, &bytes.Buffer{}, &bytes.Buffer{})

	var stderr bytes.Buffer
	if err := s.runner().Run(ctx, "launchctl", []string{"load", "-w", plistPath}, nil, &bytes.Buffer{}, &stderr); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("launchctl load failed: %w (%s)", err, msg)
		}
		return fmt.Errorf("launchctl load failed: %w", err)
	}
	return nil
}

func (s *Scheduler) disableDarwin(ctx context.Context) error {
	plistPath, err := s.launchdPlistPath()
	if err != nil {
		return err
	}
	_ = s.runner().Run(ctx, "launchctl", []string{"unload", "-w", plistPath}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist %s: %w", plistPath, err)
	}
	return nil
}

func (s *Scheduler) isRegisteredDarwin(ctx context.Context) (bool, error) {
	err := s.runner().Run(ctx, "launchctl", []string{"list", launchdLabel}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (s *Scheduler) isSystemdUserAvailable(ctx context.Context) bool {
	err := s.runner().Run(ctx, "systemctl", []string{"--user", "show-environment"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	return err == nil
}

func (s *Scheduler) enableLinux(ctx context.Context, bin string, interval time.Duration, notify bool) error {
	if s.isSystemdUserAvailable(ctx) {
		return s.enableLinuxSystemd(ctx, bin, interval, notify)
	}
	return s.enableLinuxCrontab(ctx, bin, interval, notify)
}

func (s *Scheduler) enableLinuxSystemd(ctx context.Context, bin string, interval time.Duration, notify bool) error {
	unitDir, err := s.systemdUserDir()
	if err != nil {
		return err
	}
	serviceContent, err := GenerateSystemdService(bin, notify)
	if err != nil {
		return err
	}
	timerContent, err := GenerateSystemdTimer(interval)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return fmt.Errorf("failed to create systemd user directory: %w", err)
	}
	if err := writeAtomic(filepath.Join(unitDir, systemdServiceName), []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write systemd service unit: %w", err)
	}
	if err := writeAtomic(filepath.Join(unitDir, systemdTimerName), []byte(timerContent), 0644); err != nil {
		return fmt.Errorf("failed to write systemd timer unit: %w", err)
	}

	var stderr bytes.Buffer
	if err := s.runner().Run(ctx, "systemctl", []string{"--user", "daemon-reload"}, nil, &bytes.Buffer{}, &stderr); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("systemctl --user daemon-reload failed: %w (%s)", err, msg)
		}
		return fmt.Errorf("systemctl --user daemon-reload failed: %w", err)
	}

	stderr.Reset()
	if err := s.runner().Run(ctx, "systemctl", []string{"--user", "enable", "--now", systemdTimerName}, nil, &bytes.Buffer{}, &stderr); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("systemctl --user enable failed: %w (%s)", err, msg)
		}
		return fmt.Errorf("systemctl --user enable failed: %w", err)
	}
	return nil
}

func (s *Scheduler) enableLinuxCrontab(ctx context.Context, bin string, interval time.Duration, notify bool) error {
	entry, err := GenerateCrontabEntry(bin, interval, notify)
	if err != nil {
		return err
	}
	existing, err := s.readCrontab(ctx)
	if err != nil {
		return err
	}
	filtered := filterCrontabLines(existing)
	var out strings.Builder
	for _, line := range filtered {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	out.WriteString(entry)
	out.WriteByte('\n')

	var stderr bytes.Buffer
	if err := s.runner().Run(ctx, "crontab", []string{"-"}, strings.NewReader(out.String()), &bytes.Buffer{}, &stderr); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("crontab update failed: %w (%s)", err, msg)
		}
		return fmt.Errorf("crontab update failed: %w", err)
	}
	return nil
}

func (s *Scheduler) disableLinux(ctx context.Context) error {
	if s.isSystemdUserAvailable(ctx) {
		_ = s.runner().Run(ctx, "systemctl", []string{"--user", "disable", "--now", systemdTimerName}, nil, &bytes.Buffer{}, &bytes.Buffer{})
		if unitDir, err := s.systemdUserDir(); err == nil {
			_ = os.Remove(filepath.Join(unitDir, systemdTimerName))
			_ = os.Remove(filepath.Join(unitDir, systemdServiceName))
		}
		_ = s.runner().Run(ctx, "systemctl", []string{"--user", "daemon-reload"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	} else if unitDir, err := s.systemdUserDir(); err == nil {
		_ = os.Remove(filepath.Join(unitDir, systemdTimerName))
		_ = os.Remove(filepath.Join(unitDir, systemdServiceName))
	}

	// Also remove any crontab fallback entry if present.
	existing, err := s.readCrontab(ctx)
	if err == nil && strings.Contains(existing, crontabMarker) {
		filtered := filterCrontabLines(existing)
		var out strings.Builder
		for _, line := range filtered {
			out.WriteString(line)
			out.WriteByte('\n')
		}
		_ = s.runner().Run(ctx, "crontab", []string{"-"}, strings.NewReader(out.String()), &bytes.Buffer{}, &bytes.Buffer{})
	}
	return nil
}

func (s *Scheduler) isRegisteredLinux(ctx context.Context) (bool, error) {
	if s.isSystemdUserAvailable(ctx) {
		err := s.runner().Run(ctx, "systemctl", []string{"--user", "is-enabled", systemdTimerName}, nil, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			return true, nil
		}
	}
	existing, err := s.readCrontab(ctx)
	if err != nil {
		return false, nil
	}
	for _, line := range strings.Split(existing, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, crontabMarker) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Scheduler) readCrontab(ctx context.Context) (string, error) {
	var stdout, stderr bytes.Buffer
	err := s.runner().Run(ctx, "crontab", []string{"-l"}, nil, &stdout, &stderr)
	if err != nil {
		lower := strings.ToLower(stderr.String())
		if strings.Contains(lower, "no crontab") || (stdout.Len() == 0 && strings.TrimSpace(stderr.String()) == "") {
			return "", nil
		}
		return "", fmt.Errorf("crontab -l failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func filterCrontabLines(raw string) []string {
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, crontabMarker) {
			continue
		}
		kept = append(kept, line)
	}
	return kept
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

func notifyArg(notify bool) string {
	if notify {
		return "--notify"
	}
	return "--notify=false"
}

func formatISO8601Duration(d time.Duration) string {
	totalSeconds := int64(d.Seconds())
	if totalSeconds <= 0 {
		return "PT15M"
	}
	if totalSeconds%(7*86400) == 0 {
		return fmt.Sprintf("P%dD", totalSeconds/86400)
	}
	if totalSeconds%86400 == 0 {
		return fmt.Sprintf("P%dD", totalSeconds/86400)
	}
	hours := totalSeconds / 3600
	rem := totalSeconds % 3600
	minutes := rem / 60
	seconds := rem % 60
	var b strings.Builder
	b.WriteString("PT")
	if hours > 0 {
		b.WriteString(strconv.FormatInt(hours, 10))
		b.WriteByte('H')
	}
	if minutes > 0 {
		b.WriteString(strconv.FormatInt(minutes, 10))
		b.WriteByte('M')
	}
	if seconds > 0 {
		b.WriteString(strconv.FormatInt(seconds, 10))
		b.WriteByte('S')
	}
	return b.String()
}

// GenerateWindowsTaskXML produces a Windows Task Scheduler XML definition for pw schedule run.
func GenerateWindowsTaskXML(binaryPath string, interval time.Duration, notify bool) (string, error) {
	return generateWindowsTaskXML(binaryPath, interval, notify, time.Now())
}

func generateWindowsTaskXML(binaryPath string, interval time.Duration, notify bool, startAt time.Time) (string, error) {
	if strings.TrimSpace(binaryPath) == "" {
		return "", errors.New("binary path cannot be empty")
	}
	if interval < MinInterval {
		return "", fmt.Errorf("interval %s must be at least %s", interval, MinInterval)
	}
	args := fmt.Sprintf("schedule run --quiet %s", notifyArg(notify))
	isoInterval := formatISO8601Duration(interval)
	boundary := startAt.Local().Format("2006-01-02T15:04:05")

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Periodic Patchwork package update check</Description>
  </RegistrationInfo>
  <Triggers>
    <TimeTrigger>
      <Repetition>
        <Interval>%s</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
      <StartBoundary>%s</StartBoundary>
      <Enabled>true</Enabled>
    </TimeTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
    <ExecutionTimeLimit>PT1H</ExecutionTimeLimit>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>%s</Arguments>
    </Exec>
  </Actions>
</Task>
`, isoInterval, boundary, xmlEscape(binaryPath), xmlEscape(args)), nil
}

// GenerateLaunchdPlist produces a macOS launchd user LaunchAgent plist for io.patchwork.check.
func GenerateLaunchdPlist(binaryPath string, interval time.Duration, notify bool) (string, error) {
	if strings.TrimSpace(binaryPath) == "" {
		return "", errors.New("binary path cannot be empty")
	}
	if interval < MinInterval {
		return "", fmt.Errorf("interval %s must be at least %s", interval, MinInterval)
	}
	seconds := int64(interval.Seconds())
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>schedule</string>
    <string>run</string>
    <string>--quiet</string>
    <string>%s</string>
  </array>
  <key>StartInterval</key>
  <integer>%d</integer>
  <key>RunAtLoad</key>
  <false/>
</dict>
</plist>
`, launchdLabel, xmlEscape(binaryPath), xmlEscape(notifyArg(notify)), seconds), nil
}

func quoteSystemdExec(bin string) string {
	clean := strings.ReplaceAll(bin, "\r", "")
	clean = strings.ReplaceAll(clean, "\n", "")
	if strings.ContainsAny(clean, " \t\"'\\") {
		escaped := strings.ReplaceAll(clean, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return clean
}

func formatSystemdDuration(d time.Duration) string {
	totalSeconds := int64(d.Seconds())
	if totalSeconds%3600 == 0 {
		return fmt.Sprintf("%dh", totalSeconds/3600)
	}
	if totalSeconds%60 == 0 {
		return fmt.Sprintf("%dm", totalSeconds/60)
	}
	return fmt.Sprintf("%ds", totalSeconds)
}

// GenerateSystemdService produces the Linux systemd --user service unit file content.
func GenerateSystemdService(binaryPath string, notify bool) (string, error) {
	if strings.TrimSpace(binaryPath) == "" {
		return "", errors.New("binary path cannot be empty")
	}
	return fmt.Sprintf(`[Unit]
Description=Patchwork package update check

[Service]
Type=oneshot
ExecStart=%s schedule run --quiet %s
`, quoteSystemdExec(binaryPath), notifyArg(notify)), nil
}

// GenerateSystemdTimer produces the Linux systemd --user timer unit file content.
func GenerateSystemdTimer(interval time.Duration) (string, error) {
	if interval < MinInterval {
		return "", fmt.Errorf("interval %s must be at least %s", interval, MinInterval)
	}
	return fmt.Sprintf(`[Unit]
Description=Periodic Patchwork package update check

[Timer]
OnBootSec=5m
OnUnitActiveSec=%s
Persistent=true

[Install]
WantedBy=timers.target
`, formatSystemdDuration(interval)), nil
}

func cronScheduleFromDuration(d time.Duration) string {
	totalMinutes := int(d / time.Minute)
	switch {
	case totalMinutes == 7*24*60:
		return "0 0 * * 0"
	case totalMinutes >= 24*60 && totalMinutes%(24*60) == 0:
		days := totalMinutes / (24 * 60)
		if days == 1 {
			return "0 0 * * *"
		}
		if days <= 28 {
			return fmt.Sprintf("0 0 */%d * *", days)
		}
		return "0 0 1 * *"
	case totalMinutes >= 60 && totalMinutes%60 == 0:
		hours := totalMinutes / 60
		if hours == 1 {
			return "0 * * * *"
		}
		if hours < 24 {
			return fmt.Sprintf("0 */%d * * *", hours)
		}
		return "0 0 * * *"
	case totalMinutes < 60:
		if totalMinutes < 15 {
			totalMinutes = 15
		}
		return fmt.Sprintf("*/%d * * * *", totalMinutes)
	default:
		hours := totalMinutes / 60
		if hours < 1 {
			hours = 1
		}
		if hours >= 24 {
			return "0 0 * * *"
		}
		return fmt.Sprintf("0 */%d * * *", hours)
	}
}

func shellQuotePOSIX(s string) string {
	clean := strings.ReplaceAll(s, "\r", "")
	clean = strings.ReplaceAll(clean, "\n", "")
	clean = strings.ReplaceAll(clean, "\x00", "")
	isSafe := true
	for i := 0; i < len(clean); i++ {
		c := clean[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '/' || c == '.' || c == '_' || c == '-' || c == ':') {
			isSafe = false
			break
		}
	}
	if isSafe && clean != "" {
		return clean
	}
	return "'" + strings.ReplaceAll(clean, "'", `'\''`) + "'"
}

// GenerateCrontabEntry produces a single crontab line for Linux environments without systemd --user.
func GenerateCrontabEntry(binaryPath string, interval time.Duration, notify bool) (string, error) {
	if strings.TrimSpace(binaryPath) == "" {
		return "", errors.New("binary path cannot be empty")
	}
	if interval < MinInterval {
		return "", fmt.Errorf("interval %s must be at least %s", interval, MinInterval)
	}
	expr := cronScheduleFromDuration(interval)
	return fmt.Sprintf("%s %s schedule run --quiet %s %s", expr, shellQuotePOSIX(binaryPath), notifyArg(notify), crontabMarker), nil
}
