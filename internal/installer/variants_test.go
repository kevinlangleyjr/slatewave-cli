package installer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/shell"
)

func TestMatchConstraint(t *testing.T) {
	cases := []struct {
		expr    string
		version string
		want    bool
		wantErr bool
	}{
		{"<1.1.0", "1.0.5", true, false},
		{"<1.1.0", "1.1.0", false, false},
		{"<1.1.0", "0.9.0", true, false},
		{"<=1.0.5", "1.0.5", true, false},
		{"<=1.0.5", "1.0.6", false, false},
		{">=2.0.0", "2.0.0", true, false},
		{">=2.0.0", "1.9.9", false, false},
		{">2.0.0", "2.0.0", false, false},
		{">2.0.0", "2.0.1", true, false},
		{"=1.0.5", "1.0.5", true, false},
		{"=1.0.5", "1.0.6", false, false},
		{"1.0.5", "1.0.5", true, false},
		{"1.0.5", "1.0.6", false, false},
		{"~1.0", "1.0.5", false, true},
		{"<1.0", "1.0.0", false, true},
		{"", "1.0.0", false, true},
		{"<1.0.0", "not-a-version", false, true},
	}
	for _, c := range cases {
		got, err := matchConstraint(c.expr, c.version)
		if c.wantErr {
			if err == nil {
				t.Errorf("matchConstraint(%q, %q): want error, got match=%v", c.expr, c.version, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("matchConstraint(%q, %q): unexpected error: %v", c.expr, c.version, err)
			continue
		}
		if got != c.want {
			t.Errorf("matchConstraint(%q, %q) = %v, want %v", c.expr, c.version, got, c.want)
		}
	}
}

func TestResolveVariant_FirstMatchWins(t *testing.T) {
	variants := []manifest.InstallVariant{
		{WhenVersion: "<1.1.0", URL: "old"},
		{WhenVersion: "<2.0.0", URL: "older"},
	}
	got, err := resolveVariant(variants, "1.0.5")
	if err != nil {
		t.Fatalf("resolveVariant: %v", err)
	}
	if got == nil || got.URL != "old" {
		t.Errorf("first match should win; got %+v", got)
	}
}

func TestResolveVariant_NoMatch(t *testing.T) {
	variants := []manifest.InstallVariant{
		{WhenVersion: "<1.0.0", URL: "old"},
	}
	got, err := resolveVariant(variants, "2.0.0")
	if err != nil {
		t.Fatalf("resolveVariant: %v", err)
	}
	if got != nil {
		t.Errorf("no match should return nil; got %+v", got)
	}
}

func TestResolveVariant_MalformedConstraint(t *testing.T) {
	variants := []manifest.InstallVariant{
		{WhenVersion: "~1.0", URL: "old"},
	}
	_, err := resolveVariant(variants, "1.0.0")
	if err == nil {
		t.Fatal("malformed constraint should error")
	}
	if !strings.Contains(err.Error(), "variants[0]") {
		t.Errorf("error should index the offending variant: %v", err)
	}
}

func TestApplyVariant_OverlaysOnlyNonEmpty(t *testing.T) {
	parent := manifest.Install{
		Type: "curl",
		URL:  "default-url",
		Dest: "default-dest",
	}
	v := &manifest.InstallVariant{URL: "variant-url"}
	out := applyVariant(parent, v)
	if out.URL != "variant-url" {
		t.Errorf("URL should be overridden; got %q", out.URL)
	}
	if out.Dest != "default-dest" {
		t.Errorf("Dest should be preserved when variant omits it; got %q", out.Dest)
	}
	if out.Type != "curl" {
		t.Errorf("Type should fall through; got %q", out.Type)
	}
}

func TestApplyVariant_FilesClearsSingleFileFields(t *testing.T) {
	parent := manifest.Install{URL: "default-url", Dest: "default-dest"}
	v := &manifest.InstallVariant{
		Files: []manifest.InstallFile{{URL: "f", Dest: "/p"}},
	}
	out := applyVariant(parent, v)
	if out.URL != "" || out.Dest != "" {
		t.Errorf("variant with Files should clear URL/Dest; got URL=%q Dest=%q", out.URL, out.Dest)
	}
	if len(out.Files) != 1 || out.Files[0].URL != "f" {
		t.Errorf("Files should be replaced; got %+v", out.Files)
	}
}

func TestApplyVariant_NilReturnsParent(t *testing.T) {
	parent := manifest.Install{URL: "default-url"}
	out := applyVariant(parent, nil)
	if out.URL != "default-url" {
		t.Errorf("nil variant should pass through; got %+v", out)
	}
}

// detectVersion shells out to a real /bin/sh, so the tests use a tiny
// helper script in t.TempDir() to fake `lsd --version`. shell.Run runs
// commands through `sh -c`, so anything resolvable on PATH works.
func writeFakeBin(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}
	return dir
}

func prependPath(t *testing.T, dir string) {
	t.Helper()
	prev := os.Getenv("PATH")
	if err := os.Setenv("PATH", dir+":"+prev); err != nil {
		t.Fatalf("setenv PATH: %v", err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", prev) })
}

func TestDetectVersion_RegexCapture(t *testing.T) {
	dir := writeFakeBin(t, "lsd", "#!/bin/sh\necho 'lsd 1.0.5'\n")
	prependPath(t, dir)

	th := manifest.Theme{
		Theme: manifest.Meta{
			DetectCommand: "lsd --version",
			VersionRegex:  `lsd ([0-9]+\.[0-9]+\.[0-9]+)`,
		},
	}
	got, err := detectVersion(th)
	if err != nil {
		t.Fatalf("detectVersion: %v", err)
	}
	if got != "1.0.5" {
		t.Errorf("got %q, want 1.0.5", got)
	}
}

func TestDetectVersion_RegexMisses(t *testing.T) {
	dir := writeFakeBin(t, "lsd", "#!/bin/sh\necho 'unrelated output'\n")
	prependPath(t, dir)

	th := manifest.Theme{
		Theme: manifest.Meta{
			DetectCommand: "lsd --version",
			VersionRegex:  `lsd ([0-9]+\.[0-9]+\.[0-9]+)`,
		},
	}
	_, err := detectVersion(th)
	if err == nil {
		t.Fatal("regex miss should error, not silently fall back")
	}
	if !strings.Contains(err.Error(), "did not capture") {
		t.Errorf("error should explain the regex miss: %v", err)
	}
}

func TestDetectVersion_MissingRegex(t *testing.T) {
	th := manifest.Theme{
		Theme: manifest.Meta{
			DetectCommand: "lsd --version",
			// VersionRegex empty → caller treats as "no variant logic"
		},
	}
	got, err := detectVersion(th)
	if err != nil {
		t.Fatalf("missing regex should be quiet, not error: %v", err)
	}
	if got != "" {
		t.Errorf("missing regex should return empty version; got %q", got)
	}
}

func TestDetectVersion_BadRegexCompile(t *testing.T) {
	th := manifest.Theme{
		Theme: manifest.Meta{
			DetectCommand: "true",
			VersionRegex:  "(unbalanced",
		},
	}
	_, err := detectVersion(th)
	if err == nil {
		t.Fatal("bad regex should error")
	}
}

// TestShellRun_Smoke is a defensive check that the shell package's
// timeout shape still works the way detectVersion assumes — if shell.Run
// stops respecting the context deadline, the whole detect path could
// hang. Cheap insurance.
func TestShellRun_Smoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	out, err := shell.Run(ctx, "echo hello")
	if err != nil {
		t.Fatalf("shell.Run: %v", err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Errorf("expected 'hello' in output: %q", out)
	}
}
