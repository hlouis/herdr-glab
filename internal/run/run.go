// Package run executes external commands and captures their output.
package run

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Error describes a failed command, keeping both output streams because glab
// reports HTTP errors on stdout and stderr.
type Error struct {
	Name   string
	Args   []string
	Stdout string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = e.Err.Error()
	}
	sub := ""
	if len(e.Args) > 0 {
		sub = " " + e.Args[0]
	}
	return fmt.Sprintf("%s%s: %s", e.Name, sub, msg)
}

func (e *Error) Unwrap() error {
	return e.Err
}

// Output runs name with args and returns stdout.
func Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return OutputIn(ctx, "", name, args...)
}

// OutputIn runs name with args in dir and returns stdout.
func OutputIn(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), &Error{Name: name, Args: args, Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
	}
	return stdout.Bytes(), nil
}
