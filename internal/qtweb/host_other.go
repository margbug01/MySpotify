//go:build !linux

package qtweb

import (
	"errors"
	"os/exec"

	"vinyl/internal/l10n"
)

func enabled() bool { return false }
func hostCommand() (*exec.Cmd, error) {
	return nil, errors.New(l10n.T("Qt WebEngine 后端仅用于 Linux"))
}
