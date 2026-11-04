package cluster

import (
	"strconv"
	"testing"
)

// A block of C4 with more than MaxBlock records does not join its records into one component, and a partitioned run skips it as one run does.
func TestPartitionLargeBlock(t *testing.T) {
	var in Input
	for i := range MaxBlock + 1 {
		in.Records = append(in.Records, Record{Source: "crossref", NativeID: "c" + strconv.Itoa(i), Kind: "article", Title: "Paper number " + strconv.Itoa(i) + " on graphs", Creators: author("Ann Smith"), Date: "2000"})
	}
	in.Records = append(in.Records,
		Record{Source: "crossref", NativeID: "x1", Kind: "article", Title: "Spectral gaps for random regular graphs", Creators: author("Paul Erdos"), Date: "1990"},
		Record{Source: "openalex", NativeID: "W1", Kind: "article", Title: "Spectral gaps for random regular graphs", Creators: author("Paul Erdos"), Date: "1991"},
	)
	res := run(t, in)
	if res.Skipped != 1 {
		t.Fatalf("skipped = %d, want 1", res.Skipped)
	}
	p := Partitioned(Input{Records: unique(in.Records)}, 50)
	if p.Components != MaxBlock+2 || p.Largest != 2 || len(p.Large) != 1 {
		t.Fatalf("components = %d, largest = %d, large = %d", p.Components, p.Largest, len(p.Large))
	}
	if p.Buckets != 5 {
		t.Fatalf("buckets = %d, want 5", p.Buckets)
	}
}
