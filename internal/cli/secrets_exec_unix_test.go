//go:build !windows

package cli

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func TestExecuteQSecretsReplacesWrapper(t *testing.T) {
	const stageName = "Q_SKILLVAULT_EXEC_TEST_STAGE"
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	switch os.Getenv(stageName) {
	case "replace":
		os.Setenv(stageName, "replaced")
		os.Setenv("Q_SKILLVAULT_EXEC_TEST_PID", strconv.Itoa(os.Getpid()))
		if err := executeQSecrets(exec.Command(exe, "-test.run=^TestExecuteQSecretsReplacesWrapper$")); err != nil {
			os.Exit(91)
		}
		os.Exit(92)
	case "replaced":
		if os.Getenv("Q_SKILLVAULT_EXEC_TEST_PID") != strconv.Itoa(os.Getpid()) {
			os.Exit(93)
		}
		os.Exit(37)
	}
	child := exec.Command(exe, "-test.run=^TestExecuteQSecretsReplacesWrapper$")
	child.Env = append(os.Environ(), stageName+"=replace")
	var exit *exec.ExitError
	if err := child.Run(); !errors.As(err, &exit) || exit.ExitCode() != 37 {
		t.Fatalf("wrapper was not replaced or status changed: %v", err)
	}
}

func TestExecuteQSecretsMissingCommand(t *testing.T) {
	if err := executeQSecrets(exec.Command("q-skillvault-no-such-executable-test")); err == nil {
		t.Fatal("missing executable accepted")
	}
}
