package textbook

import (
	"io"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// Records emits one source record for the book (RS13.1): a textbook with its title, its authors, its edition and its home page and repository as links. The record comes from the entry of the book in Books, so Records does not read the snapshot.
func (b Book) Records(_ io.Reader, emit func(cluster.Record) error) error {
	rec := cluster.Record{
		NativeID: b.ID,
		Level:    "work",
		Kind:     "textbook",
		Title:    b.Name,
		Edition:  b.Edition,
	}
	for _, a := range b.Authors {
		rec.Creators = append(rec.Creators, resource.Creator{Name: a, Role: "author"})
	}
	for _, l := range []string{b.Home, b.Repository} {
		if l != "" {
			rec.Links = append(rec.Links, l)
		}
	}
	return emit(rec)
}
