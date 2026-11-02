package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
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
const shardBytes = 1 << 20

// sharded matches the paths of the files that a Cloudflare build puts in shards: the files of the nodes, the resources and the inventory, except the index page of each of these sections.
var sharded = regexp.MustCompile(`^(?:(?:wiki|res|inventory)/.+/index\.html|node/[0-9A-Za-z~]+\.md|api/node/[0-9A-Za-z~]+\.json)$`)

// shardOf returns the shard of the path p, for n shards. The worker computes the same hash.
func shardOf(p string, n int) int {
	h := fnv.New32a()
	h.Write([]byte(p))
	return int(h.Sum32() % uint32(n)) // #nosec G115 -- n is a small positive count.
}

// worker is the _worker.js of a Cloudflare build. BASE is the URL path of the site and SHARDS the count of shards.
// It sends node/TAG/ to the page of the node, it serves each sharded file from its shard, and it gives each other path to the static files.
const worker = `// The worker of Math 2.0. m2 site build -cloudflare writes this file.
// A request for node/TAG/ goes to the page of the node. A request for a file of a node, a resource or the inventory gets the file from its shard. Each other request gets the static file.
const base = BASE;
const shards = SHARDS;
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
  async fetch(request, env) {
    const url = new URL(request.url);
    const path = url.pathname.slice(base.length).replace(/^\/+/, "");
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
      const body = await lookup(env, url, p);
      if (body !== undefined) {
        const type = types[p.slice(p.lastIndexOf(".") + 1)] || "application/octet-stream";
        return new Response(request.method === "HEAD" ? null : body, {
          headers: { "content-type": type, "cache-control": "public, max-age=0, must-revalidate" },
        });
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
	if err := b.write("_worker.js", []byte(w)); err != nil {
		return err
	}
	var include []string
	for _, p := range []string{"node", "wiki", "res", "inventory", "api/node"} {
		include = append(include, b.base+"/"+p+"/*")
	}
	routes := map[string]any{"version": 1, "include": include, "exclude": []string{}}
	return b.writeJSON("_routes.json", routes)
}

// writeShards writes the sharded files to shards/N.json and returns the count of shards.
func (b *builder) writeShards() (int, error) {
	total := 0
	for _, data := range b.shards {
		total += len(data)
	}
	n := 16
	for n*shardBytes < total && n < 4096 {
		n *= 2
	}
	parts := make([]map[string]string, n)
	for p, data := range b.shards {
		i := shardOf(p, n)
		if parts[i] == nil {
			parts[i] = map[string]string{}
		}
		parts[i][p] = string(data)
	}
	shards := b.shards
	b.shards = nil // write the shards themselves to files
	defer func() { b.shards = shards }()
	for i, part := range parts {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if part == nil {
			part = map[string]string{}
		}
		if err := enc.Encode(part); err != nil {
			return 0, err
		}
		if err := b.write(fmt.Sprintf("%s/%d.json", ShardDir, i), buf.Bytes()); err != nil {
			return 0, err
		}
	}
	return n, nil
}
