package katex_test

import (
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/katex"
)

func TestMathML(t *testing.T) {
	r, err := katex.New(map[string]string{`\R`: `\mathbb{R}`})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		tex      string
		display  bool
		want     []string
		dontWant []string
	}{
		{`x \in \R^n`, false, []string{`<math xmlns="http://www.w3.org/1998/Math/MathML">`, `<mi>ℝ</mi>`, `<annotation encoding="application/x-tex">x \in \R^n</annotation>`}, []string{`katex-html`, `mathvariant="double-struck"`}},
		{`\mathcal{O}_X \mathfrak{g} \mathbf{v} \boldsymbol{\alpha} \mathbb{1}`, false, []string{`<mi>𝒪</mi>`, `<mi>𝔤</mi>`, `<mi>𝐯</mi>`, `<mi>𝜶</mi>`, `𝟙`}, []string{`mathvariant`}},
		{`\mathcal{L}`, false, []string{`<mi>ℒ</mi>`}, nil},
		{`f'(a) + g''`, false, []string{`<mrow><mi>f</mi><mo mathvariant="normal" lspace="0em" rspace="0em">′</mo></mrow>`}, []string{`<msup>`}},
		{`x^2`, false, []string{`<msup><mi>x</mi><mn>2</mn></msup>`}, nil},
		{`\textbf{if } x`, false, []string{`<mtext style="font-weight:bold;">if</mtext>`}, nil},
		{`\operatorname{rank} A`, false, []string{`<mi mathvariant="normal">rank</mi>`}, nil},
		{`a < b`, true, []string{`display="block"`, `&lt;`}, nil},
	} {
		out, err := r.Render(c.tex, c.display)
		if err != nil {
			t.Fatal(err)
		}
		got := katex.MathML(out)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("MathML of %s has no %s:\n%s", c.tex, w, got)
			}
		}
		for _, w := range c.dontWant {
			if strings.Contains(got, w) {
				t.Errorf("MathML of %s has %s:\n%s", c.tex, w, got)
			}
		}
	}
	if got := katex.MathML(`<span>no math</span>`); got != "" {
		t.Errorf("MathML of HTML with no math element is %q", got)
	}
}
