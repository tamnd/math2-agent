package align_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/ingest"
)

func ExampleIndex_Match() {
	x, err := align.NewIndex("wikidata", []ingest.Record{
		{ID: "Q1", Title: "theorem of the two sets"},
		{ID: "Q2", Title: "group", Meta: map[string]string{"nlab": "group"}},
		{ID: "Q3", Title: "ring"},
	}, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	// The name finds the item.
	for _, c := range x.Match(align.Item{Names: []string{"The theorem of the two sets"}}, 1) {
		fmt.Println(c.Record.ID, c.Score)
	}
	// An id in a third system finds the item too, with the score 1.
	for _, c := range x.Match(align.Item{Names: []string{"Groups"}, SameAs: map[string]string{"nlab": "group"}}, 1) {
		fmt.Println(c.Record.ID, c.Score, c.Channels[align.ChannelID])
	}
	// Output:
	// Q1 1
	// Q2 1 1
}
