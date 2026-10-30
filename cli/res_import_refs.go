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
	"github.com/tamnd/math2-agent/resource/s2"
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
	kept, rejected, hash, err := saveBuilt(root, refs.Entry(), refs.Snapshot, recs)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "refs: %d unresolved references, %d records (%d cited by two or more, %d by id, %d by a person), %d ids looked up, %d rejected, snapshot %s\n", n.Unresolved, kept, n.Cited, n.ByID, n.Person, n.Lookups, rejected, hash)
	return ExitOK
}

// saveBuilt writes the records of a source that m2 builds, as refs and s2. It adds the entry of the source to manifests/sources.yaml when the catalogue does not have it, and a snapshot for each new set of records. A record that fails the checks of the clustering goes to the rejected records. It gives the counts of the kept and the rejected records, and the hash of the snapshot.
func saveBuilt(root string, entry ingest.Source, snapshot func([]rescluster.Record, string) (ingest.Snapshot, error), recs []rescluster.Record) (kept, rejected int, hash string, err error) {
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		return 0, 0, "", err
	}
	src := cat.Source(entry.ID)
	if src == nil {
		cat.Sources = append(cat.Sources, entry)
		src = &cat.Sources[len(cat.Sources)-1]
	}
	sn, err := snapshot(recs, time.Now().UTC().Format(time.DateOnly))
	if err != nil {
		return 0, 0, "", err
	}
	if have := src.Snapshot(sn.Hash); have != nil {
		sn = *have
	} else {
		src.Snapshots = append(src.Snapshots, sn)
	}
	var rejects []rescluster.Reject
	good := recs[:0]
	for _, r := range recs {
		r.Snapshot = sn.Hash
		if why := rescluster.Check(&r); why != "" {
			rejects = append(rejects, rescluster.Reject{NativeID: r.NativeID, Snapshot: r.Snapshot, Reason: why})
			continue
		}
		good = append(good, r)
	}
	if err := rescluster.WriteRecords(root, entry.ID, rescluster.SourceFile, good); err != nil {
		return 0, 0, "", err
	}
	if err := rescluster.WriteRejects(root, entry.ID, rejects); err != nil {
		return 0, 0, "", err
	}
	if err := cat.Write(root); err != nil {
		return 0, 0, "", err
	}
	return len(good), len(rejects), sn.Hash, nil
}

// runResImportS2 builds the source records of Semantic Scholar: the corpus ids of the in-scope resources with a DOI or an arXiv id (RS3.2).
func runResImportS2(root string, gap time.Duration, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res import: s2: %v\n", err)
		return ExitFailure
	}
	rs, err := resource.ReadResources(root)
	if err != nil {
		return fail(err)
	}
	c := fetch.NewClient(gap)
	recs, n, err := s2.Build(context.Background(), rs, s2.Batch(s2URL, c.Post))
	if err != nil {
		return fail(err)
	}
	kept, rejected, hash, err := saveBuilt(root, s2.Entry(), s2.Snapshot, recs)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "s2: asked %d, found %d, records %d, rejected %d, requests %d, snapshot %s\n", n.Asked, n.Found, kept, rejected, n.Requests, hash)
	return ExitOK
}

// s2URL is the batch endpoint of Semantic Scholar. A test sets it to a local server.
var s2URL = s2.BatchURL
