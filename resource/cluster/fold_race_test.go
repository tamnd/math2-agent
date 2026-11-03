package cluster

import (
	"sync"
	"testing"
)

func TestFoldConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 2000 {
				if got := Fold("Théorème de Gödel, été"); got != "theoreme de godel ete" {
					t.Errorf("Fold gives %q", got)
					return
				}
			}
		})
	}
	wg.Wait()
}
