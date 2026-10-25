// Package msc reads MSC2020 codes and the topic map of math2 (spec 18.1).
//
// MSC2020 is the Mathematics Subject Classification of zbMATH Open and the AMS. It has three levels. A top-level code such as 11-XX names a class. A second-level code such as 11Axx names an area of the class. The codes 11-00 to 11-11 are also at the second level, and they name a kind of work in the class, such as a textbook. A third-level code such as 11A41 names a topic of an area.
//
// The topic map is the file topics/msc2020.yaml of the math2 tree. It gives each code a short label in our own words, and it can give the tag of the topic node of the code. The official descriptions of the codes are under CC BY-NC-SA, thus the map does not have them (spec 16.6).
package msc

import (
	"regexp"
	"strings"
)

var code = regexp.MustCompile(`^\d\d(?:-XX|-\d\d|[A-Z]xx|[A-Z]\d\d)$`)

// Valid reports whether s has the form of an MSC2020 code: 11-XX, 11-02, 11Axx or 11A41. Valid checks the form only. The topic map tells which codes exist.
func Valid(s string) bool {
	return code.MatchString(s)
}

// Level returns the level of the code in the hierarchy: 1 for 11-XX, 2 for 11Axx and 11-02, and 3 for 11A41. It returns 0 when the code is not valid.
func Level(c string) int {
	switch {
	case !Valid(c):
		return 0
	case strings.HasSuffix(c, "-XX"):
		return 1
	case c[2] == '-' || strings.HasSuffix(c, "xx"):
		return 2
	}
	return 3
}

// Parent returns the code one level above the code: 11Axx for 11A41, and 11-XX for 11Axx and 11-02. It returns the empty string for a top-level code and for a code that is not valid.
func Parent(c string) string {
	switch Level(c) {
	case 2:
		return c[:2] + "-XX"
	case 3:
		return c[:3] + "xx"
	}
	return ""
}

// Class returns the top-level code of the class of the code, for example 11-XX for 11A41. It returns the empty string for a code that is not valid.
func Class(c string) string {
	if !Valid(c) {
		return ""
	}
	return c[:2] + "-XX"
}

// Compare compares two codes in the order of the classification: a code comes before the codes under it, and 11-XX, 11-00, 11Axx, 11A05 and 11Bxx are in order. The result is -1, 0 or +1.
func Compare(a, b string) int {
	return strings.Compare(sortKey(a), sortKey(b))
}

// sortKey drops the -XX and xx ends, because the hyphen sorts before the letters and an empty end sorts before the digits.
func sortKey(c string) string {
	if Level(c) == 3 || c == "" {
		return c
	}
	return strings.TrimSuffix(strings.TrimSuffix(c, "-XX"), "xx")
}
