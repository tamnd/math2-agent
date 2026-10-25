package bump

import (
	"fmt"
	"path"
	"strings"
	"time"
)

// Report is the bump report of spec 07.9. Over months, the reports measure the cost to stay current with Mathlib.
type Report struct {
	Date     string // the date of the bump, YYYY-MM-DD
	From, To Pins
	Early    bool // the bump is to Mathlib master: an early-warning build that never merges
	Breaks   []Break
	Changed  int      // the node files that the renames changed
	Stale    []string // the nodes whose fidelity level went down
	Upstream []Catchup
	Audit    string        // the summary of the audit, or empty when the audit did not run
	Green    bool          // the build and the audit pass with the new pins
	Took     time.Duration // the time from the new pins to the end of the last build
}

// Path returns the path of the report from the tree root.
func (r *Report) Path() string { return path.Join("reports", "bump", r.Date+".md") }

// Count returns the number of breaks of the kind.
func (r *Report) Count(k Kind) int {
	n := 0
	for _, b := range r.Breaks {
		if b.Kind == k {
			n++
		}
	}
	return n
}

// Markdown returns the report as Markdown.
func (r *Report) Markdown() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bump of %s\n\n", r.Date)
	fmt.Fprintf(&b, "Mathlib moves from %s to %s. Lean moves from %s to %s.", pin(r.From), pin(r.To), r.From.Toolchain, r.To.Toolchain)
	if r.Early {
		b.WriteString(" This is an early-warning build of Mathlib master. Do not merge it.")
	}
	b.WriteString("\n\n")
	switch {
	case r.Green:
		fmt.Fprintf(&b, "The build and the audit pass with the new pins. The time to green is %s.\n\n", r.Took.Round(time.Second))
	case r.Count(Broken) > 0:
		b.WriteString("The build fails. Each error in the Math2 code is a prove job of high priority. A release cannot pin the new Mathlib until these errors are fixed.\n\n")
	default:
		b.WriteString("The build passes and the audit fails. Look at the audit report.\n\n")
	}
	if r.Audit != "" {
		fmt.Fprintf(&b, "Audit: %s\n\n", r.Audit)
	}

	b.WriteString("## Breaks\n\n| Kind | Count | What happens |\n|---|---|---|\n")
	for _, k := range kinds {
		fmt.Fprintf(&b, "| %s | %d | %s |\n", k, r.Count(k), action[k])
	}
	b.WriteString("\n")
	if len(r.Breaks) > 0 {
		b.WriteString("| Kind | Node or file | Old | New | Note |\n|---|---|---|---|---|\n")
		for _, x := range r.Breaks {
			where := x.Tag
			if x.Kind == Broken {
				where = fmt.Sprintf("%s:%d", x.File, x.Line)
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", x.Kind, where, code(x.Old), code(x.New), cell(x.Text))
		}
		b.WriteString("\n")
	}
	if r.Changed > 0 {
		fmt.Fprintf(&b, "The renames changed %s.\n\n", count(r.Changed, "node file", "node files"))
	}

	b.WriteString("## Stale reviews\n\n")
	if len(r.Stale) == 0 {
		b.WriteString("No node has a lower fidelity level.\n\n")
	} else {
		fmt.Fprintf(&b, "%s a lower fidelity level, because the type of a binding changed (rule F02). It goes into the fidelity queue: %s.\n\n", count(len(r.Stale), "node has", "nodes have"), strings.Join(r.Stale, ", "))
	}

	b.WriteString("## Upstream\n\n")
	if len(r.Upstream) == 0 {
		b.WriteString("No Math2 declaration has the type hash of a Mathlib declaration.\n")
	} else {
		b.WriteString("These Math2 declarations have the type hash of a Mathlib declaration. Bind the node to Mathlib and deprecate the Math2 copy.\n\n| Node | Math2 | Mathlib |\n|---|---|---|\n")
		for _, c := range r.Upstream {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", c.Tag, code(c.Decl), code(strings.Join(c.Mathlib, ", ")))
		}
	}
	return []byte(b.String())
}

// action tells what happens with a break of each kind.
var action = map[Kind]string{
	Deprecated: "the bump binds the new name",
	Renamed:    "the bump binds the declaration with the same type hash",
	Refactored: "the reviews are stale, and the node goes into the fidelity queue",
	Lost:       "a person finds the new binding",
	Broken:     "a prove job repairs the Math2 code",
}

// pin returns the tag and the short commit of the Mathlib pins.
func pin(p Pins) string {
	if len(p.Commit) >= 12 {
		return fmt.Sprintf("%s (%s)", p.Tag, p.Commit[:12])
	}
	return p.Tag
}

// code returns s as Markdown code, or empty.
func code(s string) string {
	if s == "" {
		return ""
	}
	return "`" + strings.ReplaceAll(s, "`", "") + "`"
}

// cell returns s on one line, for a cell of a table.
func cell(s string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(s), " "), "|", `\|`)
}

// count returns n and the singular or the plural.
func count(n int, one, more string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, more)
}
