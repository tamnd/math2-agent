package review

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/battery"
)

// WriteText writes the packet as text for a terminal of the width in columns. The informal statement and the Lean statement are side by side. A width under 60 gives 60.
func (p Packet) WriteText(w io.Writer, width int) error {
	width = max(width, 60)
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "%s %s: %s\n", p.Tag, p.Kind, p.Name)
	fmt.Fprintf(bw, "statement hash  %s\n", p.StatementHash)
	if p.Stale {
		fmt.Fprintln(bw, "The statement changed after the last build. Run m2 build before you review.")
	}
	if len(p.Bindings) == 0 {
		fmt.Fprintln(bw, "The node has no Lean binding.")
	}
	for _, b := range p.Bindings {
		fmt.Fprintln(bw)
		fmt.Fprintf(bw, "binding         %s\n", b.Decl)
		fmt.Fprintf(bw, "match           %s\n", b.Match)
		if b.Adapter != "" {
			fmt.Fprintf(bw, "adapter         %s\n", b.Adapter)
		}
		fmt.Fprintf(bw, "you review      %s\n", b.Checked)
		fmt.Fprintf(bw, "type hash       %s\n", or(b.TypeHash, "no record"))
		fmt.Fprintf(bw, "fidelity        %s\n", b.Fidelity)
		fmt.Fprintf(bw, "author          %s\n", or(b.Author, "not in the register"))
		if b.Error != "" {
			fmt.Fprintf(bw, "The build has no statement of %s: %s.\n", b.Checked, b.Error)
			continue
		}
		fmt.Fprintln(bw)
		sideBySide(bw, or(p.Section, "Statement"), p.Statement, "Lean", b.Lean, width)
		fmt.Fprintln(bw)
		fmt.Fprintln(bw, "Lean without notation")
		for _, l := range wrap(b.LeanAll, width-2) {
			fmt.Fprintln(bw, "  "+l)
		}
		if len(b.Constants) > 0 {
			fmt.Fprintln(bw)
			fmt.Fprintln(bw, "Constants")
		}
		for _, c := range b.Constants {
			nodes := "no node binds it"
			switch {
			case len(c.Nodes) == 0 && c.Structural:
				nodes = "logic or notation"
			case len(c.Nodes) > 0:
				var parts []string
				for _, n := range c.Nodes {
					parts = append(parts, fmt.Sprintf("%s %s (%s)", n.Tag, n.Name, or(n.Fidelity.String(), "unbound")))
				}
				nodes = "bound to " + strings.Join(parts, ", ")
			}
			fmt.Fprintf(bw, "  %s: %s\n", c.Name, nodes)
			if c.Type != "" {
				for _, l := range wrap(c.Type, width-6) {
					fmt.Fprintln(bw, "      "+l)
				}
			}
		}
		fmt.Fprintln(bw)
		if b.Battery == nil {
			fmt.Fprintln(bw, "Battery         not run")
		} else {
			writeBattery(bw, *b.Battery, width)
		}
		if len(b.Reviews) > 0 {
			fmt.Fprintln(bw)
			fmt.Fprintln(bw, "Reviews")
		}
		for _, r := range b.Reviews {
			fmt.Fprintf(bw, "  %s %s %s\n", r.ID, or(r.Reviewer, "-"), r.State)
		}
	}
	return bw.Flush()
}

func writeBattery(w io.Writer, r battery.Report, width int) {
	f1 := "no"
	if r.F1 {
		f1 = "yes"
	}
	fmt.Fprintf(w, "Battery         F1 %s\n", f1)
	for _, c := range r.Checks {
		res := string(c.Result)
		if c.Class != "" {
			res += " " + c.Class
		}
		fmt.Fprintf(w, "  %2d %-30s %s\n", c.Number, c.Name, res)
		// The back-translation, the critic and each check that did not pass show their detail.
		if c.Detail != "" && (c.Result != battery.Pass || c.Number == 2 || c.Number == 9) {
			for _, l := range wrap(c.Detail, width-6) {
				fmt.Fprintln(w, "      "+l)
			}
		}
	}
}

// sideBySide writes two texts in two columns with a rule between them.
func sideBySide(w io.Writer, lhead, left, rhead, right string, width int) {
	col := (width - 3) / 2
	ls := append([]string{lhead, strings.Repeat("-", min(col, utf8.RuneCountInString(lhead)))}, wrap(left, col)...)
	rs := append([]string{rhead, strings.Repeat("-", min(col, utf8.RuneCountInString(rhead)))}, wrap(right, col)...)
	for i := range max(len(ls), len(rs)) {
		var l, r string
		if i < len(ls) {
			l = ls[i]
		}
		if i < len(rs) {
			r = rs[i]
		}
		fmt.Fprintf(w, "%s%s | %s\n", l, strings.Repeat(" ", col-utf8.RuneCountInString(l)), r)
	}
}

// wrap breaks text into lines of at most width runes. It keeps the line breaks of the text, and it breaks a word that is longer than the width.
func wrap(text string, width int) []string {
	var out []string
	for para := range strings.Lines(text) {
		para = strings.TrimRight(para, "\n")
		indent := para[:len(para)-len(strings.TrimLeft(para, " "))]
		if utf8.RuneCountInString(indent) > width/2 {
			indent = ""
		}
		line := indent
		for _, word := range strings.Fields(para) {
			for utf8.RuneCountInString(word) > width-utf8.RuneCountInString(indent) {
				if strings.TrimSpace(line) != "" {
					out = append(out, line)
					line = indent
				}
				cut := runeIndex(word, width-utf8.RuneCountInString(indent))
				out = append(out, indent+word[:cut])
				word = word[cut:]
			}
			switch {
			case strings.TrimSpace(line) == "":
				line = indent + word
			case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) > width:
				out = append(out, line)
				line = indent + word
			default:
				line += " " + word
			}
		}
		out = append(out, line)
	}
	return out
}

// runeIndex returns the byte index of the rune n of s.
func runeIndex(s string, n int) int {
	i := 0
	for j := range s {
		if i == n {
			return j
		}
		i++
	}
	return len(s)
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
