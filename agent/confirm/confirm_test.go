package confirm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent/confirm"
	"github.com/tamnd/math2-agent/agent/write"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/node"
)

type fake struct {
	answer string
	reqs   []*llm.Request
}

func (f *fake) Complete(_ context.Context, r *llm.Request) (*llm.Response, error) {
	f.reqs = append(f.reqs, r)
	return &llm.Response{Content: []llm.Block{llm.TextBlock(f.answer)}}, nil
}

func TestParse(t *testing.T) {
	for in, want := range map[string]string{
		"VERDICT: same\nREASON: it agrees.":           "same it agrees.",
		"**VERDICT:** Stronger.\n**REASON:** drops x": "stronger drops x",
		"I think they agree.":                         "unclear the answer gives no reason",
		"VERDICT: maybe\nREASON: hard":                "unclear hard",
	} {
		v, r := confirm.Parse(in)
		if got := v + " " + r; got != want {
			t.Errorf("Parse(%q) is %q, want %q", in, got, want)
		}
	}
}

func TestCheck(t *testing.T) {
	n, err := node.Parse("nodes/2VQ/2VQDK71A27.md", []byte("---\ntag: 2VQDK71A27\nkind: statement\nname: Widget lemma\n---\n\n## Statement\n\nEach widget is a gadget.\n\n## Proof\n\nClear.\n"))
	if err != nil {
		t.Fatal(err)
	}
	items := []write.SourceItem{
		{ID: "ax://paper/2401.00001#A", Paper: "2401.00001", Version: 2, Locator: "Lemma 1", Class: "open", Licence: "cc-by", Text: "Lemma 1. Every widget is a gadget."},
		{ID: "ax://paper/2402.00002#B", Paper: "2402.00002", Version: 1, Locator: "Theorem 3", Class: "record"},
	}
	f := &fake{answer: "VERDICT: same\nREASON: both say that a widget is a gadget."}
	rs, err := confirm.Check(context.Background(), f, "gpt-x", "claude-y", n, items)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Verdict != confirm.Same || rs[0].Model != "gpt-x" || rs[1].Verdict != confirm.Person || rs[1].Model != "" {
		t.Fatalf("the results are %+v", rs)
	}
	if len(f.reqs) != 1 {
		t.Fatalf("the check sent %d requests, and only the open item needs one", len(f.reqs))
	}
	msg := f.reqs[0].Messages[0].Content[0].Text
	if !strings.Contains(msg, "Each widget is a gadget.") || strings.Contains(msg, "Clear.") || !strings.Contains(msg, "Every widget is a gadget.") {
		t.Errorf("the request has the main section and the item text only:\n%s", msg)
	}
	report := string(confirm.Report(rs, "2026-11-03"))
	if !strings.Contains(report, "same: 1\n") || !strings.Contains(report, "person: 1\n") || !strings.Contains(report, "| 2VQDK71A27 | statement | Widget lemma | 2401.00001v2 Lemma 1 |") {
		t.Errorf("the report is\n%s", report)
	}
}

func TestMainSection(t *testing.T) {
	if got := confirm.MainSection("\n## Definition\n\nA is B.\n\n## Remarks\n\nC.\n"); got != "## Definition\n\nA is B." {
		t.Errorf("the main section is %q", got)
	}
	if got := confirm.MainSection("no section"); got != "" {
		t.Errorf("a body with no section gives %q", got)
	}
}
