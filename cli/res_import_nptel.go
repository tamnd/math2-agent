package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/nptel"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/nptelvideo"
)

// nptelPageURL gives the URL of the data file of the page of a course of NPTEL. A test sets it to a local server.
var nptelPageURL = nptel.PageURL

// ytbUploaded gives the upload day of a video with the fetcher ytb at the path bin. ytb gives a JSON array with one object for each video. A test sets it to a fake.
var ytbUploaded = func(ctx context.Context, bin, video string) (string, error) {
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "video", "-o", "json", "--", video) // #nosec G204 -- the user names the fetcher, and the video id comes from the page of NPTEL with no shell. A video id can start with "-", so "--" ends the flags.
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ytb video %s: %w: %s", video, err, bytes.TrimSpace(errb.Bytes()))
	}
	var vs []struct {
		UploadDate string `json:"upload_date"`
	}
	if err := json.Unmarshal(out.Bytes(), &vs); err != nil {
		return "", fmt.Errorf("ytb video %s: %w", video, err)
	}
	for _, v := range vs {
		if t, err := time.Parse(time.RFC3339, v.UploadDate); err == nil {
			return t.Format(time.DateOnly), nil
		}
	}
	return "", nil
}

func runResImportNptelVideo(root, bin string, gap time.Duration, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res import: %s: %v\n", nptelvideo.Source, err)
		return ExitFailure
	}
	if bin == "" {
		bin = "ytb"
	}
	rs, err := resource.ReadResources(root)
	if err != nil {
		return fail(err)
	}
	c := fetch.NewClient(gap)
	page := func(ctx context.Context, id string) ([]nptel.Unit, error) {
		b, err := c.Get(ctx, nptelPageURL(id))
		if errors.Is(err, fetch.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return nptel.Outline(bytes.NewReader(b))
	}
	uploaded := func(ctx context.Context, video string) (string, error) {
		day, err := ytbUploaded(ctx, bin, video)
		if err != nil {
			fmt.Fprintf(stderr, "m2 res import: %s: %v, so the course has no date\n", nptelvideo.Source, err)
		}
		return day, nil
	}
	recs, outs, n, err := nptelvideo.Build(context.Background(), rs, page, uploaded)
	if err != nil {
		return fail(err)
	}
	kept, rejected, hash, err := saveBuilt(root, nptelvideo.Entry(), nptelvideo.Snapshot, recs)
	if err != nil {
		return fail(err)
	}
	written, missing, err := putStructures(root, nptelvideo.Source, hash, func(emit func(id string, nodes []resource.Node) error) error {
		for _, o := range outs {
			if err := emit(o.ID, o.Nodes); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%s: asked %d, found %d, records %d, rejected %d, outlines %d, %d with no resource, snapshot %s\n", nptelvideo.Source, n.Asked, n.Found, kept, rejected, written, missing, hash)
	return ExitOK
}
