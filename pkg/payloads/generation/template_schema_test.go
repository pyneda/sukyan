package generation

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A misspelled key unmarshals to the zero value instead of failing, so a
// template can silently lose its configuration. `detection_conditions: or` went
// unnoticed in 24 templates precisely because the zero value behaved like "or".
// KnownFields makes any unknown key a hard error.
func TestEmbeddedTemplatesHaveNoUnknownKeys(t *testing.T) {
	checked := 0
	err := fs.WalkDir(localTemplates, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ext := path.Ext(p); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		raw, err := localTemplates.ReadFile(p)
		if err != nil {
			t.Errorf("%s: cannot read: %v", p, err)
			return nil
		}
		dec := yaml.NewDecoder(strings.NewReader(string(raw)))
		dec.KnownFields(true)
		var g PayloadGenerator
		if err := dec.Decode(&g); err != nil {
			t.Errorf("%s: %v", p, err)
			return nil
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walking embedded templates: %v", err)
	}
	if checked == 0 {
		t.Fatal("no templates were checked")
	}
	t.Logf("validated %d embedded templates", checked)
}

// DetectionCondition must be explicitly "and" or "or"; an empty value silently
// behaves as "or" and hides typos in the key name.
func TestEmbeddedTemplatesDeclareValidDetectionCondition(t *testing.T) {
	gens, err := LoadGenerators("")
	if err != nil {
		t.Fatalf("LoadGenerators: %v", err)
	}
	for _, g := range gens {
		if len(g.DetectionMethods) < 2 {
			continue
		}
		switch g.DetectionCondition {
		case And, Or:
		default:
			t.Errorf("template %q has %d detection methods but detection_condition=%q; "+
				"declare 'and' or 'or' explicitly", g.ID, len(g.DetectionMethods), g.DetectionCondition)
		}
	}
}
