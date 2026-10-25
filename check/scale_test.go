package check_test

import (
	"bytes"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/tag"
)

// synthetic returns n node files. Each is a copy of a node in the valid set with a new tag. The links and the edges of each copy resolve against the valid set.
func synthetic(t testing.TB, n int) ([]check.File, check.Index) {
	t.Helper()
	valid := readTree(t, validRoot)
	ix := index(t, validRoot)
	used := map[string]bool{}
	for id := range ix {
		used[id] = true
	}
	r := rand.NewChaCha8([32]byte{})
	var files []check.File
	for i := 0; len(files) < n; i++ {
		src := valid[i%len(valid)]
		old := strings.TrimSuffix(src.Path[strings.LastIndexByte(src.Path, '/')+1:], ".md")
		if tag.IsProvisional(old) || bytes.Contains(src.Data, []byte("\nretired:")) {
			continue
		}
		id, err := tag.Mint(r, func(s string) bool { return used[s] })
		if err != nil {
			t.Fatal(err)
		}
		used[id] = true
		data := bytes.Replace(src.Data, []byte("tag: "+old+"\n"), []byte("tag: "+id+"\n"), 1)
		files = append(files, check.File{Path: "nodes/" + tag.Shard(id) + "/" + id + ".md", Data: data})
	}
	return files, ix
}

// TestScale checks a tree of 10,000 synthetic nodes. Run it with -v to see the time.
func TestScale(t *testing.T) {
	if testing.Short() {
		t.Skip("the scale test is slow")
	}
	files, ix := synthetic(t, 10000)
	var c check.Checker
	start := time.Now()
	rep := c.Check(files, ix)
	took := time.Since(start)
	for _, f := range rep.Findings[:min(len(rep.Findings), 5)] {
		t.Error(f)
	}
	t.Logf("m2 check on %d synthetic nodes took %v", rep.Files, took.Round(time.Millisecond))
}

func BenchmarkCheck(b *testing.B) {
	files, ix := synthetic(b, 1000)
	var c check.Checker
	for b.Loop() {
		c.Check(files, ix)
	}
}
