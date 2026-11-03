package site

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// LinksFile is the map from each tag to the page of its node, as a slash path in the site. Only a Cloudflare build writes it.
const LinksFile = "api/links.json"

// ShardDir is the directory of the shards of a Cloudflare build.
//
// Cloudflare Pages takes at most 20,000 files in one deployment, and the site has a page, a Markdown file and a JSON file for each node, and a page for each resource and each part of the inventory.
// A Cloudflare build puts these files in shards instead: shards/N.json is an object from the path of each file to its content, and the worker serves each file from its shard.
// The shard of a path is the FNV-1a hash of the path modulo the count of shards. The build chooses the count so that a shard has about shardBytes bytes.
const ShardDir = "shards"

// shardBytes is the size of a shard that the build aims for.
const shardBytes = 512 << 10

// maxSharded is the size above which a file stays a static file, so that the worker does not read a large shard for it. The worker gives a path that is not in its shard to the static files.
const maxSharded = 512 << 10

// sharded matches the paths of the files that a Cloudflare build puts in shards: the files of the nodes, the resources and the inventory, except the index page of each of these sections.
var sharded = regexp.MustCompile(`^(?:(?:wiki|res|inventory)/.+/index\.html|node/[0-9A-Za-z~]+\.md|api/node/[0-9A-Za-z~]+\.json)$`)

// shardOf returns the shard of the path p, for n shards. The worker computes the same hash.
func shardOf(p string, n int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(p))
	return int(h.Sum32() % uint32(n)) // #nosec G115 -- n is a small positive count.
}

// worker is the _worker.js of a Cloudflare build. BASE is the URL path of the site and SHARDS the count of shards.
// It sends node/TAG/ to the page of the node, it serves each sharded file from its shard, and it gives each other path to the static files.
const worker = `// The worker of Math 2.0. m2 site build -cloudflare writes this file.
// A request for node/TAG/ goes to the page of the node. A request for a file of a node, a resource or the inventory gets the file from its shard. Each other request gets the static file.
const base = BASE;
const shards = SHARDS;
const cacheVersion = CACHE_VERSION;
const socialPacks = SOCIAL_PACKS;
const sharded = /^(?:(?:wiki|res|inventory)\/.+|node\/[0-9A-Za-z~]+\.md|api\/node\/[0-9A-Za-z~]+\.json)$/;
const types = { html: "text/html; charset=utf-8", md: "text/markdown; charset=utf-8", json: "application/json; charset=utf-8" };
const cache = new Map();
let links;

// shardOf is the FNV-1a hash of the path modulo the count of shards, as in the build.
function shardOf(p) {
  let h = 0x811c9dc5;
  for (const c of new TextEncoder().encode(p)) {
    h ^= c;
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h % shards;
}

async function lookup(env, url, p) {
  const i = shardOf(p);
  let s = cache.get(i);
  if (!s) {
    const r = await env.ASSETS.fetch(new URL(base + "/` + ShardDir + `/" + i + ".json", url));
    if (!r.ok) return undefined;
    s = await r.json();
    if (cache.size >= 48) cache.delete(cache.keys().next().value);
    cache.set(i, s);
  }
  return s[p];
}

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    const path = url.pathname.slice(base.length).replace(/^\/+/, "");
    if (socialPacks && path.startsWith("social/")) {
      const image = path.match(/^social\/([0-9a-f]{24})\/([0-9]+)\/([0-9]+)\/([0-9A-Za-z]+)\.jpg$/);
      if (!image) return env.ASSETS.fetch(request);
      if (request.method !== "GET" && request.method !== "HEAD") return new Response("Method not allowed", { status: 405 });
      const offset = Number(image[2]);
      const length = Number(image[3]);
      if (!Number.isSafeInteger(offset) || !Number.isSafeInteger(length) || length < 1 || length > 250000) return new Response("Not found", { status: 404 });
      const key = new Request(new URL(url.pathname, url));
      if (request.method === "GET") {
        const hit = await caches.default.match(key);
        if (hit) return hit;
      }
      let body;
      let source = "r2";
      if (env.SOCIAL_PACKS) {
        try {
          const object = await env.SOCIAL_PACKS.get("social/" + image[1] + ".pack", { range: { offset, length } });
          if (object) body = object.body;
        } catch (_) { /* The Pages pack below keeps the image available. */ }
      }
      if (!body) {
        source = "pages";
        // Pages does not yet return 206 for ranges. Read one small pack and
        // slice it, or use the partial body when Pages gains range support.
        const packURL = new URL(base + "/social/packs/" + image[1] + ".pack", url);
        const pack = await env.ASSETS.fetch(new Request(packURL, { headers: { range: "bytes=" + offset + "-" + (offset + length - 1) } }));
        if (!pack.ok) return new Response("Image not found", { status: 404 });
        const bytes = await pack.arrayBuffer();
        body = pack.status === 206 ? bytes : bytes.slice(offset, offset + length);
        if (body.byteLength !== length) return new Response("Image not found", { status: 404 });
      }
      const response = new Response(request.method === "HEAD" ? null : body, {
        headers: { "content-type": "image/jpeg", "content-length": String(length), "cache-control": "public, max-age=31536000, immutable", "x-math2-image-source": source },
      });
      if (request.method === "GET") ctx.waitUntil(caches.default.put(key, response.clone()));
      return response;
    }
    const m = path.match(/^node\/([0-9A-Z]+)\/?$/);
    if (m) {
      if (!links) {
        const r = await env.ASSETS.fetch(new URL(base + "/` + LinksFile + `", url));
        if (!r.ok) return env.ASSETS.fetch(request);
        links = await r.json();
      }
      const to = links[m[1]];
      if (!to) return env.ASSETS.fetch(request);
      return Response.redirect(new URL(base + "/" + to, url).toString(), 302);
    }
    if ((request.method === "GET" || request.method === "HEAD") && sharded.test(path)) {
      let p = decodeURIComponent(path);
      if (p.endsWith("/")) p += "index.html";
      else if (!/\.(md|json|html)$/.test(p)) {
        if ((await lookup(env, url, p + "/index.html")) !== undefined) {
          return Response.redirect(new URL(url.pathname + "/" + url.search, url).toString(), 301);
        }
        return env.ASSETS.fetch(request);
      }
      // Cached responses are keyed by the build, so a new deployment cannot
      // reuse the content of a previous shard even when the URL is unchanged.
      const key = new Request(new URL(base + "/__cache/" + cacheVersion + "/" + p, url));
      if (request.method === "GET") {
        const hit = await caches.default.match(key);
        if (hit) return hit;
      }
      const body = await lookup(env, url, p);
      if (body !== undefined) {
        const type = types[p.slice(p.lastIndexOf(".") + 1)] || "application/octet-stream";
        const response = new Response(request.method === "HEAD" ? null : body, {
          headers: { "content-type": type, "cache-control": "public, max-age=60, s-maxage=86400" },
        });
        if (request.method === "GET") ctx.waitUntil(caches.default.put(key, response.clone()));
        return response;
      }
    }
    return env.ASSETS.fetch(request);
  },
};
`

// writeWorker writes the files that a Cloudflare build has in place of the page of each short link and of each sharded file: the links map, the shards, the worker and its routes.
// The routes run the worker only for the short links and the sharded sections, so that the other requests do not count against the limits of Workers.
func (b *builder) writeWorker() error {
	links := make(map[string]string, len(b.order))
	for _, v := range b.order {
		links[v.Tag] = "wiki/" + v.Slug + "/"
	}
	if err := b.writeJSON(LinksFile, links); err != nil {
		return err
	}
	n, err := b.writeShards()
	if err != nil {
		return err
	}
	base, err := json.Marshal(b.base)
	if err != nil {
		return err
	}
	w := strings.Replace(worker, "BASE", string(base), 1)
	w = strings.Replace(w, "SHARDS", strconv.Itoa(n), 1)
	version, err := json.Marshal(fmt.Sprintf("%s-%s-%d", b.opt.Commit, b.opt.Version, b.opt.Date.Unix()))
	if err != nil {
		return err
	}
	w = strings.Replace(w, "CACHE_VERSION", string(version), 1)
	w = strings.Replace(w, "SOCIAL_PACKS", "true", 1)
	if err := b.write("_worker.js", []byte(w)); err != nil {
		return err
	}
	if err := b.write("_headers", []byte("/social/*\n  Cache-Control: public, max-age=31536000, immutable\n/assets/*\n  Cache-Control: public, max-age=3600, stale-while-revalidate=86400\n")); err != nil {
		return err
	}
	var include []string
	for _, p := range []string{"node", "wiki", "res", "inventory", "api/node"} {
		include = append(include, b.base+"/"+p+"/*")
	}
	include = append(include, b.base+"/social/*")
	routes := map[string]any{"version": 1, "include": include, "exclude": []string{}}
	return b.writeJSON("_routes.json", routes)
}

// writeShards writes the sharded files to shards/N.json and returns the count of shards. It reads the spool of one shard at a time, so that the build does not hold all the sharded files in memory.
func (b *builder) writeShards() (int, error) {
	sp := b.spool
	b.spool = nil // write the shards themselves to files
	defer func() { b.spool = sp }()
	n := 16
	for n*shardBytes < sp.total && n < spoolBuckets {
		n *= 2
	}
	for i := range n {
		part := map[string]string{}
		// n divides spoolBuckets, so bucket j holds paths of shard j mod n.
		for j := i; j < spoolBuckets; j += n {
			if err := sp.read(j, func(p string, data []byte) { part[p] = string(data) }); err != nil {
				return 0, err
			}
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(part); err != nil {
			return 0, err
		}
		if err := b.write(fmt.Sprintf("%s/%d.json", ShardDir, i), buf.Bytes()); err != nil {
			return 0, err
		}
	}
	return n, os.RemoveAll(sp.dir)
}

// spoolDir is the directory in the output where a Cloudflare build keeps the sharded files until it writes the shards. writeShards removes it.
const spoolDir = ".shard-spool"

// spoolBuckets is the count of spool files, and the largest count of shards. It is a power of two, as each count of shards is.
const spoolBuckets = 4096

// shardSpool keeps the sharded files of a build on disk: bucket j holds the files whose path hashes to j modulo spoolBuckets, as records of the path and the data, each with its length.
type shardSpool struct {
	dir   string
	total int // the bytes of the files
}

func (s *shardSpool) bucket(j int) string { return filepath.Join(s.dir, strconv.Itoa(j)) }

func (s *shardSpool) add(p string, data []byte) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil { // #nosec G301 -- the site is public.
		return err
	}
	f, err := os.OpenFile(s.bucket(shardOf(p, spoolBuckets)), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644) // #nosec G302 G304 -- as in write.
	if err != nil {
		return err
	}
	rec := binary.AppendUvarint(nil, uint64(len(p)))
	rec = append(rec, p...)
	rec = binary.AppendUvarint(rec, uint64(len(data)))
	rec = append(rec, data...)
	if _, err := f.Write(rec); err != nil {
		_ = f.Close()
		return err
	}
	s.total += len(data)
	return f.Close()
}

// read calls f for each file in bucket j, in the order of add. A later file with the same path replaces an earlier one.
func (s *shardSpool) read(j int, f func(p string, data []byte)) error {
	data, err := os.ReadFile(s.bucket(j)) // #nosec G304 -- the build makes the path.
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for len(data) > 0 {
		var fields [2][]byte
		for k := range fields {
			l, m := binary.Uvarint(data)
			if m <= 0 || uint64(len(data)-m) < l { // #nosec G115 -- len(data)-m is not negative.
				return fmt.Errorf("site: the shard spool %d is damaged", j)
			}
			fields[k] = data[m : m+int(l)] // #nosec G115 -- l is at most len(data)-m.
			data = data[m+int(l):]         // #nosec G115 -- l is at most len(data)-m.
		}
		f(string(fields[0]), fields[1])
	}
	return nil
}
