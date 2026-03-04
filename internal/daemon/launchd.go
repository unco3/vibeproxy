package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const launchdLabel = "com.vibeproxy.agent"

var plistTemplate = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.Label}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.Executable}}</string>
		<string>start</string>
		<string>--foreground</string>
	</array>
	<key>WorkingDirectory</key>
	<string>{{.WorkDir}}</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>{{.LogDir}}/stdout.log</string>
	<key>StandardErrorPath</key>
	<string>{{.LogDir}}/stderr.log</string>
</dict>
</plist>
`))

type plistData struct {
	Label      string
	Executable string
	WorkDir    string
	LogDir     string
}

func plistPath() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", launchdLabel+".plist")
}

// InstallLaunchd creates a launchd plist and loads it.
func InstallLaunchd(workDir string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}
	// Resolve symlinks for a stable path
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolving symlinks: %w", err)
	}

	logDir := filepath.Join(os.Getenv("HOME"), ".vibeproxy")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return err
	}

	path := plistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating plist: %w", err)
	}
	defer f.Close()

	data := plistData{
		Label:      launchdLabel,
		Executable: exe,
		WorkDir:    workDir,
		LogDir:     logDir,
	}
	if err := plistTemplate.Execute(f, data); err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}

	// Load the service
	cmd := exec.Command("launchctl", "load", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %s: %w", out, err)
	}

	return nil
}

// UninstallLaunchd unloads and removes the launchd plist.
func UninstallLaunchd() error {
	path := plistPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("launchd service not installed")
	}

	cmd := exec.Command("launchctl", "unload", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl unload: %s: %w", out, err)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("removing plist: %w", err)
	}

	return nil
}
