package resource

import (
	"cmp"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/tag"
)

// Minted is one provisional id that Mint changed to a resource id.
type Minted struct {
	Provisional string
	ID          string
	Kind        string
}

// Used returns the set of the ids of the one id space of K3.6 in the tree at root: the tags of registers/tags.reg and registers/retired.reg, the row ids of registers/inventory/ and the resource ids of registers/resources/. A new resource id must not be in the set (rule H01).
func Used(root string) (map[string]bool, error) {
	regs := filepath.Join(root, "registers")
	used, err := tag.ReadUsed(filepath.Join(regs, "tags.reg"), filepath.Join(regs, "retired.reg"))
	if err != nil {
		return nil, err
	}
	rows, err := inventory.ReadRegister(root)
	if err != nil {
		return nil, err
	}
	res, err := Registered(root)
	if err != nil {
		return nil, err
	}
	for id := range rows {
		used[id] = true
	}
	for id := range res {
		used[id] = true
	}
	return used, nil
}

// Mint changes each provisional id of records/resources/ in the tree at root to a new resource id (RS3.1). An import gives a new resource a provisional id, ~ and eight hex digits, as for a node. Mint draws each id with tag.Mint against the ids of Used, so a resource id is never equal to a tag or an inventory id. It also changes a provisional id in the field to of a relation and in the target of a scope decision. Then it writes the resources and adds the new ids to registers/resources/ with the given commit. If r is nil, Mint uses crypto/rand.
func Mint(root string, r io.Reader, commit string) ([]Minted, error) {
	used, err := Used(root)
	if err != nil {
		return nil, err
	}
	rs, err := ReadResources(root)
	if err != nil {
		return nil, err
	}
	byID := map[string]string{}
	var minted []Minted
	for i := range rs {
		p := rs[i].ID
		if !tag.IsProvisional(p) {
			continue
		}
		if _, ok := byID[p]; ok {
			return nil, fmt.Errorf("resource: two resources have the provisional id %s", p)
		}
		id, err := tag.Mint(r, func(s string) bool { return used[s] })
		if err != nil {
			return nil, err
		}
		used[id] = true
		byID[p] = id
		minted = append(minted, Minted{p, id, rs[i].Kind})
	}
	if len(minted) == 0 {
		return nil, nil
	}
	for i := range rs {
		res := &rs[i]
		if id, ok := byID[res.ID]; ok {
			res.ID = id
		}
		for j := range res.Relations {
			res.Relations[j].To = swap(res.Relations[j].To, byID)
		}
		if res.Scope != nil {
			res.Scope.Target = swap(res.Scope.Target, byID)
		}
		for _, ref := range append(refs(res), res.ID) {
			if tag.IsProvisional(ref) {
				return nil, fmt.Errorf("resource: %s names the provisional id %s, which no resource has", res.ID, ref)
			}
		}
	}
	es := make([]Entry, len(minted))
	for i, m := range minted {
		es[i] = Entry{m.ID, commit, m.Kind}
	}
	// The register comes first: when the write of the records fails, an id is lost, but no id is used twice.
	if err := AppendRegister(root, es); err != nil {
		return nil, err
	}
	if err := WriteResources(root, rs); err != nil {
		return nil, err
	}
	slices.SortFunc(minted, func(a, b Minted) int { return cmp.Compare(a.Provisional, b.Provisional) })
	return minted, nil
}

// swap changes the resource id at the start of ref, as <id> or <id>/e<n>, when it is a provisional id of byID.
func swap(ref string, byID map[string]string) string {
	id, rest := ref, ""
	if i := strings.IndexByte(ref, '/'); i >= 0 {
		id, rest = ref[:i], ref[i:]
	}
	if n, ok := byID[id]; ok {
		return n + rest
	}
	return ref
}

// refs returns the resource ids that the relations and the scope of r name.
func refs(r *Resource) []string {
	var out []string
	for _, rel := range r.Relations {
		id := rel.To
		if i := strings.IndexByte(id, '/'); i >= 0 {
			id = id[:i]
		}
		out = append(out, id)
	}
	if r.Scope != nil && r.Scope.Target != "" {
		out = append(out, r.Scope.Target)
	}
	return out
}
