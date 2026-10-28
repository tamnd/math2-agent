package wpcorpus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DefaultAPI is the Action API of the English Wikipedia.
const DefaultAPI = "https://en.wikipedia.org/w/api.php"

// Fetcher gets the current revision of an article from the Action API. It
// sends one request at a time, with maxlag, as the Wikimedia rules ask.
type Fetcher struct {
	API       string // the Action API URL. Empty means DefaultAPI.
	UserAgent string
	Client    *http.Client // nil means a client with a 60 second timeout
}

type queryResponse struct {
	Error *struct {
		Code string `json:"code"`
		Info string `json:"info"`
	} `json:"error"`
	Query struct {
		Pages []struct {
			PageID    int    `json:"pageid"`
			NS        int    `json:"ns"`
			Title     string `json:"title"`
			Missing   bool   `json:"missing"`
			Invalid   bool   `json:"invalid"`
			Revisions []struct {
				RevID     int    `json:"revid"`
				Timestamp string `json:"timestamp"`
				Slots     struct {
					Main struct {
						Content string `json:"content"`
					} `json:"main"`
				} `json:"slots"`
			} `json:"revisions"`
		} `json:"pages"`
	} `json:"query"`
}

// Fetch returns the current revision of the article with the title. It
// follows a redirect, so the title of the page can differ from the title
// that was asked for. It refuses a page that is not in the article
// namespace.
func (f *Fetcher) Fetch(ctx context.Context, title string) (*Page, error) {
	v := url.Values{}
	v.Set("prop", "revisions")
	v.Set("rvprop", "ids|timestamp|content")
	v.Set("rvslots", "main")
	v.Set("redirects", "1")
	v.Set("titles", title)
	var r queryResponse
	if err := f.get(ctx, v, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", title, err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("%s: the API gave the error %s: %s", title, r.Error.Code, r.Error.Info)
	}
	if len(r.Query.Pages) != 1 {
		return nil, fmt.Errorf("%s: the API gave %d pages", title, len(r.Query.Pages))
	}
	p := r.Query.Pages[0]
	switch {
	case p.Missing || p.Invalid:
		return nil, fmt.Errorf("%s: the page does not exist", title)
	case p.NS != 0:
		return nil, fmt.Errorf("%s: the page is in namespace %d, not in the article namespace", title, p.NS)
	case len(p.Revisions) != 1:
		return nil, errors.New(title + ": the API gave no revision")
	}
	rev := p.Revisions[0]
	return &Page{ID: p.PageID, Title: p.Title, Revision: rev.RevID, Timestamp: rev.Timestamp, Wikitext: rev.Slots.Main.Content}, nil
}

// maxTries is the number of tries of one request when the API asks the
// client to wait (maxlag or HTTP 429).
const maxTries = 5

// get sends one query to the Action API and decodes the answer. It adds
// the format and maxlag. On a maxlag error or HTTP 429, it waits the
// Retry-After time and tries again (WP2.8).
func (f *Fetcher) get(ctx context.Context, v url.Values, out any) error {
	api := f.API
	if api == "" {
		api = DefaultAPI
	}
	v.Set("action", "query")
	v.Set("format", "json")
	v.Set("formatversion", "2")
	v.Set("maxlag", "5")
	c := f.Client
	if c == nil {
		c = &http.Client{Timeout: 60 * time.Second}
	}
	for try := 1; ; try++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"?"+v.Encode(), nil)
		if err != nil {
			return err
		}
		if f.UserAgent != "" {
			req.Header.Set("User-Agent", f.UserAgent)
		}
		resp, err := c.Do(req)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		wait := resp.StatusCode == http.StatusTooManyRequests
		if resp.StatusCode == http.StatusOK && strings.Contains(string(data[:min(len(data), 200)]), `"code":"maxlag"`) {
			wait = true
		}
		if wait && try < maxTries {
			d := 5 * time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
				d = time.Duration(s) * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d * time.Duration(try)):
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("the API gave status %s", resp.Status)
		}
		return json.Unmarshal(data, out)
	}
}

// Revision is one revision in the history of a page.
type Revision struct {
	ID        int      `json:"revid"`
	Parent    int      `json:"parentid"`
	Timestamp string   `json:"timestamp"`
	SHA1      string   `json:"sha1"`
	Tags      []string `json:"tags"`
}

// History gives the newest revisions of a page, the newest first. limit is
// at most 50.
func (f *Fetcher) History(ctx context.Context, page, limit int) ([]Revision, error) {
	v := url.Values{}
	v.Set("prop", "revisions")
	v.Set("pageids", strconv.Itoa(page))
	v.Set("rvprop", "ids|timestamp|sha1|tags")
	v.Set("rvlimit", strconv.Itoa(limit))
	var r struct {
		Error *struct {
			Code string `json:"code"`
			Info string `json:"info"`
		} `json:"error"`
		Query struct {
			Pages []struct {
				Missing   bool       `json:"missing"`
				Revisions []Revision `json:"revisions"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := f.get(ctx, v, &r); err != nil {
		return nil, fmt.Errorf("page %d: %w", page, err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("page %d: the API gave the error %s: %s", page, r.Error.Code, r.Error.Info)
	}
	if len(r.Query.Pages) != 1 || r.Query.Pages[0].Missing {
		return nil, fmt.Errorf("page %d: the page does not exist", page)
	}
	return r.Query.Pages[0].Revisions, nil
}

// FetchRevision gives one revision of a page with its wikitext.
func (f *Fetcher) FetchRevision(ctx context.Context, rev int) (*Page, error) {
	v := url.Values{}
	v.Set("prop", "revisions")
	v.Set("revids", strconv.Itoa(rev))
	v.Set("rvprop", "ids|timestamp|content")
	v.Set("rvslots", "main")
	var r queryResponse
	if err := f.get(ctx, v, &r); err != nil {
		return nil, fmt.Errorf("revision %d: %w", rev, err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("revision %d: the API gave the error %s: %s", rev, r.Error.Code, r.Error.Info)
	}
	if len(r.Query.Pages) != 1 || len(r.Query.Pages[0].Revisions) != 1 {
		return nil, fmt.Errorf("revision %d: the API gave no revision", rev)
	}
	p := r.Query.Pages[0]
	rv := p.Revisions[0]
	return &Page{ID: p.PageID, Title: p.Title, Revision: rv.RevID, Timestamp: rv.Timestamp, Wikitext: rv.Slots.Main.Content}, nil
}

// settleAge is the age at which a revision is settled (WP2.6).
const settleAge = 7 * 24 * time.Hour

// unstableAge is the age past which no revision is searched (WP2.6).
const unstableAge = 90 * 24 * time.Hour

// RecordRevision applies the rule of WP2.6 to the history of a page, the
// newest first. It gives the newest revision that is at least 7 days old
// at now and that no later revision reverts. A revision is reverted when it
// has the tag mw-reverted, or when a later revision has the sha1 of the
// revision before it. When no revision of the last 90 days passes, it gives
// 0 and unstable. A page with no edit in the last 90 days keeps its newest
// revision.
func RecordRevision(hist []Revision, now time.Time) (rev int, unstable bool) {
	stepped := false
	for i, r := range hist {
		t, err := time.Parse(time.RFC3339, r.Timestamp)
		if err != nil {
			continue
		}
		if now.Sub(t) < settleAge {
			continue
		}
		if stepped && now.Sub(t) > unstableAge {
			break
		}
		if slices.Contains(r.Tags, "mw-reverted") {
			stepped = true
			continue
		}
		parent := ""
		if i+1 < len(hist) {
			parent = hist[i+1].SHA1
		}
		reverted := false
		for _, later := range hist[:i] {
			if parent != "" && later.SHA1 == parent {
				reverted = true
				break
			}
		}
		if !reverted {
			return r.ID, false
		}
		stepped = true
	}
	return 0, true
}

// NewestSettled gives the newest revision of the history that is at least 7
// days old, reverted or not. Rule Y13 compares it with the revision of
// record.
func NewestSettled(hist []Revision, now time.Time) (Revision, bool) {
	for _, r := range hist {
		t, err := time.Parse(time.RFC3339, r.Timestamp)
		if err == nil && now.Sub(t) >= settleAge {
			return r, true
		}
	}
	return Revision{}, false
}
