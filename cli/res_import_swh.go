package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/sourceworks"
	"github.com/tamnd/math2-agent/resource/swh"
)

// swhAPI is the API of Software Heritage. A test sets it to a local server.
var swhAPI = swh.API

// runResImportSWH builds the source records of Software Heritage: the SWHIDs of the software whose source record links to a repository on a forge (RS7.5). It reuses the SWHIDs of the last run and asks for at most limit new origins. When a request fails, it keeps the records that it has and fails.
func runResImportSWH(root string, gap time.Duration, limit int, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res import: swh: %v\n", err)
		return ExitFailure
	}
	all, err := rescluster.ReadRecords(root)
	if err != nil {
		return fail(err)
	}
	var prev []rescluster.Record
	for _, r := range all {
		if r.Source == swh.Source {
			prev = append(prev, r)
		}
	}
	c := swh.NewClient(userAgent(), gap)
	c.Base = swhAPI
	recs, n, berr := swh.Build(context.Background(), all, prev, c.Latest, limit)
	kept, rejected, hash, err := saveBuilt(root, swh.Entry(), swh.Snapshot, recs)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "swh: origins %d, reused %d, asked %d, found %d, left %d, records %d, rejected %d, snapshot %s\n", n.Origins, n.Reused, n.Asked, n.Found, n.Left, kept, rejected, hash)
	if berr != nil {
		return fail(berr)
	}
	return ExitOK
}

// runResImportSourceWorks builds the source records of the sources that are also works: the reference works, the wikis, the databases and the formal libraries (RS7.5 and RS7.6). It makes no request.
func runResImportSourceWorks(root string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res import: %s: %v\n", sourceworks.Source, err)
		return ExitFailure
	}
	sites, err := sourceworks.Sites()
	if err != nil {
		return fail(err)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		return fail(err)
	}
	recs := sourceworks.Build(sites, cat, sourceworks.Adapters)
	kept, rejected, hash, err := saveBuilt(root, sourceworks.Entry(), sourceworks.Snapshot, recs)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%s: %d sites, records %d, rejected %d, snapshot %s\n", sourceworks.Source, len(sites), kept, rejected, hash)
	return ExitOK
}
