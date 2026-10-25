package format

import (
	"regexp"
	"strings"
)

// The code rules in this file are the rules of m2 check. A link in a code block or in a code span is text, and not a link.

var wikiLink = regexp.MustCompile(`\[\[(.*?)\]\]`)

// codeLines reports for each line whether it is in a fenced code block. The fence lines are in the block.
func codeLines(lines []string) []bool {
	in := make([]bool, len(lines))
	fence := ""
	for i, l := range lines {
		trim := strings.TrimLeft(l, " ")
		switch {
		case fence != "":
			in[i] = true
			if strings.HasPrefix(trim, fence) && strings.Trim(trim, fence[:1]) == "" {
				fence = ""
			}
		case strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~"):
			in[i] = true
			fence = trim[:3]
		}
	}
	return in
}

// maskSpans changes each code span in the line to spaces. The length of the line does not change.
func maskSpans(l string) string {
	if !strings.Contains(l, "`") {
		return l
	}
	out := []byte(l)
	for i := 0; i < len(l); {
		if l[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(l) && l[j] == '`' {
			j++
		}
		end := strings.Index(l[j:], l[i:j])
		if end < 0 {
			i = j
			continue
		}
		end += j + j - i
		for k := i; k < end; k++ {
			out[k] = ' '
		}
		i = end
	}
	return string(out)
}
