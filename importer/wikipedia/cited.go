package wikipedia

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// CitedDirs are the directories of a math2 tree that Cited reads.
var CitedDirs = []string{"nodes", "topics", "registers", "manifests", "records"}

// wikiURL finds a link to an article of the English Wikipedia.
var wikiURL = regexp.MustCompile(`https?://en\.(?:m\.)?wikipedia\.org/wiki/([^\s"<>\]|]+)`)

// Cited returns the Wikipedia titles that the tree at root cites, for the signal S4 of WP1.2. It reads the YAML files, the front matter of the Markdown files and the JSON and JSONL files under CitedDirs. A title comes from a key wikipedia (a string or a list), from a close_to entry with the system wikipedia, and from a link to en.wikipedia.org. Cited parses each file, so a title keeps its apostrophes and parentheses. It skips the files under a directory wikipedia, because the importer writes them. Cited also returns the number of files that did not parse.
func Cited(root string) ([]string, int, error) {
	seen := map[string]bool{}
	bad := 0
	add := func(t string) {
		if t, ok := citedTitle(t); ok {
			seen[t] = true
		}
	}
	for _, d := range CitedDirs {
		err := filepath.WalkDir(filepath.Join(root, d), func(p string, e fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if e.IsDir() {
				if e.Name() == "wikipedia" {
					return filepath.SkipDir
				}
				return nil
			}
			ok, err := citedFile(p, add)
			if err != nil {
				return err
			}
			if !ok {
				bad++
			}
			return nil
		})
		if err != nil {
			return nil, 0, err
		}
	}
	return slices.Sorted(maps.Keys(seen)), bad, nil
}

// citedFile gives the titles of one file to add. It reports false when the file does not parse.
func citedFile(p string, add func(string)) (bool, error) {
	ext := filepath.Ext(p)
	if ext != ".md" && ext != ".yaml" && ext != ".yml" && ext != ".json" && ext != ".jsonl" {
		return true, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false, err
	}
	switch ext {
	case ".md":
		for _, m := range wikiURL.FindAllSubmatch(b, -1) {
			add(trimLink(string(m[1])))
		}
		if rest, ok := bytes.CutPrefix(b, []byte("---\n")); ok {
			if i := bytes.Index(rest, []byte("\n---")); i >= 0 {
				var v any
				if yaml.Unmarshal(rest[:i], &v) != nil {
					return false, nil
				}
				citedWalk(v, add)
			}
		}
	case ".yaml", ".yml":
		dec := yaml.NewDecoder(bytes.NewReader(b))
		for {
			var v any
			err := dec.Decode(&v)
			if err == io.EOF {
				break
			}
			if err != nil {
				return false, nil
			}
			citedWalk(v, add)
		}
	case ".json":
		var v any
		if json.Unmarshal(b, &v) != nil {
			return false, nil
		}
		citedWalk(v, add)
	case ".jsonl":
		sc := bufio.NewScanner(bytes.NewReader(b))
		sc.Buffer(nil, 1<<26)
		for sc.Scan() {
			if len(bytes.TrimSpace(sc.Bytes())) == 0 {
				continue
			}
			var v any
			if json.Unmarshal(sc.Bytes(), &v) != nil {
				return false, nil
			}
			citedWalk(v, add)
		}
	}
	return true, nil
}

// citedWalk gives the titles in a parsed value to add.
func citedWalk(v any, add func(string)) {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			switch {
			case k == "wikipedia":
				if s, ok := x.(string); ok {
					add(s)
				}
				if l, ok := x.([]any); ok {
					for _, y := range l {
						if s, ok := y.(string); ok {
							add(s)
						}
					}
				}
			case k == "close_to":
				if l, ok := x.([]any); ok {
					for _, c := range l {
						if m, ok := c.(map[string]any); ok && m["system"] == "wikipedia" {
							if s, ok := m["id"].(string); ok {
								add(s)
							}
						}
					}
				}
			}
			citedWalk(x, add)
		}
	case []any:
		for _, x := range v {
			citedWalk(x, add)
		}
	case string:
		for _, m := range wikiURL.FindAllStringSubmatch(v, -1) {
			add(trimLink(m[1]))
		}
	}
}

// trimLink removes the closing parentheses of a Markdown link from a title in a link. It also removes a comma or a semicolon at the end, and a full stop after a parenthesis.
func trimLink(t string) string {
	for {
		switch {
		case strings.HasSuffix(t, ",") || strings.HasSuffix(t, ";") || strings.HasSuffix(t, ")."):
			t = t[:len(t)-1]
		case strings.HasSuffix(t, ")") && strings.Count(t, ")") > strings.Count(t, "("):
			t = t[:len(t)-1]
		default:
			return t
		}
	}
}

// citedTitle gives the form of a cited title in the dump. It removes the section, decodes the URL escapes and the HTML entities, and reports false for a title outside the main namespace.
func citedTitle(t string) (string, bool) {
	if u, err := url.PathUnescape(t); err == nil {
		t = u
	}
	t = html.UnescapeString(t)
	t, _, _ = strings.Cut(t, "#")
	if ns, _, ok := strings.Cut(t, ":"); ok {
		switch ns {
		case "Category", "File", "Template", "Wikipedia", "Help", "Portal", "Special", "Talk", "User":
			return "", false
		}
	}
	return title(t)
}
