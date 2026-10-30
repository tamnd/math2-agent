package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/refs"
	resscope "github.com/tamnd/math2-agent/resource/scope"
)

// refsBases are the APIs that confirm an id of a reference. A test sets them to a local server.
var refsBases = map[string]string{
	"doi":   "https://api.crossref.org/works/",
	"arxiv": "https://export.arxiv.org/api/query?id_list=",
	"isbn":  "https://openlibrary.org/isbn/",
}

// refsLookup confirms a DOI with Crossref, an arXiv id with the API of arXiv, and an ISBN with Open Library. No public API confirms a Zbl or an MR number, so the lookup gives false for them.
func refsLookup(c *fetch.Client) refs.Lookup {
	return func(ctx context.Context, key, value string) (bool, error) {
		base, ok := refsBases[key]
		if !ok {
			return false, nil
		}
		u := base + url.PathEscape(value)
		if key == "isbn" {
			u += ".json"
		}
		body, err := c.Get(ctx, u)
		if errors.Is(err, fetch.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if key == "arxiv" {
			return strings.Contains(string(body), "<entry>") && strings.Contains(string(body), "/abs/"+value), nil
		}
		return true, nil
	}
}

// runResImportRefs builds the source records of the source refs from the cites edges of the tree (RS11.5). It adds the entry of the source to manifests/sources.yaml when the catalogue does not have it, and a snapshot for each new set of records.
func runResImportRefs(root string, lookup bool, gap time.Duration, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res import: refs: %v\n", err)
		return ExitFailure
	}
	rs, err := resource.ReadResources(root)
	if err != nil {
		return fail(err)
	}
	cs, err := resource.ReadCites(root)
	if err != nil {
		return fail(err)
	}
	ds, err := resscope.ReadDecisions(root)
	if err != nil {
		return fail(err)
	}
	in := refs.Input{Resources: rs, Cites: cs, Confirmed: map[string]bool{}}
	for _, d := range ds {
		if d.Kind == refs.ConfirmKind {
			in.Confirmed[d.ID] = true
		}
	}
	if lookup {
		in.Lookup = refsLookup(fetch.NewClient(gap))
	}
	recs, n, err := refs.Build(context.Background(), in)
	if err != nil {
		return fail(err)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		return fail(err)
	}
	src := cat.Source(refs.Source)
	if src == nil {
		cat.Sources = append(cat.Sources, refs.Entry())
		src = &cat.Sources[len(cat.Sources)-1]
	}
	sn, err := refs.Snapshot(recs, time.Now().UTC().Format(time.DateOnly))
	if err != nil {
		return fail(err)
	}
	if have := src.Snapshot(sn.Hash); have != nil {
		sn = *have
	} else {
		src.Snapshots = append(src.Snapshots, sn)
	}
	var rejects []rescluster.Reject
	kept := recs[:0]
	for _, r := range recs {
		r.Snapshot = sn.Hash
		if why := rescluster.Check(&r); why != "" {
			rejects = append(rejects, rescluster.Reject{NativeID: r.NativeID, Snapshot: r.Snapshot, Reason: why})
			continue
		}
		kept = append(kept, r)
	}
	if err := rescluster.WriteRecords(root, refs.Source, rescluster.SourceFile, kept); err != nil {
		return fail(err)
	}
	if err := rescluster.WriteRejects(root, refs.Source, rejects); err != nil {
		return fail(err)
	}
	if err := cat.Write(root); err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "refs: %d unresolved references, %d records (%d cited by two or more, %d by id, %d by a person), %d ids looked up, %d rejected, snapshot %s\n", n.Unresolved, len(kept), n.Cited, n.ByID, n.Person, n.Lookups, len(rejects), sn.Hash)
	return ExitOK
}
