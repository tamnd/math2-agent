// Package enumerate has the goal, the brief and the tool of the enumerate role (corpus K7.3, K7.4).
//
// An enumerate job asks a model which items of one cell it knows. A cell is an MSC code at five digits and a level, and the target of the job names it, for example 54D30/undergraduate. The brief gives the names that the inventory has for the code already, and the model lists the other items with the tool list_items. Each item becomes a mention of the source model-FAMILY in records/mentions/model-FAMILY/, where FAMILY is the family of the model in the model register. A mention changes no node, and no row comes from model mentions alone (rule I03).
//
// The sources that the model names go into the cites of a mention, and not into its identifiers in other systems, because a model can give a wrong id. A cite counts only when K9.5 confirms it.
//
// [Goal] holds when the changes are inside the field permission set of enumerate and the run added at least one mention.
package enumerate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/site"
	"github.com/tamnd/math2-agent/workspace"
)

// Levels are the levels of a cell (corpus K7.4), in order.
var Levels = []string{"school", "undergraduate", "graduate", "research"}

// mscCode is an MSC2020 code at five digits, such as 54D30.
var mscCode = regexp.MustCompile(`^[0-9]{2}[A-Z][0-9]{2}$`)

// Cell is the unit of work of an enumerate job: an MSC code at five digits and a level, with the names that the inventory has for the code.
type Cell struct {
	Code  string
	Level string
	Names []string // the names of the rows and of the model mentions with the code, sorted, each one once
}

// NewCell returns the cell of the target, in the form CODE/LEVEL. Its names come from the rows and the mentions of models with the code, because a new round gives the model the list of the rounds before it.
func NewCell(target string, rows []inventory.Row, ms []inventory.Mention) (*Cell, error) {
	code, level, ok := strings.Cut(target, "/")
	if !ok || !mscCode.MatchString(code) || !slices.Contains(Levels, level) {
		return nil, fmt.Errorf("enumerate: the target %q is not a cell of the form CODE/LEVEL, such as 54D30/undergraduate, where the level is one of %s", target, strings.Join(Levels, ", "))
	}
	c := &Cell{Code: code, Level: level}
	for _, r := range rows {
		if r.Name != "" && slices.Contains(r.MSC, code) {
			c.Names = append(c.Names, r.Name)
		}
	}
	for i := range ms {
		if m := &ms[i]; m.Name != "" && inventory.FromModel(m) && slices.Contains(m.MSC, code) {
			c.Names = append(c.Names, m.Name)
		}
	}
	slices.Sort(c.Names)
	c.Names = slices.Compact(c.Names)
	return c, nil
}

// Key returns the form of a name that two names of one item share: the letters and the digits in lower case, with no accents. "Heine–Borel theorem" and "Heine-Borel Theorem" have the same key.
func Key(name string) string {
	return strings.ReplaceAll(site.Slug(name), "-", "")
}

// Source returns the source of the mentions of the model family, such as model-claude.
func Source(family string) string { return "model-" + family }

// Brief returns the first message of an enumerate run on the job for the cell.
func Brief(job *agent.Job, c *Cell) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: list the items of mathematics that you know for the MSC2020 code %s at the %s level. The goal of the job: %s.\n\n", job.ID, c.Code, c.Level, strings.TrimSuffix(job.Goal, "."))
	fmt.Fprintf(&b, "Give each item that is not on the list below with one call of list_items. An item is a concept, a definition, a statement, a conjecture, a problem, an example, a counterexample or an object. For each item give its kind, its common name and its aliases, a statement of one sentence in your own words, the names of its prerequisites, and the sources where it appears.\n\n")
	fmt.Fprintf(&b, "A source is a system and an id: isbn with the theorem or the page in the locator, doi, arxiv, wikipedia with the page title, or another system of the crosswalk (%s). Give a source only when you can name it exactly. When you are unsure of a source, leave it out. When you are unsure that an item exists, leave it out.\n\n", strings.Join(crosswalk.Systems(), ", "))
	if len(c.Names) == 0 {
		fmt.Fprintf(&b, "The inventory has no item for %s yet.\n", c.Code)
		return b.String()
	}
	fmt.Fprintf(&b, "The inventory has these %d items for %s already. Do not list them again, also not with another name:\n\n%s\n", len(c.Names), c.Code, tools.Data("inventory/"+c.Code, "CC0-1.0", strings.Join(c.Names, "\n")))
	return b.String()
}

// item is one item of a call of list_items.
type item struct {
	Kind      string
	Name      string
	Aliases   []string
	Statement string
	Uses      []string
	Sources   []inventory.Cite
}

// listSchema is the schema of the input of list_items.
var listSchema = json.RawMessage(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{` +
	`"kind":{"type":"string","enum":["concept","definition","statement","conjecture","problem","example","counterexample","object"]},` +
	`"name":{"type":"string","description":"the common name of the item"},` +
	`"aliases":{"type":"array","items":{"type":"string"},"description":"the other names of the item"},` +
	`"statement":{"type":"string","description":"the item in one sentence, in your own words"},` +
	`"uses":{"type":"array","items":{"type":"string"},"description":"the names of the prerequisites of the item"},` +
	`"sources":{"type":"array","items":{"type":"object","properties":{` +
	`"system":{"type":"string","description":"isbn, doi, or a system of the crosswalk such as arxiv, wikipedia or wikidata"},` +
	`"id":{"type":"string","description":"the id in the system, such as 978-0-07-054235-8 or Compact space"},` +
	`"locator":{"type":"string","description":"where the item is in the source, such as Theorem 2.41"}},"required":["system","id"],"additionalProperties":false}}},` +
	`"required":["kind","name","statement"],"additionalProperties":false}}},"required":["items"],"additionalProperties":false}`)

// kinds are the kinds of the items that a model lists.
var kinds = []string{"concept", "definition", "statement", "conjecture", "problem", "example", "counterexample", "object"}

// List returns the tool list_items, which adds the items that the model lists to the mentions of the source model-FAMILY in the work tree. The mentions are by the agent id, for example agent:m2-agent/enumerate@0.1.0, and their snapshot is the model id. The tool refuses an item whose name has the key of a name of the cell or of an item before it. It leaves out each source that is not valid, and says why.
func List(ws *workspace.Workspace, c *Cell, family, model, by string) agent.Tool {
	write := tools.Write(ws)
	source := Source(family)
	seen := map[string]bool{}
	for _, n := range c.Names {
		seen[Key(n)] = true
	}
	return agent.Func{
		Def: llm.Tool{
			Name:        "list_items",
			Description: "Add the items of the cell that you know to the inventory, as the mentions of a model list. Give all the items in one call.",
			Schema:      listSchema,
		},
		Fn: func(ctx context.Context, job *agent.Job, input json.RawMessage) (agent.Result, error) {
			var in struct{ Items []item }
			if err := json.Unmarshal(input, &in); err != nil {
				return agent.Result{}, err
			}
			var notes []string
			files := map[string][]inventory.Mention{} // the mentions of each shard file that the call changes
			ids := map[string]bool{}
			added := 0
			for i, it := range in.Items {
				m, why := mention(it, c, source, model, by)
				if m == nil {
					notes = append(notes, fmt.Sprintf("item %d: %s", i+1, why))
					continue
				}
				if k := Key(m.Name); seen[k] {
					notes = append(notes, fmt.Sprintf("item %d: the inventory has %q already", i+1, m.Name))
					continue
				}
				if why != "" {
					notes = append(notes, fmt.Sprintf("item %d: %s", i+1, why))
				}
				// An id that a file has already gets the next fragment, which can be in another shard.
				native := c.Code + "." + site.Slug(m.Name)
				for n := 1; ; n++ {
					m.ID = inventory.MentionID(source, native, n)
					sh := inventory.MentionShard(m.ID)
					if _, ok := files[sh]; !ok {
						have, err := read(ws, path(source, sh))
						if err != nil {
							return agent.Result{}, err
						}
						files[sh] = have
						for _, h := range have {
							ids[h.ID] = true
						}
					}
					if !ids[m.ID] {
						ids[m.ID] = true
						files[sh] = append(files[sh], *m)
						break
					}
				}
				seen[Key(m.Name)] = true
				added++
			}
			if added == 0 {
				return agent.Result{}, fmt.Errorf("added no item of the %d. %s", len(in.Items), strings.Join(notes, ". "))
			}
			for _, sh := range slices.Sorted(maps.Keys(files)) {
				text, err := inventory.FormatMentions(files[sh])
				if err != nil {
					return agent.Result{}, err
				}
				args, err := json.Marshal(map[string]string{"path": path(source, sh), "text": string(text)})
				if err != nil {
					return agent.Result{}, err
				}
				if _, err := write.Call(ctx, job, args); err != nil {
					return agent.Result{}, err
				}
			}
			text := fmt.Sprintf("Added %d of %d items to %s.", added, len(in.Items), source)
			if len(notes) > 0 {
				text += " " + strings.Join(notes, ". ") + "."
			}
			return agent.Result{Text: text}, nil
		},
	}
}

// path returns the path of the shard file of the mentions of the source.
func path(source, shard string) string {
	return inventory.MentionsDir + "/" + source + "/" + shard + ".jsonl"
}

// read returns the mentions of the file in the work tree, or none when the file does not exist.
func read(ws *workspace.Workspace, p string) ([]inventory.Mention, error) {
	data, err := ws.Read(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ms, err := inventory.ParseMentions(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return ms, nil
}

// mention returns the mention of the item, or nil and the reason when the item is not valid. A mention with a reason left out a source.
func mention(it item, c *Cell, source, model, by string) (*inventory.Mention, string) {
	name := clean(it.Name)
	statement := clean(it.Statement)
	switch {
	case name == "" || Key(name) == "":
		return nil, "the item has no name"
	case !slices.Contains(kinds, it.Kind):
		return nil, fmt.Sprintf("the kind %q is not one of %s", it.Kind, strings.Join(kinds, ", "))
	case statement == "":
		return nil, fmt.Sprintf("the item %q has no statement", name)
	}
	m := &inventory.Mention{
		Source:   source,
		Snapshot: model,
		Locator:  "MSC " + c.Code + ", " + c.Level,
		Kind:     it.Kind,
		Name:     name,
		Aliases:  names(it.Aliases, name),
		Uses:     names(it.Uses, ""),
		Text:     statement,
		MSC:      []string{c.Code},
		Class:    "open",
		By:       by,
	}
	var bad []string
	for _, s := range it.Sources {
		s = inventory.Cite{System: strings.ToLower(clean(s.System)), ID: strings.TrimSpace(s.ID), Locator: clean(s.Locator)}
		if err := checkCite(s); err != nil {
			bad = append(bad, err.Error())
			continue
		}
		if !slices.Contains(m.Cites, s) {
			m.Cites = append(m.Cites, s)
		}
	}
	if len(bad) > 0 {
		return m, fmt.Sprintf("%q is added with no source %s", name, strings.Join(bad, ", and no source "))
	}
	return m, ""
}

// checkCite reports whether the cite has an id of the form of its system.
func checkCite(s inventory.Cite) error {
	switch s.System {
	case "doi":
		if !doi.MatchString(s.ID) {
			return fmt.Errorf("%s: %q is not a DOI such as 10.1007/978-3-642-61795-0", s.System, s.ID)
		}
		return nil
	case "isbn":
		if !ISBN(s.ID) {
			return fmt.Errorf("%s: %q is not an ISBN with a correct check digit", s.System, s.ID)
		}
		return nil
	}
	if err := crosswalk.Check(s.System, s.ID); err != nil {
		return fmt.Errorf("%s: %w", s.System, err)
	}
	return nil
}

// doi is the form of a DOI.
var doi = regexp.MustCompile(`^10\.[0-9]{4,9}/\S+$`)

// ISBN reports whether s is an ISBN-10 or an ISBN-13 with a correct check digit. Hyphens and spaces between the digits are allowed.
func ISBN(s string) bool {
	d := strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, s)
	sum := 0
	switch len(d) {
	case 10:
		for i, r := range d {
			v := int(r - '0')
			if r == 'X' && i == 9 {
				v = 10
			} else if r < '0' || r > '9' {
				return false
			}
			sum += (10 - i) * v
		}
		return sum%11 == 0
	case 13:
		for i, r := range d {
			if r < '0' || r > '9' {
				return false
			}
			sum += int(r-'0') * (1 + 2*(i%2))
		}
		return sum%10 == 0
	}
	return false
}

// names returns the names with space cleaned, each one once, with no empty name and no name with the key of not.
func names(list []string, not string) []string {
	var out []string
	for _, n := range list {
		n = clean(n)
		if n != "" && Key(n) != "" && (not == "" || Key(n) != Key(not)) && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// clean replaces each run of space with one space, and removes the space at the ends.
func clean(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

// Goal returns the goal of an enumerate run on the job: the changes are inside the field permission set of enumerate, and they add at least one mention.
func Goal(ws *workspace.Workspace, job *agent.Job) agent.Goal {
	return agent.OnChanges(ws, func(_ context.Context, changes []workspace.Change) (bool, string, error) {
		if len(changes) == 0 {
			return false, "the run has not added a mention yet. List the items with list_items", nil
		}
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		return true, "", nil
	})
}
