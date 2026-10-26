package site

import (
	"encoding/json"
	"strings"
	"testing"
)

const provStatement = `---
tag: 7K3QX2
kind: statement
name: Lagrange's theorem
msc: [20D60]
same_as:
  wikidata: Q505798
---

## Statement

The order of a subgroup of a finite group divides the order of the group.

## Explanations

### L1 agent

A small group fits into a big group a whole number of times.

### L2 agent

The cosets of the subgroup split the group into parts of the same size.
`

const provLog = `{"field":"*","op":"add","agent":null,"human":"github:writer","at":"2026-09-01T10:00:00Z"}
{"field":"body.explanations.L1","op":"add","agent":"m2-agent/explain@1.2.0","model":"claude-opus-5-5","prompt":"explain/7","run":"r-2026-09-02-1","human":null,"at":"2026-09-02T10:00:00Z"}
{"field":"body.explanations.L2","op":"add","agent":"m2-agent/explain@1.2.0","model":"claude-opus-5-5","prompt":"explain/7","run":"r-2026-09-02-1","human":null,"at":"2026-09-02T10:00:00Z"}
{"field":"same_as.wikidata","op":"add","agent":"m2-agent/align@0.3.0","run":"r-2026-09-02-2","human":null,"at":"2026-09-02T11:00:00Z"}
{"field":"body.explanations.L2","op":"quarantine","agent":null,"human":"orcid:0000-0002-1825-0097","at":"2026-09-05T10:00:00Z"}
{"field":"same_as.wikidata","op":"quarantine","agent":null,"human":"orcid:0000-0002-1825-0097","at":"2026-09-05T10:00:00Z"}
`

func TestProvenance(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/7K3/7K3QX2.md", provStatement)
	writeFile(t, content, "prov/7K3/7K3QX2.jsonl", provLog)
	out := buildSite(t, content, "")

	page := readNode(t, out, "7K3QX2")
	for _, want := range []string{
		"This page does not show the explanation L2 and the field same_as.wikidata.",
		"no person checked it yet:</b> the explanation L1.",
		"A small group fits",
		`<a href="https://github.com/o/math2/blob/main/prov/7K3/7K3QX2.jsonl">provenance log</a>`,
		`<a href="https://github.com/writer">writer</a>`,
		`<code>m2-agent/explain@1.2.0</code> <span class="muted">agent run r-2026-09-02-1</span>`,
		`<a href="https://orcid.org/0000-0002-1825-0097">ORCID 0000-0002-1825-0097</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not have %q", want)
		}
	}
	for _, hidden := range []string{"The cosets of the subgroup", "Q505798"} {
		if strings.Contains(page, hidden) {
			t.Errorf("the page shows %q, which is in quarantine", hidden)
		}
	}

	md := readFile(t, out, "node/7K3QX2.md")
	if strings.Contains(md, "The cosets") || strings.Contains(md, "Q505798") || !strings.Contains(md, "### L2 agent\n\n"+quarantineNotice+"\n") || !strings.Contains(md, "msc: [20D60]\n") {
		t.Errorf("the Markdown copy is:\n%s", md)
	}

	var j struct {
		Front map[string]any `json:"front"`
		Body  string         `json:"body"`
		Prov  struct {
			Log        string   `json:"log"`
			Unchecked  []string `json:"unchecked"`
			Quarantine []string `json:"quarantine"`
		} `json:"prov"`
	}
	if err := json.Unmarshal([]byte(readFile(t, out, "api/node/7K3QX2.json")), &j); err != nil {
		t.Fatal(err)
	}
	if _, ok := j.Front["same_as"]; ok || strings.Contains(j.Body, "The cosets") {
		t.Errorf("the JSON view shows the text in quarantine: %v", j.Front)
	}
	if strings.Join(j.Prov.Unchecked, " ") != "body.explanations.L1" || strings.Join(j.Prov.Quarantine, " ") != "body.explanations.L2 same_as.wikidata" {
		t.Errorf("the JSON view has the provenance %+v", j.Prov)
	}
	if llms := readFile(t, out, "llms-full.txt"); strings.Contains(llms, "The cosets") {
		t.Error("llms-full.txt shows the text in quarantine")
	}
}

func TestNoProvenance(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/7K3/7K3QX2.md", provStatement)
	out := buildSite(t, content, "")
	if page := readNode(t, out, "7K3QX2"); strings.Contains(page, "History") || !strings.Contains(page, "The cosets") {
		t.Error("a node with no log has a history, or hides text")
	}
	if md := readFile(t, out, "node/7K3QX2.md"); md != provStatement {
		t.Errorf("the Markdown copy is not the file:\n%s", md)
	}
}
