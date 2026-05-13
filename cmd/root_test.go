package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/version"
)

// The upgrade nag must land on stderr only — anything emitted to stdout
// during a `slatewave list --json | jq` invocation pollutes the JSON
// the consumer reads. emitUpgradeNag hardcodes os.Stderr; this test
// captures both fds during a call with a staged update result and
// asserts stdout stays empty while stderr carries the nag.
//
// Lives in cmd_test (not version_test) because the wire contract is
// "the cmd-layer postrun emits to stderr only" — internal/version
// itself only returns a Result; whether/where it gets printed is the
// command layer's responsibility, which is what this test pins.
func TestEmitUpgradeNag_WritesToStderrNotStdout(t *testing.T) {
	stdoutOut, stderrOut, restore := captureStdFds(t)
	prevCh := versionCheckCh
	t.Cleanup(func() { versionCheckCh = prevCh })

	ch := make(chan *version.Result, 1)
	ch <- &version.Result{Latest: "v9.9.9", URL: "https://example.test/release"}
	versionCheckCh = ch

	emitUpgradeNag()

	stdout, stderr := restore()
	if stdout.Len() != 0 {
		t.Errorf("nag leaked to stdout (would corrupt --json output): %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "v9.9.9") {
		t.Errorf("expected nag on stderr containing %q, got: %q", "v9.9.9", stderr.String())
	}
	_ = stdoutOut
	_ = stderrOut
}

// Silent path: if the goroutine returns nil (no update available), the
// nag must produce no output on either fd. A regression here would mean
// every up-to-date `slatewave` run starts emitting blank lines.
func TestEmitUpgradeNag_NilResultEmitsNothing(t *testing.T) {
	_, _, restore := captureStdFds(t)
	prevCh := versionCheckCh
	t.Cleanup(func() { versionCheckCh = prevCh })

	ch := make(chan *version.Result, 1)
	ch <- nil
	versionCheckCh = ch

	emitUpgradeNag()

	stdout, stderr := restore()
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("up-to-date run emitted output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

// captureStdFds swaps os.Stdout / os.Stderr to pipes, runs the body via
// the returned restore func, and returns buffers holding what was
// written to each fd. Caller invokes restore() exactly once when the
// production code under test has finished writing.
//
// emitUpgradeNag writes via fmt.Fprintln(os.Stderr, ...), which
// dereferences the package var at write time — so reassigning here
// reroutes the write, even though fd 2 itself stays attached to the
// terminal. The lipgloss styling helpers used inside the nag are
// captured by the same redirect.
func captureStdFds(t *testing.T) (stdout, stderr io.Writer, restore func() (*bytes.Buffer, *bytes.Buffer)) {
	t.Helper()
	oldStdout, oldStderr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	os.Stdout = wOut
	os.Stderr = wErr

	restore = func() (*bytes.Buffer, *bytes.Buffer) {
		_ = wOut.Close()
		_ = wErr.Close()
		var outBuf, errBuf bytes.Buffer
		_, _ = io.Copy(&outBuf, rOut)
		_, _ = io.Copy(&errBuf, rErr)
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		return &outBuf, &errBuf
	}
	return wOut, wErr, restore
}
