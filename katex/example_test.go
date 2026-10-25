package katex_test

import (
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/katex"
)

func ExampleRenderer_Render() {
	r, err := katex.New(map[string]string{`\R`: `\mathbb{R}`})
	if err != nil {
		panic(err)
	}
	html, err := r.Render(`x \in \R`, false)
	if err != nil {
		panic(err)
	}
	fmt.Println(strings.Contains(html, `<mi mathvariant="double-struck">R</mi>`))

	_, err = r.Render(`\frac{1}`, false)
	fmt.Println(err)
	// Output:
	// true
	// KaTeX parse error: Unexpected end of input in a macro argument, expected '}' at end of input: \frac{1}
}
