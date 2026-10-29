// The scripts of Math 2.0: the colour theme, copy buttons, list filters, the feeds and search. Each page can be read with no script. The build renders the math.
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

  // Times in a feed say how long ago, for example "3 days ago". The date stays as the title.
  var rtf = window.Intl && Intl.RelativeTimeFormat ? new Intl.RelativeTimeFormat("en", { numeric: "auto" }) : null;
  function ago(scope) {
    if (!rtf) return;
    scope.querySelectorAll("time[datetime]").forEach(function (t) {
      var d = (new Date(t.getAttribute("datetime")) - Date.now()) / 1000;
      if (isNaN(d)) return;
      var steps = [[60, "second"], [60, "minute"], [24, "hour"], [7, "day"], [4.35, "week"], [12, "month"], [Infinity, "year"]];
      for (var i = 0; i < steps.length; i++) {
        if (Math.abs(d) < steps[i][0]) {
          if (!t.title) t.title = t.textContent;
          t.textContent = rtf.format(Math.round(d), steps[i][1]);
          return;
        }
        d /= steps[i][0];
      }
    });
  }
  ago(document);

  // Latest and Discover load the next page when the reader comes near the end, so the feed goes on. The sentinel is the element after the last item: the pager of Latest or the last card of Discover. The links to the pages stay for a reader with no script.
  function endless(feed, sentinel, item) {
    var next = document.querySelector("[data-next]");
    if (!feed || !sentinel || !next || !window.IntersectionObserver) return;
    var busy = false;
    var watch = new IntersectionObserver(function (es) {
      if (busy || !next || !es.some(function (e) { return e.isIntersecting; })) return;
      busy = true;
      fetch(next.href).then(function (r) { return r.text(); }).then(function (html) {
        var doc = new DOMParser().parseFromString(html, "text/html");
        var more = doc.querySelector(feed.matches("[data-deck]") ? "[data-deck]" : "[data-feed]");
        var end = feed.querySelector(".slide-end");
        if (more) {
          more.querySelectorAll(item).forEach(function (el) {
            // A day of Latest can start on one page and go on to the next.
            var last = feed.lastElementChild;
            var day = el.querySelector(".day");
            if (day && last && last.querySelector(".day") && last.querySelector(".day").textContent === day.textContent) {
              el.querySelectorAll(".post").forEach(function (p) { last.appendChild(p); });
              return;
            }
            if (end) feed.insertBefore(el, end); else feed.appendChild(el);
          });
          ago(feed);
        }
        var n = doc.querySelector("[data-next]");
        if (n) {
          next.href = n.href;
        } else {
          next = null;
          watch.disconnect();
          var newEnd = doc.querySelector(".slide-end");
          if (end && newEnd) end.replaceWith(newEnd);
          var pager = document.querySelector(".pager");
          if (pager) pager.hidden = true;
        }
        busy = false;
        // Observe the sentinel again, so that it loads one more page when it is still on the screen.
        if (next) { watch.unobserve(sentinel); watch.observe(sentinel); }
      }).catch(function () { busy = false; });
    }, { rootMargin: "0px 0px 600px 0px" });
    watch.observe(sentinel);
  }
  var feed = document.querySelector("[data-feed]");
  if (feed) endless(feed, document.querySelector(".pager"), ".feed-day");
  var deck = document.querySelector("[data-deck]");
  if (deck) {
    endless(deck, deck.querySelector(".slide-end"), ".slide:not(.slide-end)");
    // The keys ↓, j and space go to the next card, ↑ and k to the one before.
    document.addEventListener("keydown", function (e) {
      if (e.target.closest && e.target.closest("input, textarea")) return;
      var step = { ArrowDown: 1, j: 1, PageDown: 1, ArrowUp: -1, k: -1, PageUp: -1 }[e.key];
      if (!step) return;
      e.preventDefault();
      var slides = deck.querySelectorAll(".slide");
      var y = window.scrollY + 2;
      var at = 0;
      slides.forEach(function (s, i) { if (s.offsetTop <= y) at = i; });
      var to = slides[Math.min(slides.length - 1, Math.max(0, at + step))];
      if (to) to.scrollIntoView({ behavior: "smooth" });
    });
  }

  // Share a card with the share sheet of the device, or else copy its link.
  document.addEventListener("click", function (e) {
    var b = e.target.closest && e.target.closest("[data-share]");
    if (!b) return;
    var url = new URL(b.dataset.share, location.href).href;
    if (navigator.share) {
      navigator.share({ title: b.dataset.title, url: url }).catch(function () { /* the reader closed the sheet */ });
    } else if (navigator.clipboard) {
      navigator.clipboard.writeText(url).then(function () {
        var old = b.textContent;
        b.textContent = "Link copied";
        setTimeout(function () { b.textContent = old; }, 1500);
      });
    }
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
    if (!q) { status.textContent = "Type to search the ideas."; return; }
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
      var kind = h.e.k ? h.e.k.charAt(0).toUpperCase() + h.e.k.slice(1) : "";
      meta.textContent = [kind].concat((h.e.a || []).slice(0, 3)).filter(Boolean).join(" · ");
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
