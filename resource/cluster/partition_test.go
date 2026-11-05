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

// The records of one block of C4 with low scores stay in separate components, and a pair with a high score is one component, in memory and with spill files.
func TestPartitionBlockPairs(t *testing.T) {
	in := Input{Records: []Record{
		{Source: "crossref", NativeID: "a", Kind: "article", Title: "Spectral gaps for random regular graphs", Creators: author("Ann Smith"), Date: "2000"},
		{Source: "openalex", NativeID: "W1", Kind: "article", Title: "Spectral gaps for random regular graphs", Creators: author("Ann Smith"), Date: "2001"},
		{Source: "crossref", NativeID: "b", Kind: "article", Title: "The cohomology of finite groups of Lie type", Creators: author("Ann Smith"), Date: "2000"},
		{Source: "crossref", NativeID: "c", Kind: "article", Title: "Boundary layers in viscous fluids", Creators: author("Ann Smith"), Date: "2001"},
	}}
	p := Partitioned(in, 1)
	if p.Components != 3 || p.Largest != 2 {
		t.Fatalf("components = %d, largest = %d, want 3 and 2", p.Components, p.Largest)
	}
	if p.Bucket[0] != p.Bucket[1] || p.Bucket[0] == p.Bucket[2] || p.Bucket[2] == p.Bucket[3] {
		t.Fatalf("buckets = %v", p.Bucket)
	}
	pt := partitioner{blocks: newBlockSet(t.TempDir())}
	var recs []int32
	for i := range in.Records {
		recs = append(recs, pt.record(&in.Records[i]))
	}
	_, q, err := pt.buckets(recs, 1)
	if err != nil {
		t.Fatal(err)
	}
	if q.Components != 3 || q.Largest != 2 {
		t.Fatalf("with spill files: components = %d, largest = %d, want 3 and 2", q.Components, q.Largest)
	}
}
