package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pleware/initagent/internal/brand"
)

func systemdUnitText(exe, user string) string {
	return fmt.Sprintf(`[Unit]
Description=%s device connector
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%s agent run
Environment=%s=agent
Restart=always
RestartSec=3
User=%s

[Install]
WantedBy=default.target
`, brand.Name, exe, brand.EnvManaged, user)
}

func launchdPlistText(exe, logPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>agent</string>
		<string>run</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict><key>%s</key><string>agent</string></dict>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
	<key>StandardOutPath</key><string>%s</string>
	<key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, brand.LaunchdLabel, exe, brand.EnvManaged, logPath, logPath)
}

// InstallService registers the agent to run in the background and starts it now.
// Linux: user systemd unit (falls back to system unit when running as root).
// macOS: launchd user agent.
// Windows: scheduled task on user logon.
func InstallService() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)

	switch runtime.GOOS {
	case "linux":
		return installSystemd(exe)
	case "darwin":
		return installLaunchd(exe)
	case "windows":
		return installWindowsTask(exe)
	default:
		return fmt.Errorf("service install not supported on %s; run `%s agent run` under your own supervisor", runtime.GOOS, brand.Binary)
	}
}

func installSystemd(exe string) error {
	unitFile := brand.ConnectorUnit + ".service"
	if os.Geteuid() == 0 {
		user := os.Getenv("SUDO_USER")
		if user == "" {
			user = "root"
		}
		unit := systemdUnitText(exe, user)
		if err := os.WriteFile("/etc/systemd/system/"+unitFile, []byte(unit), 0o644); err != nil {
			return err
		}
		for _, args := range [][]string{{"daemon-reload"}, {"enable", "--now", brand.ConnectorUnit}} {
			if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
				return fmt.Errorf("systemctl %v: %s", args, out)
			}
		}
		return nil
	}
	// User unit: survives logout only with lingering; enable it best-effort.
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	unit := systemdUnitText(exe, "")
	// User units must not set User=.
	unit = removeLine(unit, "User=")
	if err := os.WriteFile(filepath.Join(dir, unitFile), []byte(unit), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"--user", "daemon-reload"}, {"--user", "enable", "--now", brand.ConnectorUnit}} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %v: %s", args, out)
		}
	}
	exec.Command("loginctl", "enable-linger").Run() // best effort
	return nil
}

func installLaunchd(exe string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logDir := filepath.Join(home, brand.ConfigDir)
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	logPath := filepath.Join(logDir, "agent.log")
	plist := launchdPlistText(exe, logPath)
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	plistPath := filepath.Join(dir, brand.LaunchdLabel+".plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return err
	}
	exec.Command("launchctl", "unload", plistPath).Run() // reload if present
	if out, err := exec.Command("launchctl", "load", plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %s", out)
	}
	return nil
}

func installWindowsTask(exe string) error {
	// A tiny runner supplies the metadata used by the safe updater. Keeping the
	// Scheduled Task action simple avoids Windows quoting bugs in paths with
	// spaces, and the updater can restart the same named task after swapping.
	runner := filepath.Join(filepath.Dir(exe), brand.ConnectorUnit+".cmd")
	batchExe := strings.ReplaceAll(exe, "%", "%%")
	contents := fmt.Sprintf("@echo off\r\nset \"%s=agent\"\r\nset \"%s=%s\"\r\n\"%s\" agent run\r\n", brand.EnvManaged, brand.EnvWindowsTask, brand.WindowsTaskName, batchExe)
	if err := os.WriteFile(runner, []byte(contents), 0o600); err != nil {
		return err
	}
	action := fmt.Sprintf(`"%s"`, runner)
	for _, args := range [][]string{
		{"/Create", "/TN", brand.WindowsTaskName, "/SC", "ONLOGON", "/TR", action, "/F"},
		{"/Run", "/TN", brand.WindowsTaskName},
	} {
		if out, err := exec.Command("schtasks.exe", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("schtasks %v: %s", args, out)
		}
	}
	return nil
}

func removeLine(s, prefix string) string {
	out := ""
	for _, line := range splitLines(s) {
		if len(line) >= len(prefix) && line[:len(prefix)] == prefix {
			continue
		}
		out += line + "\n"
	}
	return out
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
