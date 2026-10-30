// Package ids holds the normal forms and the checks of the external ids of a resource (spec corpus resources RS3.2 and RS3.3).
//
// Normalize takes an id as a source gives it, for example with a URL prefix, in a different case or with hyphens, and returns the normal form that the store keeps. Check reports whether a value is already in its normal form and passes its check digit. Rule H03 calls Check for each id of each resource.
package ids

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ErrUnknown is the error for a key that is not a system of RS3.2.
var ErrUnknown = errors.New("ids: unknown system")

type system struct {
	strip []string // prefixes to remove, compared with no case
	fold  func(string) string
	re    *regexp.Regexp
	check func(string) error
}

var (
	digits = regexp.MustCompile(`^[1-9][0-9]*$`)
	native = regexp.MustCompile(`^[^\s]+$`)
	slug   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:+/-]*$`)
	term   = regexp.MustCompile(`^[0-9]{4}(-[a-z0-9]+)*$`)
	rorRE  = regexp.MustCompile(`^0[0-9a-hjkmnp-tv-z]{6}[0-9]{2}$`)
	// carminSlug is the slug of a video or a collection of Carmin.tv, as the-pentagram-map-and-ramifications. A long slug is cut, and can end in a hyphen.
	carminSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

var systems = map[string]system{
	"doi": {
		strip: []string{"https://doi.org/", "http://doi.org/", "https://dx.doi.org/", "http://dx.doi.org/", "doi:"},
		fold:  strings.ToLower,
		re:    regexp.MustCompile(`^10\.[0-9]{4,9}/\S+$`),
	},
	"arxiv": {
		strip: []string{"https://arxiv.org/abs/", "http://arxiv.org/abs/", "arxiv:"},
		fold:  func(s string) string { id, _ := splitVersion(s); return id },
		re:    regexp.MustCompile(`^([0-9]{4}\.[0-9]{4,5}|[a-z-]+(\.[A-Z]{2})?/[0-9]{7})$`),
		check: arxivNew,
	},
	"arxiv_v": {fold: strings.ToLower, re: regexp.MustCompile(`^v[1-9][0-9]*$`)},
	"zbl":     {strip: []string{"zbl "}, fold: strings.TrimSpace, re: regexp.MustCompile(`^([0-9]{4}\.[0-9]{5}|[1-9][0-9]*)$`)},
	"zbl_id":  {fold: zeros, re: digits},
	"jfm":     {strip: []string{"jfm "}, fold: strings.TrimSpace, re: regexp.MustCompile(`^[0-9]{2}\.[0-9]{4}\.[0-9]{2}$`)},
	"mr":      {strip: []string{"mr"}, fold: zeros, re: digits},
	"isbn":    {strip: []string{"isbn:", "isbn"}, fold: isbn, re: regexp.MustCompile(`^97[89][0-9]{10}$`), check: isbn13},
	"issn":    {strip: []string{"issn:", "issn"}, fold: issn, re: regexp.MustCompile(`^[0-9]{4}-[0-9]{3}[0-9X]$`), check: issnCheck},
	"issn_l":  {strip: []string{"issn:", "issn"}, fold: issn, re: regexp.MustCompile(`^[0-9]{4}-[0-9]{3}[0-9X]$`), check: issnCheck},
	"oclc":    {fold: oclc, re: digits},
	"lccn":    {strip: []string{"https://lccn.loc.gov/", "http://lccn.loc.gov/"}, fold: lccn, re: regexp.MustCompile(`^([a-z]{0,3}[0-9]{8}|[a-z]{0,2}[0-9]{10})$`)},
	"openlibrary_work": {
		strip: []string{"https://openlibrary.org/works/", "/works/"},
		fold:  strings.ToUpper,
		re:    regexp.MustCompile(`^OL[1-9][0-9]*W$`),
	},
	"openlibrary_edition": {
		strip: []string{"https://openlibrary.org/books/", "/books/"},
		fold:  strings.ToUpper,
		re:    regexp.MustCompile(`^OL[1-9][0-9]*M$`),
	},
	"wikidata":     {strip: []string{"https://www.wikidata.org/wiki/", "http://www.wikidata.org/entity/", "wd:"}, fold: strings.ToUpper, re: regexp.MustCompile(`^Q[1-9][0-9]*$`)},
	"openalex":     {strip: []string{"https://openalex.org/"}, fold: strings.ToUpper, re: regexp.MustCompile(`^W[1-9][0-9]*$`)},
	"s2":           {strip: []string{"corpusid:"}, fold: zeros, re: digits},
	"hal":          {strip: []string{"https://hal.science/"}, fold: hal, re: regexp.MustCompile(`^[a-z]+-[0-9]{8}$`)},
	"numdam":       {re: native},
	"eudml":        {strip: []string{"urn:eudml:doc:"}, fold: zeros, re: digits},
	"euclid":       {re: native},
	"mathnetru":    {re: native},
	"cyberleninka": {re: native},
	"cnki":         {re: native},
	"jstage":       {re: native},
	"hathi":        {strip: []string{"https://hdl.handle.net/2027/"}, re: regexp.MustCompile(`^[a-z0-9]{2,6}\.[$A-Za-z0-9._:/-]+$`)},
	"hathi_record": {strip: []string{"https://catalog.hathitrust.org/record/"}, fold: zeros, re: digits},
	"ia":           {strip: []string{"https://archive.org/details/"}, re: regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)},
	"gutenberg":    {strip: []string{"https://www.gutenberg.org/ebooks/"}, fold: zeros, re: digits},
	"gbooks":       {re: regexp.MustCompile(`^[A-Za-z0-9_-]{12}$`)},
	"handle":       {strip: []string{"https://hdl.handle.net/", "hdl:"}, re: regexp.MustCompile(`^[0-9]+(\.[0-9A-Za-z]+)*/\S+$`)},
	"urn":          {fold: urn, re: regexp.MustCompile(`^urn:nbn:[a-z]{2}(:[a-z0-9]+)*[:-]\S+$`)},
	"course":       {re: regexp.MustCompile(`^[^:\s]+:\S+$`), check: course},
	"offering":     {re: regexp.MustCompile(`^[^:\s]+:\S+:\S+$`), check: offering},
	"ocw":          {re: slug},
	"coursera":     {re: slug},
	"edx":          {re: slug},
	"nptel":        {fold: zeros, re: digits},
	"icourse163":   {re: slug},
	"xuetangx":     {re: slug},
	"khan":         {re: slug},
	"youtube_playlist": {
		re: regexp.MustCompile(`^(PL|UU|LL|FL|OL)[A-Za-z0-9_-]{10,}$`),
	},
	"youtube_video": {re: regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)},
	"swmath":        {strip: []string{"https://zbmath.org/software/"}, fold: zeros, re: digits},
	"zenodo":        {strip: []string{"https://zenodo.org/records/", "https://zenodo.org/record/"}, fold: zeros, re: digits},
	"swh":           {re: regexp.MustCompile(`^swh:1:(cnt|dir|rev|rel|snp|ori):[0-9a-f]{40}(;\S+)?$`)},
	"ror":           {strip: []string{"https://ror.org/"}, fold: strings.ToLower, re: rorRE, check: ror},
	"orcid":         {strip: []string{"https://orcid.org/", "http://orcid.org/"}, fold: strings.ToUpper, re: regexp.MustCompile(`^[0-9]{4}-[0-9]{4}-[0-9]{4}-[0-9]{3}[0-9X]$`), check: orcid},
	"mgp":           {strip: []string{"https://www.mathgenealogy.org/id.php?id=", "https://mathgenealogy.org/id.php?id="}, fold: zeros, re: digits},
	"carmin": {
		strip: []string{"oai:carmin.tv:", "https://www.carmin.tv/fr/video/", "https://www.carmin.tv/en/video/"},
		re:    carminSlug,
	},
	"carmin_collection": {
		strip: []string{"videos:collection:"},
		re:    carminSlug,
	},
	"nnt":   {strip: []string{"https://theses.fr/", "https://www.theses.fr/", "http://www.theses.fr/"}, fold: strings.ToUpper, re: regexp.MustCompile(`^[0-9]{4}[A-Z0-9]{8}$`)},
	"idref": {strip: []string{"https://www.idref.fr/", "http://www.idref.fr/", "https://idref.fr/"}, fold: strings.ToUpper, re: regexp.MustCompile(`^[0-9]{8}[0-9X]$`), check: idref},
}

// Keys returns the keys of the systems of RS3.2, in sorted order.
func Keys() []string {
	out := make([]string, 0, len(systems))
	for k := range systems {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Known reports whether key is a system of RS3.2.
func Known(key string) bool {
	_, ok := systems[key]
	return ok
}

// Normalize returns the normal form of the id v of the system key. It removes the spaces at the ends and the URL prefix, and it changes the case and the form that RS3.3 gives. It returns an error when the result does not have the syntax of the system or fails its check digit.
func Normalize(key, v string) (string, error) {
	s, ok := systems[key]
	if !ok {
		return "", fmt.Errorf("%w %q", ErrUnknown, key)
	}
	v = strings.TrimSpace(v)
	for _, p := range s.strip {
		if len(v) >= len(p) && strings.EqualFold(v[:len(p)], p) {
			v = strings.TrimSpace(v[len(p):])
			break
		}
	}
	if s.fold != nil {
		v = s.fold(v)
	}
	if !s.re.MatchString(v) {
		return "", fmt.Errorf("ids: %q is not a %s id", v, key)
	}
	if s.check != nil {
		if err := s.check(v); err != nil {
			return "", fmt.Errorf("ids: %s %q: %w", key, v, err)
		}
	}
	return v, nil
}

// Check returns nil when v is the normal form of a valid id of the system key. A valid id that is not in its normal form is an error, so a record keeps one form of each id.
func Check(key, v string) error {
	n, err := Normalize(key, v)
	if err != nil {
		return err
	}
	if n != v {
		return fmt.Errorf("ids: the normal form of the %s id %q is %q", key, v, n)
	}
	return nil
}

// SplitArxiv returns the arXiv id and the version of v, for example 2203.17207 and v2 for arXiv:2203.17207v2. The version is empty when v has none.
func SplitArxiv(v string) (id, version string, err error) {
	raw := strings.TrimSpace(v)
	for _, p := range systems["arxiv"].strip {
		if len(raw) >= len(p) && strings.EqualFold(raw[:len(p)], p) {
			raw = raw[len(p):]
			break
		}
	}
	_, version = splitVersion(raw)
	id, err = Normalize("arxiv", v)
	return id, version, err
}

// ISBN10 returns the ISBN-10 form of an ISBN-13 in normal form, for the display. An ISBN with the prefix 979 has no ISBN-10 form, so ISBN10 returns an empty string.
func ISBN10(v string) string {
	if len(v) != 13 || !strings.HasPrefix(v, "978") {
		return ""
	}
	body := v[3:12]
	sum := 0
	for i := range 9 {
		sum += (10 - i) * int(body[i]-'0')
	}
	c := (11 - sum%11) % 11
	if c == 10 {
		return body + "X"
	}
	return body + strconv.Itoa(c)
}

func splitVersion(s string) (id, version string) {
	i := strings.LastIndexByte(s, 'v')
	if i <= 0 || i == len(s)-1 {
		return s, ""
	}
	for _, c := range s[i+1:] {
		if c < '0' || c > '9' {
			return s, ""
		}
	}
	return s[:i], s[i:]
}

// arxivNew checks the month of a new-scheme id, and the five digits after 1501.
func arxivNew(v string) error {
	yymm, num, ok := strings.Cut(v, ".")
	if !ok || strings.Contains(v, "/") {
		return nil
	}
	if m, _ := strconv.Atoi(yymm[2:]); m < 1 || m > 12 {
		return errors.New("the month is not 01 to 12")
	}
	if yymm >= "1501" && len(num) != 5 || yymm < "1501" && len(num) != 4 {
		return errors.New("the number has the wrong length for its month")
	}
	return nil
}

func zeros(s string) string {
	t := strings.TrimLeft(strings.TrimSpace(s), "0")
	if t == "" && s != "" {
		return "0"
	}
	return t
}

// oclc removes the prefix (OCoLC) and then the prefix ocm, ocn or on, as a MARC record gives them.
func oclc(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimPrefix(s, "(ocolc)")
	for _, p := range []string{"ocm", "ocn", "on"} {
		if strings.HasPrefix(s, p) {
			s = s[len(p):]
			break
		}
	}
	return zeros(s)
}

func isbn(s string) string {
	s = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(s))
	if len(s) != 10 || isbn10(s) != nil {
		return s
	}
	body := "978" + s[:9]
	return body + strconv.Itoa(isbnDigit13(body))
}

func isbn10(s string) error {
	sum := 0
	for i := range 10 {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			sum += (10 - i) * int(c-'0')
		case c == 'X' && i == 9:
			sum += 10
		default:
			return errors.New("an ISBN-10 has nine digits and a check digit")
		}
	}
	if sum%11 != 0 {
		return errors.New("the check digit is wrong")
	}
	return nil
}

func isbnDigit13(body string) int {
	sum := 0
	for i := range 12 {
		w := 1
		if i%2 == 1 {
			w = 3
		}
		sum += w * int(body[i]-'0')
	}
	return (10 - sum%10) % 10
}

func isbn13(s string) error {
	if isbnDigit13(s[:12]) != int(s[12]-'0') {
		return errors.New("the check digit is wrong")
	}
	return nil
}

func issn(s string) string {
	s = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(s))
	if len(s) == 8 {
		return s[:4] + "-" + s[4:]
	}
	return s
}

func issnCheck(s string) error {
	d := s[:4] + s[5:]
	sum := 0
	for i := range 7 {
		sum += (8 - i) * int(d[i]-'0')
	}
	want := (11 - sum%11) % 11
	got := int(d[7] - '0')
	if d[7] == 'X' {
		got = 10
	}
	if got != want {
		return errors.New("the check digit is wrong")
	}
	return nil
}

// lccn applies the normalization of the Library of Congress: no spaces, nothing after a slash, and a serial number after a hyphen padded to six digits.
func lccn(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, " ", ""))
	s, _, _ = strings.Cut(s, "/")
	if pre, serial, ok := strings.Cut(s, "-"); ok && len(serial) <= 6 {
		s = pre + strings.Repeat("0", 6-len(serial)) + serial
	}
	return s
}

func hal(s string) string {
	s = strings.ToLower(s)
	if i := strings.LastIndex(s, "v"); i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		if _, err := strconv.Atoi(s[i+1:]); err == nil {
			s = s[:i]
		}
	}
	return s
}

func urn(s string) string {
	if len(s) >= 8 && strings.EqualFold(s[:8], "urn:nbn:") {
		return strings.ToLower(s)
	}
	return s
}

// ror checks the two check digits of a ROR id: the first seven characters are a number in Crockford base32, and the digits are 98 minus the number times 100, mod 97.
func ror(s string) error {
	const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"
	n := 0
	for _, c := range s[:7] {
		n = n*32 + strings.IndexRune(alphabet, c)
	}
	want := 98 - (n*100)%97
	if got, _ := strconv.Atoi(s[7:]); got != want {
		return errors.New("the check digits are wrong")
	}
	return nil
}

// idref checks the check character of a PPN of IdRef: the first eight digits with the weights 9 to 2, mod 11. The value 10 is written X.
func idref(s string) error {
	sum := 0
	for i := range 8 {
		sum += (9 - i) * int(s[i]-'0')
	}
	want := (11 - sum%11) % 11
	got := int(s[8] - '0')
	if s[8] == 'X' {
		got = 10
	}
	if got != want {
		return errors.New("the check character is wrong")
	}
	return nil
}

// orcid checks the check character of ISO 7064 mod 11-2.
func orcid(s string) error {
	d := strings.ReplaceAll(s, "-", "")
	total := 0
	for i := range 15 {
		total = (total + int(d[i]-'0')) * 2
	}
	want := (12 - total%11) % 11
	got := int(d[15] - '0')
	if d[15] == 'X' {
		got = 10
	}
	if got != want {
		return errors.New("the check character is wrong")
	}
	return nil
}

func course(s string) error {
	i := strings.IndexByte(s, ':')
	if !rorRE.MatchString(s[:i]) || ror(s[:i]) != nil {
		return errors.New("the part before the colon is not a valid ROR id")
	}
	return nil
}

func offering(s string) error {
	i := strings.LastIndexByte(s, ':')
	if !term.MatchString(s[i+1:]) {
		return errors.New("the term is not a year with an optional part, for example 2010-spring")
	}
	if !strings.Contains(s[:i], ":") {
		return errors.New("an offering is a course id and a term")
	}
	return course(s[:i])
}
