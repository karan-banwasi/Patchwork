package schedule

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// sanitizeNotificationText removes control characters (including NUL, CR, LF)
// so notification payloads remain single-line and cannot inject script delimiters.
func sanitizeNotificationText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\x00':
			continue
		case r == '\r' || r == '\n' || r == '\t':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			continue
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// escapePowerShellSingleQuoted escapes a string for safe embedding inside a PowerShell
// single-quoted literal ('...'), where the only special syntax is a single quote doubled to ''.
func escapePowerShellSingleQuoted(s string) string {
	clean := sanitizeNotificationText(s)
	return strings.ReplaceAll(clean, "'", "''")
}

// escapeAppleScriptString escapes a string for safe embedding inside an AppleScript
// double-quoted literal ("...").
func escapeAppleScriptString(s string) string {
	clean := sanitizeNotificationText(s)
	clean = strings.ReplaceAll(clean, `\`, `\\`)
	clean = strings.ReplaceAll(clean, `"`, `\"`)
	return clean
}

// SendNotification dispatches a native desktop notification using zero-dependency
// platform mechanisms (PowerShell Toast/Balloon on Windows, osascript on macOS,
// notify-send on Linux). Inputs are sanitized and escaped to prevent injection.
func SendNotification(ctx context.Context, runner pm.Runner, goos, title, message string) error {
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	if goos == "" {
		goos = runtime.GOOS
	}

	cleanTitle := sanitizeNotificationText(title)
	cleanMsg := sanitizeNotificationText(message)
	if cleanTitle == "" {
		cleanTitle = "Patchwork"
	}
	if cleanMsg == "" {
		return errors.New("notification message cannot be empty")
	}

	switch goos {
	case "windows":
		psTitle := escapePowerShellSingleQuoted(cleanTitle)
		psMsg := escapePowerShellSingleQuoted(cleanMsg)
		script := fmt.Sprintf(
			"$t = '%s'; $m = '%s'; "+
				"try { "+
				"[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null; "+
				"$xml = New-Object Windows.Data.Xml.Dom.XmlDocument; "+
				"$et = [System.Security.SecurityElement]::Escape($t); "+
				"$em = [System.Security.SecurityElement]::Escape($m); "+
				"$xml.LoadXml(\"<toast><visual><binding template='ToastText02'><text id='1'>$et</text><text id='2'>$em</text></binding></visual></toast>\"); "+
				"$toast = [Windows.UI.Notifications.ToastNotification]::new($xml); "+
				"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('Patchwork').Show($toast) "+
				"} catch { "+
				"Add-Type -AssemblyName System.Windows.Forms; "+
				"$n = New-Object System.Windows.Forms.NotifyIcon; "+
				"$n.Icon = [System.Drawing.SystemIcons]::Information; "+
				"$n.BalloonTipTitle = $t; "+
				"$n.BalloonTipText = $m; "+
				"$n.Visible = $true; "+
				"$n.ShowBalloonTip(5000); "+
				"Start-Sleep -Milliseconds 200; "+
				"$n.Dispose() "+
				"}",
			psTitle, psMsg,
		)
		var stderr bytes.Buffer
		args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script}
		if err := runner.Run(ctx, "powershell", args, nil, &bytes.Buffer{}, &stderr); err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg != "" {
				return fmt.Errorf("windows notification failed: %w (%s)", err, msg)
			}
			return fmt.Errorf("windows notification failed: %w", err)
		}
		return nil

	case "darwin":
		asTitle := escapeAppleScriptString(cleanTitle)
		asMsg := escapeAppleScriptString(cleanMsg)
		script := fmt.Sprintf(`display notification "%s" with title "%s"`, asMsg, asTitle)
		var stderr bytes.Buffer
		if err := runner.Run(ctx, "osascript", []string{"-e", script}, nil, &bytes.Buffer{}, &stderr); err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg != "" {
				return fmt.Errorf("macOS notification failed: %w (%s)", err, msg)
			}
			return fmt.Errorf("macOS notification failed: %w", err)
		}
		return nil

	case "linux":
		var stderr bytes.Buffer
		// "--" terminates options so package names or titles beginning with "-" cannot be parsed as flags.
		args := []string{"--app-name=Patchwork", "--", cleanTitle, cleanMsg}
		if err := runner.Run(ctx, "notify-send", args, nil, &bytes.Buffer{}, &stderr); err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg != "" {
				return fmt.Errorf("linux notification failed: %w (%s)", err, msg)
			}
			return fmt.Errorf("linux notification failed: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("desktop notifications are not supported on %s", goos)
	}
}
