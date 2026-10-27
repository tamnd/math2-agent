package wpcorpus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	api := f.API
	if api == "" {
		api = DefaultAPI
	}
	v := url.Values{}
	v.Set("action", "query")
	v.Set("format", "json")
	v.Set("formatversion", "2")
	v.Set("maxlag", "5")
	v.Set("prop", "revisions")
	v.Set("rvprop", "ids|timestamp|content")
	v.Set("rvslots", "main")
	v.Set("redirects", "1")
	v.Set("titles", title)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	c := f.Client
	if c == nil {
		c = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: the API gave status %s", title, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	var r queryResponse
	if err := json.Unmarshal(data, &r); err != nil {
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
