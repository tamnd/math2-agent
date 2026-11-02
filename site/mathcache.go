package site

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/tamnd/math2-agent/katex"
	"github.com/tamnd/math2-agent/markup"
)

// mathCache keeps the rendered math of a build, by the TeX of each formula.
// KaTeX runs in a JavaScript interpreter, and it takes most of the time of a build. The site renders each body more than once (the whole body, the lead, the excerpt), and most formulas do not change from one build to the next.
// With Options.Cache, the build reads the cache from the file at the start and writes it at the end, with only the formulas of this build. The cache is for one KaTeX release and one set of macros: a change of either empties it.
type mathCache struct {
	mu   sync.Mutex
	key  string
	old  map[string]string // the formulas of the file
	used map[string]string // the formulas of this build
}

// mathFile is the content of the cache file.
type mathFile struct {
	Key  string
	Math map[string]string
}

func mathKey(tex string, display bool) string {
	if display {
		return "D" + tex
	}
	return "I" + tex
}

// loadMathCache reads the cache file. A missing or unreadable file, or a file for another release or other macros, gives an empty cache.
func loadMathCache(file string, macros map[string]string) *mathCache {
	m, _ := json.Marshal(macros)
	sum := sha256.Sum256(append([]byte(katex.Version+"\x00"), m...))
	c := &mathCache{key: hex.EncodeToString(sum[:]), old: map[string]string{}, used: map[string]string{}}
	if file == "" {
		return c
	}
	f, err := os.Open(file) // #nosec G304 -- the build user names the file.
	if err != nil {
		return c
	}
	defer func() { _ = f.Close() }()
	z, err := gzip.NewReader(f)
	if err != nil {
		return c
	}
	var mf mathFile
	if gob.NewDecoder(z).Decode(&mf) == nil && mf.Key == c.key && mf.Math != nil {
		c.old = mf.Math
	}
	return c
}

func (c *mathCache) get(tex string, display bool) (string, bool) {
	k := mathKey(tex, display)
	c.mu.Lock()
	defer c.mu.Unlock()
	if out, ok := c.used[k]; ok {
		return out, true
	}
	if out, ok := c.old[k]; ok {
		c.used[k] = out
		return out, true
	}
	return "", false
}

func (c *mathCache) put(tex string, display bool, out string) {
	c.mu.Lock()
	c.used[mathKey(tex, display)] = out
	c.mu.Unlock()
}

// save writes the formulas of this build to the file, through a temporary file so that a failed build leaves the old cache.
func (c *mathCache) save(file string) error {
	if file == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil { // #nosec G301 -- the cache holds public content.
		return err
	}
	tmp := file + ".tmp"
	f, err := os.Create(tmp) // #nosec G304 -- as in loadMathCache.
	if err != nil {
		return err
	}
	z := gzip.NewWriter(f)
	c.mu.Lock()
	err = gob.NewEncoder(z).Encode(mathFile{Key: c.key, Math: c.used})
	c.mu.Unlock()
	if err == nil {
		err = z.Close()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, file)
}

// prerender renders the formulas of the node bodies that are not in the cache, with one KaTeX renderer for each CPU, so that the pages then take each formula from the cache.
func (b *builder) prerender(macros map[string]string) error {
	work := make(chan *nodeView)
	n := runtime.NumCPU()
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := katex.New(macros)
			if err != nil {
				errs <- err
				for range work {
				}
				return
			}
			md := markup.New(func(tex string, display bool) string {
				if _, ok := b.mathMemo.get(tex, display); !ok {
					b.mathMemo.put(tex, display, renderMath(r, tex, display))
				}
				return ""
			})
			none := func(string) (markup.Target, bool) { return markup.Target{}, false }
			for v := range work {
				_, _ = md.Render(v.Body, none) // resolve reports the errors of the bodies
			}
		}()
	}
	for _, v := range b.order {
		work <- v
	}
	close(work)
	wg.Wait()
	close(errs)
	return <-errs
}
