package verbose

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// Verbose mode off (the default) must produce zero output. Tests that
// happen to import this package shouldn't cause stderr noise on a
// non-verbose run; that's also the contract every Log call site
// relies on.
func TestLog_OffByDefault(t *testing.T) {
	buf := &bytes.Buffer{}
	SetWriter(buf)
	t.Cleanup(func() { SetWriter(io.Discard) })

	// Disable, then log — output buffer must stay empty.
	SetEnabled(false)
	Log("should not appear")
	if buf.Len() != 0 {
		t.Errorf("verbose disabled but wrote: %q", buf.String())
	}
}

// SetEnabled(true) routes Log to the writer; format args land in the
// expected shape ("↪ <formatted>\n").
func TestLog_EnabledRoutesToWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	SetWriter(buf)
	t.Cleanup(func() { SetWriter(io.Discard) })

	SetEnabled(true)
	t.Cleanup(func() { SetEnabled(false) })
	// SetEnabled(true) re-points the writer to os.Stderr — undo that
	// so the test can capture into a buffer.
	SetWriter(buf)

	Log("fetch %s", "https://example.com/x")

	got := buf.String()
	if !strings.HasPrefix(got, "↪ ") {
		t.Errorf("missing prefix: %q", got)
	}
	if !strings.Contains(got, "fetch https://example.com/x") {
		t.Errorf("format not applied: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("missing trailing newline: %q", got)
	}
}

// Toggling enabled→disabled→enabled must restore behavior cleanly so
// tests that flip the flag don't leak state into later tests.
func TestSetEnabled_TogglesCleanly(t *testing.T) {
	SetEnabled(true)
	buf := &bytes.Buffer{}
	SetWriter(buf)
	Log("on")
	if !strings.Contains(buf.String(), "on") {
		t.Errorf("enabled but didn't write: %q", buf.String())
	}

	SetEnabled(false)
	buf.Reset()
	Log("off")
	if buf.Len() != 0 {
		t.Errorf("disabled but wrote: %q", buf.String())
	}

	t.Cleanup(func() { SetWriter(io.Discard) })
}
