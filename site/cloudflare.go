package site

import (
	"encoding/json"
	"strings"
)

// LinksFile is the map from each tag to the page of its node, as a slash path in the site. Only a Cloudflare build writes it.
const LinksFile = "api/links.json"

// worker is the _worker.js of a Cloudflare build. It sends node/TAG/ to the page of the node, and gives each other path to the static files. BASE is the URL path of the site.
const worker = `// The short links of Math 2.0. m2 site build -cloudflare writes this file.
// A request for node/TAG/ goes to the page of the node. Each other request gets the static file.
const base = BASE;
let links;

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const m = url.pathname.slice(base.length).match(/^\/node\/([0-9A-Z]+)\/?$/);
    if (!m) return env.ASSETS.fetch(request);
    if (!links) {
      const r = await env.ASSETS.fetch(new URL(base + "/` + LinksFile + `", url));
      if (!r.ok) return env.ASSETS.fetch(request);
      links = await r.json();
    }
    const to = links[m[1]];
    if (!to) return env.ASSETS.fetch(request);
    return Response.redirect(new URL(base + "/" + to, url).toString(), 302);
  },
};
`

// writeWorker writes the files that a Cloudflare build has in place of the page of each short link: the links map, the worker and its routes. The routes run the worker only for node/*, so that the other requests do not count against the limits of Workers.
func (b *builder) writeWorker() error {
	links := make(map[string]string, len(b.order))
	for _, v := range b.order {
		links[v.Tag] = "wiki/" + v.Slug + "/"
	}
	if err := b.writeJSON(LinksFile, links); err != nil {
		return err
	}
	base, err := json.Marshal(b.base)
	if err != nil {
		return err
	}
	if err := b.write("_worker.js", []byte(strings.Replace(worker, "BASE", string(base), 1))); err != nil {
		return err
	}
	routes := map[string]any{"version": 1, "include": []string{b.base + "/node/*"}, "exclude": []string{}}
	return b.writeJSON("_routes.json", routes)
}
