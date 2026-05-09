package manifest

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// schemaPath is the canonical JSON Schema for slatewave manifests. Lives
// at the repo root so editor tooling and contributors can reference it
// via $schema; tests load it relative to this package.
const schemaPath = "../../schemas/manifest.schema.json"

// TestEmbeddedManifests_ValidateAgainstSchema TOML→JSON-roundtrips every
// embedded manifest and validates it against the schema at schemas/
// manifest.schema.json. Catches drift in two directions:
//
//   - A manifest that adds a typo'd field name slips past the Go struct
//     parser silently (BurntSushi/toml ignores unknown fields), so without
//     a schema check the CLI ships a manifest whose author thinks they
//     set "destination" but actually set nothing.
//
//   - A struct field added without a schema entry passes the Go parser
//     but fails the schema. Forces schema and code to evolve together.
//
// Pure-Go: no external CLI tools, runs as part of `go test ./...` so
// every PR exercises it. The CI workflow also wires `make validate-
// manifests` to run this test by name for an explicit-named gate.
func TestEmbeddedManifests_ValidateAgainstSchema(t *testing.T) {
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schemaDoc any
	if err := json.Unmarshal(schemaData, &schemaDoc); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaPath, schemaDoc); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	schema, err := c.Compile(schemaPath)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	err = fs.WalkDir(EmbeddedManifests, "embedded", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".toml") {
			return nil
		}
		data, err := fs.ReadFile(EmbeddedManifests, path)
		if err != nil {
			return err
		}
		// TOML → Go map → JSON. Going through a raw map (instead of the
		// Theme struct) preserves unknown fields so the schema check
		// catches typos the struct parser would have silently dropped.
		var raw map[string]any
		if _, err := toml.Decode(string(data), &raw); err != nil {
			t.Errorf("%s: parse TOML: %v", path, err)
			return nil
		}
		var jsonBuf bytes.Buffer
		if err := json.NewEncoder(&jsonBuf).Encode(raw); err != nil {
			t.Errorf("%s: marshal JSON: %v", path, err)
			return nil
		}
		var doc any
		if err := json.NewDecoder(&jsonBuf).Decode(&doc); err != nil {
			t.Errorf("%s: re-parse JSON: %v", path, err)
			return nil
		}
		if err := schema.Validate(doc); err != nil {
			t.Errorf("%s: schema violation:\n%v", filepath.Base(path), err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
