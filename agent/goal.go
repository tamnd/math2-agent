package agent

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/tamnd/math2-agent/workspace"
)

// OnChanges returns a [Goal] that runs check on the changes of the work tree. The goal runs check again only when the changes are not the same as at the last call, thus a turn that changes no file costs no build. The goal does not keep the result of a check that gave an error.
func OnChanges(ws *workspace.Workspace, check func(ctx context.Context, changes []workspace.Change) (done bool, why string, err error)) Goal {
	var (
		last string
		done bool
		why  string
	)
	return func(ctx context.Context) (bool, string, error) {
		changes, err := ws.Changes(ctx)
		if err != nil {
			return false, "", err
		}
		key := changesKey(changes)
		if key == last {
			return done, why, nil
		}
		d, w, err := check(ctx, changes)
		if err != nil {
			last = ""
			return false, "", err
		}
		last, done, why = key, d, w
		return done, why, nil
	}
}

// changesKey returns a hash of the changes. Each part has its length first, thus two different lists of changes do not give the same text.
func changesKey(changes []workspace.Change) string {
	h := sha256.New()
	part := func(b []byte, present bool) {
		n := int64(len(b))
		if !present {
			n = -1
		}
		_ = binary.Write(h, binary.BigEndian, n)
		h.Write(b)
	}
	for _, c := range changes {
		part([]byte(c.Path), true)
		part(c.Old, c.Old != nil)
		part(c.New, c.New != nil)
	}
	return hex.EncodeToString(h.Sum(nil))
}
