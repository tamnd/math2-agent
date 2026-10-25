// summary.mjs CHECK REPORTS CONFIG VERSION prints the site report in Markdown. CHECK is the JSON of m2 site check, REPORTS is the output directory of Lighthouse CI and CONFIG is lighthouserc.json. It exits with 1 when a page misses a target.
import { readFileSync } from "node:fs";
import { join } from "node:path";

const [checkPath, reportDir, configPath, version] = process.argv.slice(2);
const check = JSON.parse(readFileSync(checkPath, "utf8"));
const config = JSON.parse(readFileSync(configPath, "utf8"));
const manifest = JSON.parse(readFileSync(join(reportDir, "manifest.json"), "utf8"));
const asserts = config.ci.assert.assertions;
const minA11y = asserts["categories:accessibility"][1].minScore;
const maxPaint = Math.min(asserts["first-contentful-paint"][1].maxNumericValue, asserts["largest-contentful-paint"][1].maxNumericValue);
const t = config.ci.collect.settings.throttling;

const pages = manifest.map((m) => {
  const r = JSON.parse(readFileSync(m.jsonPath, "utf8"));
  const failed = Object.entries(r.audits)
    .filter(([, a]) => a.scoreDisplayMode === "binary" && a.score === 0 && r.categories.accessibility.auditRefs.some((ref) => ref.id === a.id))
    .map(([id]) => id);
  return {
    page: new URL(m.url).pathname.replace(/^\/math2/, ""),
    a11y: r.categories.accessibility.score,
    failed,
    fcp: Math.round(r.audits["first-contentful-paint"].numericValue),
    lcp: Math.round(r.audits["largest-contentful-paint"].numericValue),
  };
});

const problems = (target) => (check.Problems || []).filter((p) => p.Target === target);
const heaviest = check.Heaviest[0];
const slowest = pages.reduce((a, b) => (Math.max(b.fcp, b.lcp) > Math.max(a.fcp, a.lcp) ? b : a));
const a11yOK = pages.every((p) => p.a11y >= minA11y);
const paintOK = pages.every((p) => p.fcp < maxPaint && p.lcp < maxPaint);
const mark = (ok) => (ok ? "pass" : "**FAIL**");

const rows = [
  ["Each page takes less than 50 KB on the network before math fonts", problems("weight").length === 0, `the heaviest page is ${heaviest.Page}, ${heaviest.Bytes} bytes with gzip`],
  ["Each formula has MathML", problems("mathml").length === 0, `${check.Formulas} formulas`],
  ["A reader can read each page with JavaScript off", problems("nojs").length === 0, "the math is HTML and MathML from the build, and each script is deferred"],
  ["Each figure has a list with the same data", problems("lists").length === 0, `${check.Figures} figures`],
  ["Each page loads files from the site only, so an archive works offline", problems("offline").length === 0, `${check.Pages} pages`],
  ["Each page passes each accessibility audit of Lighthouse", a11yOK, `${pages.length} pages, one for each template`],
  [`The first paint and the largest paint come in less than ${maxPaint} ms`, paintOK, `the slowest page is ${slowest.page}, at ${Math.max(slowest.fcp, slowest.lcp)} ms`],
];

const out = [];
out.push(`# Site report`, ``);
out.push(`This report tests the wiki against the targets of spec 13.7. The site is the site of the fixtures in testdata/nodes/valid, built by ${version}.`, ``);
out.push(`Lighthouse simulates a mid-range phone on 4G: a round trip of ${t.rttMs} ms, ${t.throughputKbps / 1000} Mbps down, ${t.uploadThroughputKbps / 1000} Mbps up and a CPU that is ${t.cpuSlowdownMultiplier} times slower. These are the Fast 4G profile of Chrome DevTools and the mid-tier phone of Lighthouse.`, ``);
out.push(`| Target | Result | Detail |`, `|---|---|---|`);
for (const [target, ok, detail] of rows) out.push(`| ${target} | ${mark(ok)} | ${detail} |`);
out.push(``, `## Pages`, ``, `| Page | Accessibility | First paint (ms) | Largest paint (ms) |`, `|---|---|---|---|`);
for (const p of pages) out.push(`| ${p.page} | ${Math.round(p.a11y * 100)}${p.failed.length ? " (" + p.failed.join(", ") + ")" : ""} | ${p.fcp} | ${p.lcp} |`);
if ((check.Problems || []).length) {
  out.push(``, `## Problems from m2 site check`, ``);
  for (const p of check.Problems) out.push(`- ${p.Page}: ${p.Target}: ${p.Message}`);
}
console.log(out.join("\n"));
process.exit(rows.every(([, ok]) => ok) ? 0 : 1);
