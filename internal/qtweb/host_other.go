//go:build !linux

package qtweb

import (
	"errors"
	"os/exec"
)

func enabled() bool { return false }
func hostCommand() (*exec.Cmd, error) {
	return nil, errors.New("Qt WebEngine 后端仅用于 Linux")
}
