// Package poller keeps one detached background process refreshing GitLab data
// for the current herdr server. See doc/design.md §3.
package poller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hlouis/herdr-glab/internal/plugin"
	"github.com/hlouis/herdr-glab/internal/refresh"
)

const (
	recordFile   = "poller.json"
	lockFile     = "poller.lock"
	stopFile     = "poller.stop"
	logFile      = "poller.log"
	maxLogSize   = 1 << 20
	cycleTimeout = 2 * time.Minute
)

type record struct {
	PID         int       `json:"pid"`
	SocketPath  string    `json:"socket_path"`
	BinaryMtime int64     `json:"binary_mtime"`
	StartedAt   time.Time `json:"started_at"`
}

// Ensure starts the poller unless one is already running for this herdr
// server and binary, or the user stopped it.
func Ensure(env plugin.Env) error {
	if stopRequested(env) {
		return nil
	}
	unlock, err := lock(env)
	if err != nil {
		return err
	}
	defer unlock()

	exe, mtime, err := executable()
	if err != nil {
		return err
	}
	if rec, err := readRecord(env); err == nil && alive(rec.PID) {
		if rec.SocketPath == env.SocketPath && rec.BinaryMtime == mtime {
			return nil
		}
		_ = syscall.Kill(rec.PID, syscall.SIGTERM)
	}
	return spawn(env, exe, mtime)
}

// Stop terminates the poller and keeps it stopped until ClearStop.
func Stop(env plugin.Env) error {
	if err := os.MkdirAll(env.StateDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(env.StateDir, stopFile), nil, 0o644); err != nil {
		return err
	}
	if rec, err := readRecord(env); err == nil && alive(rec.PID) {
		_ = syscall.Kill(rec.PID, syscall.SIGTERM)
	}
	return nil
}

func ClearStop(env plugin.Env) error {
	err := os.Remove(filepath.Join(env.StateDir, stopFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Run refreshes on every fetch interval until ctx ends, the user stops it,
// another poller replaces it, or the herdr server socket disappears.
func Run(ctx context.Context, d refresh.Deps) error {
	self := os.Getpid()
	ticker := time.NewTicker(d.Config.FetchInterval)
	defer ticker.Stop()
	log.Printf("poller %d started, interval %s, host %s", self, d.Config.FetchInterval, d.Config.Host)

	for {
		if reason := exitReason(d.Env, self); reason != "" {
			log.Printf("poller %d exiting: %s", self, reason)
			return nil
		}
		cycleCtx, cancel := context.WithTimeout(ctx, cycleTimeout)
		if _, err := refresh.All(cycleCtx, d); err != nil {
			log.Printf("refresh: %v", err)
		}
		cancel()

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func exitReason(env plugin.Env, self int) string {
	if stopRequested(env) {
		return "stop requested"
	}
	if rec, err := readRecord(env); err != nil || rec.PID != self {
		return "replaced by another poller"
	}
	if env.SocketPath != "" {
		if _, err := os.Stat(env.SocketPath); err != nil {
			return "herdr server socket is gone"
		}
	}
	return ""
}

func spawn(env plugin.Env, exe string, mtime int64) error {
	if err := os.MkdirAll(env.StateDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(env.StateDir, logFile)
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxLogSize {
		_ = os.Rename(logPath, logPath+".1")
	}
	out, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	cmd := exec.Command(exe, "poller")
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start poller: %w", err)
	}
	rec := record{PID: cmd.Process.Pid, SocketPath: env.SocketPath, BinaryMtime: mtime, StartedAt: time.Now()}
	if err := writeRecord(env, rec); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// lock serializes Ensure across concurrent event hooks.
func lock(env plugin.Env) (func(), error) {
	if err := os.MkdirAll(env.StateDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(env.StateDir, lockFile), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

func executable() (string, int64, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", 0, err
	}
	info, err := os.Stat(exe)
	if err != nil {
		return "", 0, err
	}
	return exe, info.ModTime().UnixNano(), nil
}

func stopRequested(env plugin.Env) bool {
	_, err := os.Stat(filepath.Join(env.StateDir, stopFile))
	return err == nil
}

func alive(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

func readRecord(env plugin.Env) (record, error) {
	var rec record
	data, err := os.ReadFile(filepath.Join(env.StateDir, recordFile))
	if err != nil {
		return rec, err
	}
	return rec, json.Unmarshal(data, &rec)
}

func writeRecord(env plugin.Env, rec record) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(env.StateDir, recordFile), data, 0o644)
}
