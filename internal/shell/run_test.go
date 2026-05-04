package shell

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRun_EchoRoundtrip(t *testing.T) {
	// `echo hi` is the lowest common denominator — both sh and cmd.exe
	// implement it as a builtin, so this asserts the shell dispatch is
	// wired up without depending on any external binary.
	out, err := Run(context.Background(), "echo hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != "hi" {
		t.Errorf("got %q want %q", got, "hi")
	}
}

func TestRun_NonZeroExitReturnsError(t *testing.T) {
	// `exit 1` is portable: sh -c 'exit 1' and cmd /C 'exit 1' both
	// terminate non-zero. The error must surface so callers (Detect,
	// verifyInstalled) can treat it as "tool not present / verify failed".
	_, err := Run(context.Background(), "exit 1")
	if err == nil {
		t.Fatal("Run with exit 1 should return error")
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		// `sleep` isn't a builtin on Windows; skip rather than tangle
		// with timeout binaries that may or may not be on PATH.
		t.Skip("sleep semantics differ on windows")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := Run(ctx, "sleep 5")
	if err == nil {
		t.Fatal("expected ctx timeout to terminate the process")
	}
}

func TestRunInherit_NonZeroExitReturnsError(t *testing.T) {
	if err := RunInherit(context.Background(), "exit 1"); err == nil {
		t.Fatal("RunInherit with exit 1 should return error")
	}
}
