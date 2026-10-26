// The scripts of the Math 2.0 wiki: the colour theme, copy buttons, list filters and search. Each page can be read with no script. The build renders the math.
(function () {
  "use strict";

  var base = document.body.dataset.base || "/";
  var root = document.documentElement;

  // The colour theme. The button changes between light and dark and keeps the choice.
  var button = document.querySelector(".theme");
  if (button) {
    button.addEventListener("click", function () {
      var dark = root.dataset.theme === "dark" || (root.dataset.theme === "" && matchMedia("(prefers-color-scheme: dark)").matches);
      root.dataset.theme = dark ? "light" : "dark";
      localStorage.setItem("m2-theme", root.dataset.theme);
    });
  }

  // Click a tag or a URI to copy it.
  document.querySelectorAll("code.copy").forEach(function (el) {
    el.addEventListener("click", function () {
      if (!navigator.clipboard) return;
      navigator.clipboard.writeText(el.textContent).then(function () {
        var old = el.title;
        el.title = "Copied";
        setTimeout(function () { el.title = old; }, 1200);
      });
    });
  });

  // Filter a list of nodes.
  document.querySelectorAll("input.filter").forEach(function (input) {
    var table = input.nextElementSibling;
    input.addEventListener("input", function () {
      var q = input.value.toLowerCase();
      table.querySelectorAll("tbody tr").forEach(function (tr) {
        tr.hidden = q !== "" && tr.textContent.toLowerCase().indexOf(q) < 0;
      });
    });
  });

  // Search. The index is api/search.json. A page on disk cannot read it, so the offline archive sets window.m2Search instead. A match on the tag or the name ranks first.
  var box = document.getElementById("q");
  if (!box) return;
  var list = document.getElementById("results");
  var status = document.getElementById("search-status");
  var index = null;

  function fold(s) {
    return (s || "").toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "");
  }

  // tagForm gives the tag that q names when q is a tag with lowercase letters, hyphens or the letters I, L and O, as tag.Normalize in Go reads it. It gives "" for other text.
  function tagForm(q) {
    var t = q.toUpperCase().replace(/-/g, "").replace(/[IL]/g, "1").replace(/O/g, "0");
    return /^([0-9A-HJKMNP-TV-Z]{6}|[0-9A-HJKMNP-TV-Z]{10})$/.test(t) ? t : "";
  }

  function score(entry, words, tq) {
    if (tq && entry.t === tq) return 1000;
    var name = fold(entry.n);
    var aliases = fold((entry.a || []).join(" "));
    var rest = fold(entry.t + " " + entry.k + " " + (entry.m || []).join(" "));
    var total = 0;
    for (var i = 0; i < words.length; i++) {
      var w = words[i];
      if (fold(entry.t) === w) total += 100;
      else if (name.indexOf(w) === 0) total += 20;
      else if (name.indexOf(w) >= 0) total += 10;
      else if (aliases.indexOf(w) >= 0) total += 6;
      else if (rest.indexOf(w) >= 0) total += 3;
      else return 0;
    }
    return total;
  }

  function show() {
    var q = box.value.trim();
    var url = new URL(location.href);
    if (q) url.searchParams.set("q", q); else url.searchParams.delete("q");
    // A browser can refuse this for a page on disk.
    try { history.replaceState(null, "", url); } catch (e) { /* keep the old URL */ }
    list.textContent = "";
    if (!q) { status.textContent = "Type to search the nodes and the spec documents."; return; }
    if (!index) { status.textContent = "Loading the index."; return; }
    var words = fold(q).split(/\s+/);
    var tq = tagForm(q);
    var hits = index.map(function (e) { return { e: e, s: score(e, words, tq) }; })
      .filter(function (h) { return h.s > 0; })
      .sort(function (a, b) { return b.s - a.s || a.e.n.localeCompare(b.e.n); })
      .slice(0, 50);
    status.textContent = hits.length === 0 ? "No results. The graph is small now, so many concepts are not in it yet." : hits.length + (hits.length === 1 ? " result" : " results");
    hits.forEach(function (h) {
      var li = document.createElement("li");
      var a = document.createElement("a");
      a.href = /^(\/|[a-z]+:)/.test(h.e.u) ? h.e.u : base + h.e.u;
      a.textContent = h.e.n;
      var meta = document.createElement("span");
      meta.className = "meta";
      meta.textContent = [h.e.k, h.e.t].concat(h.e.a || []).concat(h.e.m || []).join(" · ");
      li.appendChild(a);
      li.appendChild(meta);
      list.appendChild(li);
    });
  }

  box.value = new URL(location.href).searchParams.get("q") || "";
  box.addEventListener("input", show);
  var load = window.m2Search ? Promise.resolve(window.m2Search) : fetch(base + "api/search.json").then(function (r) { return r.json(); });
  load.then(function (data) {
    index = data;
    show();
  }).catch(function () { status.textContent = "The search index did not load."; });
  show();
})();
