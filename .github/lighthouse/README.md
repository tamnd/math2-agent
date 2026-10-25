# Lighthouse

The job `site targets` of the ci workflow runs Lighthouse CI on one page of each template of the wiki. It checks two targets of spec 13.7 that `m2 site check` cannot check, because they need a browser:

- Each page passes each accessibility audit of Lighthouse.
- The first contentful paint and the largest contentful paint come in less than 1 s on a mid-range phone on 4G.

The network is the Fast 4G profile of Chrome DevTools: a round trip of 60 ms, 9 Mbps down and 1.5 Mbps up. The phone is the CPU slowdown of 4 that Lighthouse uses for a mid-tier phone. Lighthouse simulates both from one load of each page.

package-lock.json pins each package. To change the version, edit package.json, then run `npm install --package-lock-only --ignore-scripts` in this directory.

report.sh does each step. It builds the site of the fixtures with a given m2 binary, runs `m2 site check`, serves the site with `m2 site serve` on 127.0.0.1:8000 and runs Lighthouse CI. Then summary.mjs writes one Markdown report with a row for each target and a row for each page. The ci workflow and the release workflow both run it. To run it on your machine, from the root of the repository:

```sh
go build -o /tmp/m2 ./cmd/m2
npm ci --ignore-scripts --prefix .github/lighthouse
.github/lighthouse/report.sh /tmp/m2 /tmp/report/site-report.md
```

The Lighthouse reports go to /tmp/report/lighthouse-report. Port 8000 must be free.
