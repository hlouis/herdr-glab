package run

import (
	"bytes"
	"context"
	"os"
	"os/exec"
)

// OutputEnv runs a command with extra environment entries ("KEY=value").
func OutputEnv(ctx context.Context, extraEnv []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), &Error{Name: name, Args: args, Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
	}
	return stdout.Bytes(), nil
}
