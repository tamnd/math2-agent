package tag

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Current is the Until value of an alias assignment that is in effect.
const Current = "-"

// Alias is one line of registers/aliases.reg: the alias, the tag that it names, the date from which it names the tag and the date until which it names the tag. The dates have the form 2006-01-02. Until is Current while the assignment is in effect.
type Alias struct {
	Name  string
	Tag   string
	From  string
	Until string
}

// String returns the alias assignment as one register line.
func (a Alias) String() string { return a.Name + " " + a.Tag + " " + a.From + " " + a.Until }

var (
	aliasRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	dateRE  = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// IsAlias reports whether s has the form of an alias: words of lowercase letters and digits with a hyphen between them, for example lagrange-theorem-groups.
func IsAlias(s string) bool { return aliasRE.MatchString(s) }

// ReadAliases returns the alias assignments in the register at path, in file order. Blank lines and lines that start with # are not records. A file that does not exist has no assignments.
func ReadAliases(path string) ([]Alias, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the caller gives a path in the working tree.
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Alias
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		f := strings.Fields(sc.Text())
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if len(f) != 4 {
			return nil, fmt.Errorf("%s:%d: the line has %d fields, and an alias assignment has 4: alias, tag, from, until", path, n, len(f))
		}
		a := Alias{Name: f[0], Tag: f[1], From: f[2], Until: f[3]}
		switch {
		case !IsAlias(a.Name):
			return nil, fmt.Errorf("%s:%d: %q is not an alias", path, n, a.Name)
		case !IsTag(a.Tag):
			return nil, fmt.Errorf("%s:%d: %q is not a tag", path, n, a.Tag)
		case !dateRE.MatchString(a.From):
			return nil, fmt.Errorf("%s:%d: %q is not a date", path, n, a.From)
		case a.Until != Current && !dateRE.MatchString(a.Until):
			return nil, fmt.Errorf("%s:%d: %q is not a date or %s", path, n, a.Until, Current)
		}
		out = append(out, a)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}
