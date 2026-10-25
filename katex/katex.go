// Package katex renders TeX math to HTML and MathML with KaTeX, with no browser and no Node.js.
//
// The package embeds the minified script of one KaTeX release and runs it in goja, a JavaScript engine in pure Go. Render returns the same markup that katex.renderToString returns in a browser. The markup has two parts: HTML that looks the same in each browser, and MathML for screen readers. Thus a page shows its math with JavaScript off.
//
// The HTML part needs the style sheet and the fonts of the same release. Assets returns them. Serve them next to the pages, and link the style sheet from each page.
//
// Run go generate to change the release. The generator checks the tarball from npm against its published integrity.
package katex

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

//go:generate go run gen.go

// Version is the KaTeX release that the package embeds.
const Version = "0.19.1"

//go:embed katex.min.js
var script string

//go:embed assets
var assets embed.FS

// Assets returns the files that the HTML of Render needs: katex.min.css, the fonts in fonts/ and the licence of KaTeX.
func Assets() fs.FS {
	sub, _ := fs.Sub(assets, "assets")
	return sub
}

// Timeout is the longest time that one call of Render can take.
const Timeout = 5 * time.Second

// prelude makes a global object for the script and a render function with fixed options. Each call gets a new copy of the macros, so that a \gdef in one formula does not change the next formula.
const prelude = `var window = this, module = {exports: {}}, exports = module.exports;`

const glue = `
var m2katex = module.exports.renderToString ? module.exports : katex;
function m2render(tex, display, macros) {
	return m2katex.renderToString(tex, {
		displayMode: display,
		output: "htmlAndMathml",
		throwOnError: true,
		strict: "ignore",
		trust: false,
		maxExpand: 1000,
		macros: Object.assign({}, macros)
	});
}`

// Error is a formula that KaTeX cannot parse.
type Error struct {
	TeX     string // the formula
	Message string // the message of KaTeX
}

func (e *Error) Error() string { return e.Message }

// Renderer renders formulas with one set of macros. It keeps the result of each formula, so a formula that a site uses many times costs one render. Its methods are safe for concurrent use, but one Renderer renders one formula at a time.
type Renderer struct {
	mu     sync.Mutex
	vm     *goja.Runtime
	render goja.Callable
	macros goja.Value
	cache  map[key]result
}

type key struct {
	tex     string
	display bool
}

type result struct {
	html string
	err  error
}

// New returns a Renderer with the macros. A key of macros is a control sequence such as \R, and its value is the expansion, with #1 for the first argument.
func New(macros map[string]string) (*Renderer, error) {
	vm := goja.New()
	if _, err := vm.RunScript("katex.min.js", prelude+"\n"+script+"\n"+glue); err != nil {
		return nil, fmt.Errorf("katex: load KaTeX %s: %w", Version, err)
	}
	render, ok := goja.AssertFunction(vm.Get("m2render"))
	if !ok {
		return nil, errors.New("katex: the script has no render function")
	}
	m := map[string]any{}
	for k, v := range macros {
		m[k] = v
	}
	return &Renderer{vm: vm, render: render, macros: vm.ToValue(m), cache: map[key]result{}}, nil
}

// Render returns the HTML and MathML of tex. display selects display math, else the formula is inline. The error is an *Error when KaTeX cannot parse tex.
func (r *Renderer) Render(tex string, display bool) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key{tex, display}
	if res, ok := r.cache[k]; ok {
		return res.html, res.err
	}
	t := time.AfterFunc(Timeout, func() { r.vm.Interrupt("timeout") })
	v, err := r.render(goja.Undefined(), r.vm.ToValue(tex), r.vm.ToValue(display), r.macros)
	t.Stop()
	r.vm.ClearInterrupt()
	var res result
	switch {
	case err == nil:
		res.html = v.String()
	case isInterrupt(err):
		res.err = &Error{TeX: tex, Message: fmt.Sprintf("KaTeX took more than %s", Timeout)}
	default:
		res.err = &Error{TeX: tex, Message: message(err)}
	}
	r.cache[k] = res
	return res.html, res.err
}

func isInterrupt(err error) bool {
	var ie *goja.InterruptedError
	return errors.As(err, &ie)
}

// message returns the message of a JavaScript exception, without the stack.
func message(err error) string {
	var ex *goja.Exception
	if errors.As(err, &ex) {
		if o, ok := ex.Value().(*goja.Object); ok {
			if m := o.Get("message"); m != nil && !goja.IsUndefined(m) {
				return m.String()
			}
		}
		return strings.TrimSpace(ex.Value().String())
	}
	return err.Error()
}
