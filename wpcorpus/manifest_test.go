package wpcorpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	m, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if m.TemplatesVersion != 3 || m.HeadingsVersion != 2 || m.WaiversVersion != 1 {
		t.Errorf("versions %d %d %d", m.TemplatesVersion, m.HeadingsVersion, m.WaiversVersion)
	}
	if tpl := m.Template("thm"); tpl == nil || tpl.Key != "math theorem" {
		t.Errorf("the alias thm gives %+v", tpl)
	}
	if tpl := m.Template("cite journal"); tpl == nil || tpl.Class != ClassCitation {
		t.Errorf("the pattern cite gives %+v", tpl)
	}
	if tpl := m.Template("no such template"); tpl != nil {
		t.Errorf("an unmapped key gives %+v", tpl)
	}
	tpl := m.Template("math theorem")
	if got := tpl.Param("text", "math_statement", "1"); got != "math_statement" {
		t.Errorf("Param(text) = %q", got)
	}
	if got := tpl.Param("name"); got != "name" {
		t.Errorf("Param(name) = %q", got)
	}
	if got := tpl.Param("link"); got != "" {
		t.Errorf("Param(link) = %q", got)
	}
}

func TestHeadingRole(t *testing.T) {
	m, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ heading, title, want string }{
		{"Definition", "Widget", "item/definition"},
		{"Definitions", "Widget", "item/definition"},
		{"The formal definition", "Widget", "item/definition"},
		{"Proof of the widget theorem", "Widget theorem", "item/proof"},
		{"Proofs", "Widget", "item/proof"},
		{"Statement of the theorem", "Widget", ""},
		{"See also", "Widget", "section/back"},
		{"B", "Glossary of widgets", "section/index"},
		{"Other uses", "Widget", ""},
	}
	for _, tt := range tests {
		if got := m.HeadingRole(tt.heading, tt.title); got != tt.want {
			t.Errorf("HeadingRole(%q, %q) = %q, want %q", tt.heading, tt.title, got, tt.want)
		}
	}
}

func TestWaive(t *testing.T) {
	m, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if w := m.Waive("", "item/example", "article"); w == nil || w.ID != "example-in-test" {
		t.Errorf("Waive(item/example) = %+v", w)
	}
	if w := m.Waive("", "item/proof", "article"); w != nil {
		t.Errorf("Waive(item/proof) = %+v", w)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name, templates, headings, waivers, want string
	}{
		{"bad class", "templates:\n  x:\n    class: thing\n", "", "", "unknown class"},
		{"bad cue", "templates:\n  x:\n    class: item\n    cue: weak\n", "", "", "cue must be"},
		{"alias twice", "templates:\n  x:\n    class: item\n    aliases: [y]\n  y:\n    class: nav\n", "", "", "is also in"},
		{"key and alias", "templates:\n  x:\n    class: item\n    aliases: [X]\n", "", "", "is also in"},
		{"bad pattern", "templates: {}\npatterns:\n  - prefix: a\n    suffix: b\n    class: nav\n", "", "", "one of prefix and suffix"},
		{"bad role", "templates: {}\n", "roles:\n  proof: [proof]\n", "", "must start with"},
		{"fold conflict", "templates: {}\n", "roles:\n  section/works: [works]\n  section/biography: [work]\n", "", "is in"},
		{"bad regexp", "templates: {}\n", "patterns:\n  - match: '('\n    role: section/index\n", "", "pattern 1"},
		{"no reason", "templates: {}\n", "roles: {}\n", "waivers:\n  - id: a\n    pattern: {heading: item/proof}\n    author: x\n", "reason and an author"},
		{"no pattern", "templates: {}\n", "roles: {}\n", "waivers:\n  - id: a\n    reason: r\n    author: x\n", "no template and no heading"},
		{"id twice", "templates: {}\n", "roles: {}\n", "waivers:\n  - {id: a, pattern: {heading: item/proof}, reason: r, author: x}\n  - {id: a, pattern: {heading: item/proof}, reason: r, author: x}\n", "used twice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, ManifestDir)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.headings == "" {
				tt.headings = "roles: {}\n"
			}
			files := map[string]string{"templates.yaml": tt.templates, "headings.yaml": tt.headings}
			if tt.waivers != "" {
				files["waivers.yaml"] = tt.waivers
			}
			for name, body := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Load(root)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load error %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadNoWaivers(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ManifestDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"templates.yaml": "version: 1\ntemplates: {}\n", "headings.yaml": "version: 1\nroles: {}\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, err := Load(root)
	if err != nil || len(m.Waivers) != 0 {
		t.Errorf("Load with no waivers.yaml: %v", err)
	}
}

func TestNormalHeading(t *testing.T) {
	tests := []struct{ heading, title, want string }{
		{"Proof of Lagrange's theorem", "Lagrange's theorem (group theory)", "proof"},
		{"The  Statement", "X", "statement"},
		{"Applications in physics", "X", "applications in physics"},
		{"Definition:", "X", "definition"},
	}
	for _, tt := range tests {
		if got := normalHeading(tt.heading, tt.title); got != tt.want {
			t.Errorf("normalHeading(%q, %q) = %q, want %q", tt.heading, tt.title, got, tt.want)
		}
	}
	for in, want := range map[string]string{"proofs": "proof", "properties": "property", "class": "class", "basis": "basis", "torus": "torus"} {
		if got := fold(in); got != want {
			t.Errorf("fold(%q) = %q, want %q", in, got, want)
		}
	}
}
