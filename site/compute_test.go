package site

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/compute"
)

func TestRunPanel(t *testing.T) {
	opt := Options{Content: fixtures, Out: filepath.Join(t.TempDir(), "site"), URL: "https://example.org/math2", Repo: "o/math2", AgentRepo: "o/math2-agent", Version: "test", Build: t.TempDir()}
	if _, err := build.Run(context.Background(), build.Options{Root: fixtures, Out: opt.Build}); err != nil {
		t.Fatal(err)
	}
	out := "blake3:" + strings.Repeat("1", 64)
	run, err := json.Marshal(compute.Run{Tag: "ZNAZT5", Arch: "arm64", Runtime: "runsc", Network: "none", Output: out, Seconds: 2.25, Problem: "the record has no output hash"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, opt.Build, "compute/ZNAZT5/arm64/run.json", string(run))
	if err := Build(opt); err != nil {
		t.Fatal(err)
	}

	page := readFile(t, opt.Out, "node/ZNAZT5/index.html")
	for _, want := range []string{
		`<h2 id="run-h">Run this</h2>`,
		`<pre><code>m2 compute verify ZNAZT5</code></pre>`,
		`<a href="https://github.com/o/math2/blob/main/compute/ZNA/ZNAZT5/goldbach.py"><code>goldbach.py</code></a>`,
		`<code>{&#34;limit&#34;:10000}</code>`,
		`none yet. The first run records it.`,
		`<td>arm64</td><td><code>` + out + `</code></td><td>no</td><td>no: the record has no output hash</td><td>2.2</td>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page of ZNAZT5 has no %q", want)
		}
	}
	if page := readFile(t, opt.Out, "node/586J26/index.html"); !strings.Contains(page, "The build has no run of this computation.") || !strings.Contains(page, "<tr><th>Seed</th><td>0</td></tr>") {
		t.Error("the page of 586J26 does not say that the build has no run, or does not show the seed 0")
	}
	if page := readFile(t, opt.Out, "node/Q529ES/index.html"); strings.Contains(page, "Run this") {
		t.Error("a statement has a run panel")
	}
}
