//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

// Replace the wrapper so q-secrets receives signals directly, keeps the same
// foreground terminal and owns the final exit status. Q_SECRET_KEY is needed by
// q-secrets itself; q-secrets removes it before launching the user's command.
func executeQSecrets(cmd *exec.Cmd) error {
	if cmd.Err != nil {
		return cmd.Err
	}
	return syscall.Exec(cmd.Path, cmd.Args, os.Environ())
}
