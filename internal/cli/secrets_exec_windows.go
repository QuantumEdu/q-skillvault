//go:build windows

package cli

import "os/exec"

// Windows has no POSIX exec replacement. Keep the existing subprocess behavior.
func executeQSecrets(cmd *exec.Cmd) error { return cmd.Run() }
