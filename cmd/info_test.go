package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/jsonout"
)

const manifestForInfo = `[theme]
slug = "okayish"
name = "OK"
category = "editor"
detect_command = "true"

[install]
type = "curl"
url = "https://example.com/theme.toml"
dest = "~/.config/ok/theme.toml"
done_message = "Restart OK to see the change."

[activate]
type = "ini-key"
file = "~/.config/ok/config"
key = "theme"
value = "slatewave"

[verify]
command = "test -f ~/.config/ok/theme.toml"
expect = "ok"
`

// TestInfoCmd_HumanRendersAllSections asserts the human renderer prints
// every populated section heading. The test is intentionally loose on
// exact formatting (lipgloss styling depends on color profile) and just
// looks for the section labels and key field values — those are the
// informational anchors users scan for.
func TestInfoCmd_HumanRendersAllSections(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestForInfo})

	if err := infoCmd.RunE(infoCmd, []string{"okayish"}); err != nil {
		t.Fatalf("infoCmd.RunE: %v", err)
	}

	out := env.out.String()
	wants := []string{
		"OK",                             // theme name
		"okayish",                        // slug
		"editor",                         // category
		"darwin, linux",                  // default OS
		"Install",                        // section heading
		"curl",                           // install type
		"https://example.com/theme.toml", // url
		"Restart OK to see the change.",  // done_message echoed under "after"
		"Activate",                       // section heading
		"ini-key",                        // activate type
		"theme = slatewave",              // ini-key set
		"Verify",                         // section heading
		"Source",                         // section heading
		"https://getslatewave.com/themes/okayish", // canonical URL
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("expected info output to contain %q, got:\n%s", w, out)
		}
	}
}

// TestInfoCmd_JSONOutputShape asserts the JSON renderer produces the
// stable wire format with every populated manifest field landing in
// the right place.
func TestInfoCmd_JSONOutputShape(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestForInfo})

	resetFlags := setFlags(t, infoCmd, map[string]string{"json": "true"})
	defer resetFlags()
	if err := infoCmd.RunE(infoCmd, []string{"okayish"}); err != nil {
		t.Fatalf("infoCmd.RunE: %v", err)
	}

	var got jsonout.InfoOutput
	if err := json.Unmarshal(env.out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\nraw: %s", err, env.out.String())
	}

	if got.Slug != "okayish" || got.Name != "OK" || got.Category != "editor" {
		t.Errorf("identity wrong: %+v", got)
	}
	if got.Install.Type != "curl" || got.Install.URL != "https://example.com/theme.toml" {
		t.Errorf("install wrong: %+v", got.Install)
	}
	if got.Activate.Type != "ini-key" || got.Activate.Key != "theme" || got.Activate.Value != "slatewave" {
		t.Errorf("activate wrong: %+v", got.Activate)
	}
	if got.Verify.Command == "" {
		t.Errorf("verify.command empty: %+v", got.Verify)
	}
	if got.SourceURL != "https://getslatewave.com/themes/okayish" {
		t.Errorf("source_url = %q", got.SourceURL)
	}
	if len(got.SupportedOS) != 2 {
		t.Errorf("supported_os = %v, want 2 entries (default darwin, linux)", got.SupportedOS)
	}
}

// Unknown slug must surface "did you mean" via noManifestError. Test
// confirms the error path runs (and the suggestion lands when there's
// a near match).
func TestInfoCmd_UnknownSlugErrors(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestForInfo})

	err := infoCmd.RunE(infoCmd, []string{"okayis"}) // typo
	if err == nil {
		t.Fatal("expected error for unknown slug, got nil")
	}
	if !strings.Contains(err.Error(), "did you mean") {
		t.Errorf("expected `did you mean` hint, got: %v", err)
	}
}
