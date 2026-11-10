//go:build windows

package app

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
)

// ensureWindowsStartMenuShortcut registers the portable executable with the
// shell. SMTC uses the Start menu shortcut's name and icon for unpackaged apps.
func ensureWindowsStartMenuShortcut(name string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		return fmt.Errorf("find powershell.exe: %w", err)
	}
	const script = `$ErrorActionPreference = 'Stop'
$programs = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'
New-Item -ItemType Directory -Path $programs -Force | Out-Null
$shortcutPath = Join-Path $programs ($env:MYSPOTIFY_DISPLAY + '.lnk')
$exe = [IO.Path]::GetFullPath($env:MYSPOTIFY_EXE)
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($shortcutPath)
if (Test-Path -LiteralPath $shortcutPath) {
  $existing = $shortcut.TargetPath
  $existingDir = if ($existing) { [IO.Path]::GetDirectoryName([IO.Path]::GetFullPath($existing)) } else { $null }
  $exeDir = [IO.Path]::GetDirectoryName($exe)
  if ($existingDir -and -not [string]::Equals($existingDir, $exeDir, [StringComparison]::OrdinalIgnoreCase)) {
    $portablePrograms = Join-Path $programs ($env:MYSPOTIFY_DISPLAY + ' (portable)')
    New-Item -ItemType Directory -Path $portablePrograms -Force | Out-Null
    $shortcutPath = Join-Path $portablePrograms ($env:MYSPOTIFY_DISPLAY + '.lnk')
    $shortcut = $shell.CreateShortcut($shortcutPath)
    $existing = $shortcut.TargetPath
    $existingDir = if ($existing) { [IO.Path]::GetDirectoryName([IO.Path]::GetFullPath($existing)) } else { $null }
    if ($existingDir -and -not [string]::Equals($existingDir, $exeDir, [StringComparison]::OrdinalIgnoreCase)) {
      exit 0
    }
  }
}
$shortcut.TargetPath = $exe
$shortcut.WorkingDirectory = Split-Path -Parent $exe
$shortcut.IconLocation = "$exe,0"
$shortcut.Description = $env:MYSPOTIFY_DISPLAY
$shortcut.Save()`
	encoded := encodePowerShell(script)
	cmd := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encoded)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = append(os.Environ(), "MYSPOTIFY_EXE="+exe, "MYSPOTIFY_DISPLAY="+name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create %s shortcut: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func encodePowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	bytes := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(bytes[i*2:], unit)
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
