package katex_test

import (
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/tamnd/math2-agent/katex"
)

func renderer(t *testing.T) *katex.Renderer {
	t.Helper()
	r, err := katex.New(map[string]string{`\Z`: `\mathbb{Z}`, `\abs`: `\left|#1\right|`})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRender(t *testing.T) {
	r := renderer(t)
	for _, c := range []struct {
		tex     string
		display bool
		want    []string
	}{
		{`\abs{x} \in \Z`, false, []string{`<math xmlns="http://www.w3.org/1998/Math/MathML">`, `<mi mathvariant="double-struck">Z</mi>`, `class="katex-html" aria-hidden="true"`, `<annotation encoding="application/x-tex">\abs{x} \in \Z</annotation>`}},
		{`\sum_{k=1}^n k`, true, []string{`class="katex-display"`, `<math xmlns="http://www.w3.org/1998/Math/MathML" display="block">`}},
		{`a < b`, false, []string{`<mo>&lt;</mo>`}},
	} {
		got, err := r.Render(c.tex, c.display)
		if err != nil {
			t.Errorf("%s: %v", c.tex, err)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s gives %s, with no %s", c.tex, got, w)
			}
		}
	}
}

func TestError(t *testing.T) {
	r := renderer(t)
	for _, tex := range []string{`\nosuchmacro{x}`, `{a`, `a & b`} {
		_, err := r.Render(tex, false)
		var e *katex.Error
		if !errors.As(err, &e) || e.TeX != tex || !strings.Contains(e.Message, "KaTeX parse error") {
			t.Errorf("%s gives the error %v", tex, err)
		}
	}
	// The renderer does not trust a formula with a link.
	if html, err := r.Render(`\href{javascript:alert(1)}{x}`, false); err != nil || strings.Contains(html, "<a ") {
		t.Errorf("a link gives %s, %v", html, err)
	}
	// A macro that expands without end stops at the limit.
	if _, err := r.Render(`\def\a{\a\a}\a`, false); err == nil {
		t.Error("a macro bomb gives no error")
	}
	// The renderer works after an error.
	if _, err := r.Render(`x`, false); err != nil {
		t.Error(err)
	}
}

// A \gdef in one formula must not change the next formula.
func TestIsolation(t *testing.T) {
	r := renderer(t)
	if _, err := r.Render(`\gdef\foo{1}\foo`, false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Render(`\foo`, false); err == nil {
		t.Error(`\foo is still defined after the formula that defined it`)
	}
}

func TestConcurrent(t *testing.T) {
	r := renderer(t)
	want, err := r.Render(`\frac{1}{2}`, false)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			got, err := r.Render(`\frac{1}{2}`, i%2 == 0)
			if err != nil || (i%2 == 1 && got != want) {
				t.Errorf("render %d gives %v", i, err)
			}
		})
	}
	wg.Wait()
}

// Each font that the style sheet uses is in Assets.
func TestAssets(t *testing.T) {
	a := katex.Assets()
	css, err := fs.ReadFile(a, "katex.min.css")
	if err != nil {
		t.Fatal(err)
	}
	urls := regexp.MustCompile(`url\(([^)]+)\)`).FindAllSubmatch(css, -1)
	if len(urls) < 20 {
		t.Fatalf("the style sheet has %d fonts", len(urls))
	}
	for _, u := range urls {
		if _, err := fs.Stat(a, string(u[1])); err != nil {
			t.Errorf("the style sheet uses %s: %v", u[1], err)
		}
	}
	if _, err := fs.Stat(a, "LICENSE"); err != nil {
		t.Error(err)
	}
}
