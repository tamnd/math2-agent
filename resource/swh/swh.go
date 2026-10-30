// Package swh builds the source records of Software Heritage (spec corpus resources RS3.2 and RS7.5, phase R3). The source gives the SWHID, the key swh, to the software whose source record links to a repository on a forge.
//
// For each such record, the source asks the public API of Software Heritage for the latest visit of the repository, as an origin, with a snapshot. The SWHID is the id of that snapshot, swh:1:snp:<hash>. The record of the source has the SWHID and the strong ids of the source record that it enriches, so the clustering puts the SWHID on the resource that has the same swMATH id or DOI.
//
// The API needs no key. Its terms allow access item by item and forbid massive extraction, and the server gives its quota in the headers X-RateLimit-Limit, X-RateLimit-Remaining and X-RateLimit-Reset. The client keeps a gap between two requests and spreads the requests that remain over the time to the reset. A run reuses the SWHIDs of the last run, and asks only for the origins that have none.
package swh

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Source is the name of the source in manifests/sources.yaml.
const Source = "swh"

// Version is the version of the builder.
const Version = "swh/1"

// API is the public API of Software Heritage.
const API = "https://archive.softwareheritage.org/api/1"

// forges are the hosts whose repository URL has the form https://<host>/<owner>/<name>.
var forges = map[string]bool{"github.com": true, "bitbucket.org": true, "codeberg.org": true}

// gitlabs are the hosts of GitLab, where a repository can be in a subgroup, and the path ends before /-/.
var gitlabs = map[string]bool{"gitlab.com": true, "framagit.org": true, "salsa.debian.org": true, "gitlab.inria.fr": true}

// Origin gives the URL of the repository of a link to a forge, as Software Heritage names the origin, or "" when the link is not a link to a repository on a known forge. It removes the www prefix, a trailing slash, the suffix .git and the path below the repository.
func Origin(link string) string {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	path := strings.Trim(u.Path, "/")
	switch {
	case forges[host]:
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			return ""
		}
		path = parts[0] + "/" + parts[1]
	case gitlabs[host]:
		path, _, _ = strings.Cut(path, "/-/")
		path = strings.Trim(path, "/")
		if strings.Count(path, "/") < 1 {
			return ""
		}
	default:
		return ""
	}
	path = strings.TrimSuffix(path, ".git")
	if path == "" || strings.ContainsAny(path, " ?#") || strings.HasSuffix(path, "/") {
		return ""
	}
	return "https://" + host + "/" + path
}

// Visit gives the hash of the snapshot of the latest visit of the origin, or "" when Software Heritage does not have the origin or has no visit with a snapshot.
type Visit func(ctx context.Context, origin string) (string, error)

// Client asks the API for the latest visits. It keeps the quota that the server gives in its headers.
type Client struct {
	HTTP      *http.Client
	Base      string        // the API, API by default
	UserAgent string        // the User-Agent of each request
	Gap       time.Duration // the shortest time between two requests
	Retries   int           // the retries of one request after the status 429 or 5xx

	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error

	mu        sync.Mutex
	last      time.Time
	remaining int // the requests that remain in the window, or -1 when no answer gave the header
	reset     time.Time
}

// NewClient makes a client with the gap.
func NewClient(userAgent string, gap time.Duration) *Client {
	return &Client{HTTP: &http.Client{Timeout: 60 * time.Second}, Base: API, UserAgent: userAgent, Gap: gap, Retries: 3, remaining: -1}
}

// wait gives the time to wait before the next request. It is the gap since the last request, or the time to the reset divided by the requests that remain, whichever is longer. With no request left, it is the time to the reset.
func (c *Client) wait(now time.Time) time.Duration {
	d := c.Gap - now.Sub(c.last)
	if c.remaining >= 0 && c.reset.After(now) {
		left := c.reset.Sub(now)
		if c.remaining == 0 {
			return left
		}
		if spread := left / time.Duration(c.remaining); spread > d {
			d = spread
		}
	}
	return max(d, 0)
}

// quota reads the headers of the quota of one answer.
func (c *Client) quota(h http.Header) {
	rem, err1 := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	reset, err2 := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	if err1 != nil || err2 != nil {
		return
	}
	c.remaining, c.reset = rem, time.Unix(reset, 0)
}

// Latest gives the hash of the snapshot of the latest visit of the origin, with the signature of Visit.
func (c *Client) Latest(ctx context.Context, origin string) (string, error) {
	if c.now == nil {
		c.now = time.Now
	}
	if c.sleep == nil {
		c.sleep = sleep
	}
	base := c.Base
	if base == "" {
		base = API
	}
	u := base + "/origin/" + origin + "/visit/latest/?require_snapshot=true"
	var last error
	for try := 0; try <= c.Retries; try++ {
		c.mu.Lock()
		d := c.wait(c.now())
		if try > 0 {
			d = max(d, time.Duration(1<<try)*time.Second)
		}
		err := c.sleep(ctx, d)
		c.last = c.now()
		c.mu.Unlock()
		if err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTP.Do(req) // #nosec G107 G704 -- the URL is the fixed API with an origin of a known forge.
		if err != nil {
			last = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		c.mu.Lock()
		c.quota(resp.Header)
		c.mu.Unlock()
		switch {
		case err != nil:
			last = err
		case resp.StatusCode == http.StatusOK:
			var v struct {
				Snapshot string `json:"snapshot"`
			}
			if err := json.Unmarshal(body, &v); err != nil {
				return "", fmt.Errorf("swh: %s: %w", origin, err)
			}
			return v.Snapshot, nil
		case resp.StatusCode == http.StatusNotFound:
			return "", nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			last = fmt.Errorf("swh: %s: status %d", origin, resp.StatusCode)
			c.mu.Lock()
			if resp.StatusCode == http.StatusTooManyRequests && c.remaining < 0 {
				c.remaining, c.reset = 0, c.now().Add(time.Minute)
			}
			c.mu.Unlock()
		default:
			return "", fmt.Errorf("swh: %s: status %d: %.200s", origin, resp.StatusCode, body)
		}
	}
	return "", last
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ID gives the SWHID of the snapshot with the hash, or an error when the hash is not 40 hexadecimal digits.
func ID(snapshot string) (string, error) {
	return ids.Normalize(Source, "swh:1:snp:"+strings.ToLower(snapshot))
}

// Counts are the counts of one run.
type Counts struct {
	Origins int // the records of software with a repository on a known forge
	Reused  int // the records whose origin has a SWHID already, from the last run or from an earlier record
	Asked   int // the origins that the run asked for
	Found   int // the origins that Software Heritage has with a snapshot
	Left    int // the origins that the run did not ask for, because of the limit or an error, and the origin of the error
}

// keys gives the strong ids of the record, which join the record of the source with the resource.
func keys(r *cluster.Record) resource.IDs {
	out := resource.IDs{}
	for _, k := range cluster.Strong {
		if v := r.IDs[k]; len(v) > 0 {
			out[k] = slices.Clone(v)
		}
	}
	return out
}

// Build gives one source record for each source record of kind software that links to a repository on a known forge, has a strong id, and whose origin has a SWHID. The native id of the record is the key of the source record that it enriches, so two packages in one repository stay two records. The records of the last run, prev, give the SWHIDs of their origins again with no request. A run asks for at most limit new origins, and 0 means no limit. When a request fails, the run asks for no more origins, and gives the records that it has with the error, so that the caller can keep them.
func Build(ctx context.Context, recs, prev []cluster.Record, visit Visit, limit int) ([]cluster.Record, Counts, error) {
	var c Counts
	known := map[string]string{}
	for _, p := range prev {
		if len(p.Links) > 0 && p.IDs.Get(Source) != "" {
			known[p.Links[0]] = p.IDs.Get(Source)
		}
	}
	asked := map[string]bool{}
	var out []cluster.Record
	var stop error
	for i := range recs {
		r := &recs[i]
		if r.Source == Source || r.Kind != "software" {
			continue
		}
		origin := ""
		for _, l := range r.Links {
			if origin = Origin(l); origin != "" {
				break
			}
		}
		strong := keys(r)
		if origin == "" || len(strong) == 0 {
			continue
		}
		c.Origins++
		id, ok := known[origin]
		switch {
		case ok:
			c.Reused++
		case asked[origin]:
			continue
		case stop != nil || (limit > 0 && c.Asked >= limit):
			c.Left++
			continue
		default:
			asked[origin] = true
			c.Asked++
			snp, err := visit(ctx, origin)
			if err != nil {
				stop = fmt.Errorf("swh: %s: %w", origin, err)
				c.Left++
				continue
			}
			if snp == "" {
				continue
			}
			if id, err = ID(snp); err != nil {
				stop = fmt.Errorf("swh: %s: %w", origin, err)
				continue
			}
			c.Found++
			known[origin] = id
		}
		strong[Source] = resource.Values{id}
		out = append(out, cluster.Record{Source: Source, NativeID: r.Key(), Level: "expression", Kind: "software", Title: r.Title, IDs: strong, Links: []string{origin}})
	}
	return out, c, stop
}

// Entry gives the entry of the source in manifests/sources.yaml. A SWHID is an intrinsic hash and a fact, and the source keeps the ids only.
func Entry() ingest.Source {
	return ingest.Source{
		ID:          Source,
		Name:        "Software Heritage, the SWHIDs of the repositories of software",
		Kind:        "catalogue",
		Home:        "https://www.softwareheritage.org",
		Licence:     "LicenseRef-facts",
		Class:       ingest.RecordOnly,
		Channel:     "api",
		Adapter:     Version,
		Terms:       &ingest.Terms{Rate: "the quota of the headers X-RateLimit-*, and one request a second at most", AI: "none stated"},
		Attribution: "Software Heritage, https://www.softwareheritage.org",
	}
}

// Snapshot gives the snapshot of a set of records: the hash of the records, on the day.
func Snapshot(recs []cluster.Record, day string) (ingest.Snapshot, error) {
	h := blake3.New()
	enc := json.NewEncoder(h)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			return ingest.Snapshot{}, err
		}
	}
	return ingest.Snapshot{URL: "m2 res import swh", Hash: "blake3:" + hex.EncodeToString(h.Sum(nil)), Date: day}, nil
}
