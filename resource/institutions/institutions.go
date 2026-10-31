// Package institutions builds the institution register of RS5.3, records/resources/institutions.md. The register starts from the data dump of ROR (CC0) and from Wikidata (CC0). It takes each active organization of ROR with the type education, and it adds the unit of mathematics that Wikidata names for the organization. ROR has no units, so an organization with no unit in Wikidata stays in the register with no math_unit, and its gap is visible. A build keeps the fields that a person set: the level, the unit, the catalogue and the tier.
package institutions

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
)

// File is the institution register.
const File = resource.Dir + "/institutions.md"

// The levels of an institution.
const (
	University      = "university"
	College         = "college"
	SchoolSystem    = "school-system"
	Platform        = "platform"
	OlympiadProgram = "olympiad-program"
)

// Levels are the levels of RS5.3.
var Levels = []string{University, College, SchoolSystem, Platform, OlympiadProgram}

// The tiers of an institution (RS5.3). A new institution is I4 until a person reads its catalogue and its terms.
const (
	I1 = "I1" // open course materials under an open licence, and the open course platforms
	I2 = "I2" // a public catalogue in a machine-readable form, or terms that let math2 read it
	I3 = "I3" // a public catalogue that math2 may read only page by page
	I4 = "I4" // no public catalogue, or terms that forbid reading it
)

// Tiers are the tiers of RS5.3.
var Tiers = []string{I1, I2, I3, I4}

// Unit is the unit of mathematics of an institution.
type Unit struct {
	Name     string `json:"name"`
	URL      string `json:"url,omitempty"`
	Wikidata string `json:"wikidata,omitempty"`
}

// Catalogue is where an institution publishes its course catalogue.
type Catalogue struct {
	URL    string `json:"url"`
	Format string `json:"format,omitempty"` // for example html, json, pdf, or the name of the platform
	Terms  string `json:"terms,omitempty"`  // the terms of use, as a URL or a short note
	Math   string `json:"math,omitempty"`   // the prefix of the codes of the courses of the mathematics unit, or All when the adapter keeps only those courses
}

// All is the value of Catalogue.Math when the adapter of the catalogue keeps only the courses of the mathematics unit.
const All = "*"

// Institution is one record of the register.
type Institution struct {
	ROR       string     `json:"ror,omitempty"` // the ROR id, without https://ror.org/
	Name      string     `json:"name"`
	Country   string     `json:"country,omitempty"` // the ISO 3166-1 code
	Level     string     `json:"level"`
	MathUnit  *Unit      `json:"math_unit,omitempty"`
	Catalogue *Catalogue `json:"catalogue,omitempty"`
	Tier      string     `json:"tier"`
	Wikidata  string     `json:"wikidata,omitempty"`
	Website   string     `json:"website,omitempty"`
	Status    string     `json:"status,omitempty"` // withdrawn when ROR no longer has the organization as active
}

// Key gives the key of an institution: the ROR id, or the name of a platform with no ROR id.
func (in Institution) Key() string { return cmp.Or(in.ROR, "name:"+in.Name) }

// Problems gives the problems of a record.
func (in Institution) Problems() []string {
	var out []string
	if in.Name == "" {
		out = append(out, "the institution has no name")
	}
	if !slices.Contains(Levels, in.Level) {
		out = append(out, fmt.Sprintf("the level %q is not one of %s", in.Level, strings.Join(Levels, ", ")))
	}
	if !slices.Contains(Tiers, in.Tier) {
		out = append(out, fmt.Sprintf("the tier %q is not one of %s", in.Tier, strings.Join(Tiers, ", ")))
	}
	if (in.Tier == I1 || in.Tier == I2 || in.Tier == I3) && (in.Catalogue == nil || in.Catalogue.URL == "") {
		out = append(out, "the tier "+in.Tier+" needs a catalogue with a URL")
	}
	if in.ROR == "" && in.Level != Platform && in.Level != OlympiadProgram {
		out = append(out, "only a platform or an olympiad programme may have no ROR id")
	}
	return out
}

// Read reads the register of the tree at root. A tree with no register gives no record and no error.
func Read(root string) ([]Institution, error) {
	recs, err := mdrec.Read(filepath.Join(root, File))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out, err := mdrec.Unmarshal[Institution](recs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	return out, nil
}

// Write writes the register to the tree at root, in the order of the keys.
func Write(root string, ins []Institution) error {
	slices.SortFunc(ins, func(a, b Institution) int { return cmp.Compare(a.Key(), b.Key()) })
	recs, err := mdrec.Marshal(ins)
	if err != nil {
		return err
	}
	path := filepath.Join(root, File)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return mdrec.Write(path, recs)
}

// Org is one organization of the ROR dump, with the fields that the register uses.
type Org struct {
	ID        string
	Name      string
	Country   string
	Types     []string
	Status    string
	Wikidata  string
	Website   string
	Relations []string // the ROR ids of the parent organizations
}

type rorRecord struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Types  []string
	Names  []struct {
		Value string   `json:"value"`
		Types []string `json:"types"`
	} `json:"names"`
	Locations []struct {
		Geonames struct {
			CountryCode string `json:"country_code"`
		} `json:"geonames_details"`
	} `json:"locations"`
	ExternalIDs []struct {
		Type      string   `json:"type"`
		All       []string `json:"all"`
		Preferred string   `json:"preferred"`
	} `json:"external_ids"`
	Links []struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"links"`
}

// ReadROR reads the organizations of a ROR data dump in the schema v2: a JSON array, or the zip file of Zenodo that holds it. It calls fn for each organization.
func ReadROR(data []byte, fn func(Org) error) error {
	if bytes.HasPrefix(data, []byte("PK")) {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return err
		}
		var pick *zip.File
		for _, f := range zr.File {
			if strings.HasSuffix(f.Name, ".json") && (pick == nil || strings.Contains(f.Name, "v2")) {
				pick = f
			}
		}
		if pick == nil {
			return errors.New("institutions: the zip file of ROR has no JSON file")
		}
		rc, err := pick.Open()
		if err != nil {
			return err
		}
		defer func() { _ = rc.Close() }()
		return decodeROR(rc, fn)
	}
	return decodeROR(bytes.NewReader(data), fn)
}

func decodeROR(r io.Reader, fn func(Org) error) error {
	dec := json.NewDecoder(r)
	if t, err := dec.Token(); err != nil || t != json.Delim('[') {
		return fmt.Errorf("institutions: the ROR dump is not a JSON array: %v", err)
	}
	for dec.More() {
		var rec rorRecord
		if err := dec.Decode(&rec); err != nil {
			return fmt.Errorf("institutions: the ROR dump: %w", err)
		}
		o := Org{ID: strings.TrimPrefix(rec.ID, "https://ror.org/"), Status: rec.Status, Types: rec.Types}
		for _, n := range rec.Names {
			if slices.Contains(n.Types, "ror_display") || o.Name == "" {
				o.Name = n.Value
			}
		}
		if len(rec.Locations) > 0 {
			o.Country = rec.Locations[0].Geonames.CountryCode
		}
		for _, x := range rec.ExternalIDs {
			if x.Type == "wikidata" {
				o.Wikidata = cmp.Or(x.Preferred, first(x.All))
			}
		}
		for _, l := range rec.Links {
			if l.Type == "website" && o.Website == "" {
				o.Website = l.Value
			}
		}
		if err := fn(o); err != nil {
			return err
		}
	}
	return nil
}

func first(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
}

// Endpoint is the SPARQL service of Wikidata.
const Endpoint = "https://query.wikidata.org/sparql"

// Query asks Wikidata for each unit whose field of work is mathematics (Q395) and whose parent organization has a ROR id.
const Query = `SELECT ?unit ?unitLabel ?site ?ror WHERE {
  ?unit wdt:P101 wd:Q395 ; (wdt:P749|wdt:P361) ?org .
  ?org wdt:P6782 ?ror .
  OPTIONAL { ?unit wdt:P856 ?site }
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en,mul" }
}`

// Getter gets the body of a URL.
type Getter func(ctx context.Context, url string) ([]byte, error)

// Units asks the service at endpoint for the units of mathematics. It gives the units by the ROR id of the parent organization, in the order of their QIDs.
func Units(ctx context.Context, get Getter, endpoint string) (map[string][]Unit, error) {
	q := url.Values{"query": {Query}, "format": {"json"}}
	body, err := get(ctx, endpoint+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var res struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("institutions: the answer of Wikidata: %w", err)
	}
	out := map[string][]Unit{}
	for _, b := range res.Results.Bindings {
		qid := strings.TrimPrefix(b["unit"].Value, "http://www.wikidata.org/entity/")
		ror := b["ror"].Value
		if ror == "" || slices.ContainsFunc(out[ror], func(u Unit) bool { return u.Wikidata == qid }) {
			continue
		}
		out[ror] = append(out[ror], Unit{Name: cmp.Or(b["unitLabel"].Value, qid), URL: b["site"].Value, Wikidata: qid})
	}
	for k := range out {
		slices.SortFunc(out[k], func(a, b Unit) int { return cmp.Compare(a.Wikidata, b.Wikidata) })
	}
	return out, nil
}

// Platforms are the course platforms of RS5.5. A platform has no ROR id, so that it does not take the record of its university. RS5.3 puts the open course platforms in I1. A platform whose terms forbid automated reading, or whose site blocks an automated client, is I4, and Terms gives the reason. Format is the source of the adapter that reads the platform. A build sets the tier and the catalogue of each platform from this list.
var Platforms = []Institution{
	{Name: "MIT OpenCourseWare", Country: "US", Level: Platform, Tier: I1, Catalogue: &Catalogue{URL: "https://ocw.mit.edu/search/?d=Mathematics", Format: "ocw", Terms: "CC BY-NC-SA 4.0", Math: "18."}},
	{Name: "Open Yale Courses", Country: "US", Level: Platform, Tier: I1, Catalogue: &Catalogue{URL: "https://oyc.yale.edu/courses", Format: "oyc", Terms: "CC BY-NC-SA 3.0"}},
	{Name: "FutureLearn", Country: "GB", Level: Platform, Tier: I4, Catalogue: &Catalogue{URL: "https://www.futurelearn.com/subjects/science-engineering-and-maths-courses", Format: "futurelearn", Terms: "the terms forbid access with robots, spiders or scripts"}},
	{Name: "NPTEL", Country: "IN", Level: Platform, Tier: I1, Catalogue: &Catalogue{URL: "https://nptel.ac.in/courses", Format: "nptel", Terms: "to read (RQ10)", Math: All}},
	{Name: "Khan Academy", Country: "US", Level: Platform, Tier: I4, Catalogue: &Catalogue{URL: "https://www.khanacademy.org/math", Format: "khan", Terms: "the site answers an automated client with a JavaScript challenge"}},
	{Name: "Coursera", Country: "US", Level: Platform, Tier: I4, Catalogue: &Catalogue{URL: "https://www.coursera.org/browse/math-and-logic", Format: "coursera", Terms: "the terms forbid data scraping by automated means without written consent"}},
	{Name: "edX", Country: "US", Level: Platform, Tier: I4, Catalogue: &Catalogue{URL: "https://www.edx.org/learn/math", Format: "edx", Terms: "the terms forbid access with spiders, robots, crawlers and data mining tools"}},
	{Name: "iCourse163", Country: "CN", Level: Platform, Tier: I4, Catalogue: &Catalogue{URL: "https://www.icourse163.org/", Format: "icourse163", Terms: "no sitemap and no public API; the pages get the course data with scripts"}},
	{Name: "XuetangX", Country: "CN", Level: Platform, Tier: I4, Catalogue: &Catalogue{URL: "https://www.xuetangx.com/", Format: "xuetangx", Terms: "no robots.txt, no sitemap and no public API; each page has an hCaptcha check"}},
	{Name: "Class Central", Country: "US", Level: Platform, Tier: I4, Catalogue: &Catalogue{URL: "https://www.classcentral.com/subject/maths", Format: "classcentral", Terms: "the site answers an automated client with a Cloudflare challenge"}},
}

// Counts are the outcomes of one build.
type Counts struct {
	Orgs, Added, Withdrawn, WithUnit int
	ByTier                           map[string]int
}

// Build merges the organizations of ROR, the units of Wikidata and Platforms into the register prev. An organization with the type education and the status active is in the register. Its name, country, QID and website come from ROR at each build. The level, the unit, the catalogue and the tier of a record that a person set stay. A record whose organization is no longer active in ROR stays with the status withdrawn.
func Build(prev []Institution, orgs []Org, units map[string][]Unit) ([]Institution, Counts) {
	c := Counts{ByTier: map[string]int{}}
	byKey := map[string]Institution{}
	for _, in := range prev {
		byKey[in.Key()] = in
	}
	seen := map[string]bool{}
	for _, o := range orgs {
		if o.Status != "active" || !slices.Contains(o.Types, "education") {
			continue
		}
		c.Orgs++
		seen[o.ID] = true
		in, had := byKey[o.ID]
		if !had {
			c.Added++
			in = Institution{ROR: o.ID, Level: University, Tier: I4}
		}
		in.Name, in.Country, in.Wikidata, in.Website, in.Status = o.Name, o.Country, o.Wikidata, o.Website, ""
		if us := units[o.ID]; in.MathUnit == nil && len(us) > 0 {
			u := us[0]
			in.MathUnit = &u
		}
		byKey[o.ID] = in
	}
	for _, p := range Platforms {
		in, ok := byKey[p.Key()]
		if !ok {
			c.Added++
			in = p
		}
		in.Tier, in.Catalogue = p.Tier, p.Catalogue
		byKey[p.Key()] = in
	}
	out := make([]Institution, 0, len(byKey))
	for _, in := range byKey {
		if in.ROR != "" && !seen[in.ROR] && in.Level != Platform && len(orgs) > 0 {
			if in.Status != "withdrawn" {
				c.Withdrawn++
			}
			in.Status = "withdrawn"
		}
		if in.MathUnit != nil {
			c.WithUnit++
		}
		c.ByTier[in.Tier]++
		out = append(out, in)
	}
	slices.SortFunc(out, func(a, b Institution) int { return cmp.Compare(a.Key(), b.Key()) })
	return out, c
}

// MathCourses gives, for each source of an I1, I2 or I3 catalogue that names its mathematics unit, the prefix of the codes of the courses of that unit (RS5.3). Test S6 reads it.
func MathCourses(ins []Institution) map[string]string {
	out := map[string]string{}
	for _, in := range ins {
		if in.Status != "" || in.Catalogue == nil || in.Catalogue.Format == "" || in.Catalogue.Math == "" {
			continue
		}
		if in.Tier == I1 || in.Tier == I2 || in.Tier == I3 {
			out[in.Catalogue.Format] = in.Catalogue.Math
		}
	}
	return out
}

// MathCourse tells if a course with the given course id, in the form <ror>:<code>, from the given source is a course of the mathematics unit, by the prefixes of MathCourses.
func MathCourse(math map[string]string, source, course string) bool {
	p, ok := math[source]
	if !ok {
		return false
	}
	if p == All {
		return true
	}
	_, code, found := strings.Cut(course, ":")
	return found && strings.HasPrefix(code, p)
}
