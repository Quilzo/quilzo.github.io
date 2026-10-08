// The manual's one script, and nothing depends on it: without it the menu is
// a list above the guide, the commands are selectable text, and the search
// box is not shown. With it, the menu is a drawer on a phone, a command has a
// copy button, the appearance can be chosen, and the manual can be searched.
(function () {
  "use strict";
  var doc = document.documentElement;

  // -- appearance: follow the system, light, dark -------------------------
  var themes = ["", "light", "dark"];
  var labels = { "": "follow the system", light: "light", dark: "dark" };
  var toggle = document.querySelector(".theme-toggle");
  function current() { return doc.dataset.theme || ""; }
  function label() { toggle.setAttribute("aria-label", "Appearance: " + labels[current()]); toggle.title = toggle.getAttribute("aria-label"); }
  if (toggle) {
    toggle.hidden = false;
    label();
    toggle.addEventListener("click", function () {
      var next = themes[(themes.indexOf(current()) + 1) % themes.length];
      if (next) { doc.dataset.theme = next; } else { delete doc.dataset.theme; }
      try { if (next) { localStorage.setItem("qz-theme", next); } else { localStorage.removeItem("qz-theme"); } } catch (e) {}
      label();
    });
  }

  // -- the drawer, on a narrow window ---------------------------------------
  var menu = document.querySelector(".menu-toggle");
  var drawer = document.getElementById("drawer");
  function setDrawer(open) {
    doc.classList.toggle("drawer-open", open);
    menu.setAttribute("aria-expanded", String(open));
    menu.setAttribute("aria-label", open ? "Close the menu" : "Open the menu");
    if (open) {
      var here = drawer.querySelector("[aria-current]") || drawer.querySelector("a");
      if (here) { here.focus(); }
    }
  }
  if (menu && drawer) {
    menu.addEventListener("click", function () { setDrawer(!doc.classList.contains("drawer-open")); });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && doc.classList.contains("drawer-open")) { setDrawer(false); menu.focus(); }
    });
    document.addEventListener("click", function (e) {
      if (doc.classList.contains("drawer-open") && !drawer.contains(e.target) && !menu.contains(e.target)) { setDrawer(false); }
    });
  }

  // -- copy a command -------------------------------------------------------
  var copyIcon = '<svg class="icon" viewBox="0 -960 960 960" aria-hidden="true"><path d="M360-240q-33 0-56.5-23.5T280-320v-480q0-33 23.5-56.5T360-880h360q33 0 56.5 23.5T800-800v480q0 33-23.5 56.5T720-240H360Zm0-80h360v-480H360v480ZM200-80q-33 0-56.5-23.5T120-160v-520q0-17 11.5-28.5T160-720q17 0 28.5 11.5T200-680v520h400q17 0 28.5 11.5T640-120q0 17-11.5 28.5T600-80H200Zm160-240v-480 480Z"/></svg>';
  if (navigator.clipboard) {
    document.querySelectorAll("pre:not(.out)").forEach(function (pre) {
      var code = pre.querySelector("code");
      if (!code) { return; }
      var b = document.createElement("button");
      b.type = "button"; b.className = "copy"; b.innerHTML = copyIcon + "<span>Copy</span>";
      b.setAttribute("aria-label", "Copy this command");
      b.addEventListener("click", function () {
        // Comments are for the reader; what is copied is what a shell runs.
        var text = code.innerText.split("\n").filter(function (l) { return !/^\s*#/.test(l); }).join("\n").trim();
        navigator.clipboard.writeText(text).then(function () {
          b.classList.add("done"); b.querySelector("span").textContent = "Copied";
          setTimeout(function () { b.classList.remove("done"); b.querySelector("span").textContent = "Copy"; }, 1600);
        });
      });
      pre.appendChild(b);
    });
  }

  // -- which section is being read ------------------------------------------
  var tocLinks = Array.prototype.slice.call(document.querySelectorAll(".toc a"));
  if (tocLinks.length && "IntersectionObserver" in window) {
    var byId = {};
    tocLinks.forEach(function (a) { byId[a.hash.slice(1)] = a; });
    var visible = {};
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (en) { visible[en.target.id] = en.isIntersecting; });
      var first = null;
      document.querySelectorAll(".prose h2[id]").forEach(function (h) { if (!first && visible[h.id]) { first = h.id; } });
      if (first) { tocLinks.forEach(function (a) { a.setAttribute("aria-current", String(a === byId[first])); }); }
    }, { rootMargin: "-64px 0px -55% 0px" });
    document.querySelectorAll(".prose h2[id]").forEach(function (h) { io.observe(h); });
  }

  // -- search ---------------------------------------------------------------
  var box = document.querySelector(".search");
  var input = document.getElementById("q");
  var list = document.getElementById("results");
  if (!box || !input || !list || !window.fetch) { return; }
  box.hidden = false;
  var index = null, loading = null, active = -1, shown = [];

  function load() {
    if (index || loading) { return loading; }
    loading = fetch("/search.json").then(function (r) { return r.json(); }).then(function (d) { index = d; return d; });
    return loading;
  }
  function words(s) { return s.toLowerCase().split(/[^a-z0-9]+/).filter(function (w) { return w.length > 1; }); }
  function score(e, terms, phrase) {
    var t = (e.p + " " + (e.s || "")).toLowerCase(), x = e.x.toLowerCase(), s = 0;
    for (var i = 0; i < terms.length; i++) {
      var w = terms[i], inT = t.indexOf(w) >= 0, inX = x.indexOf(w) >= 0;
      if (!inT && !inX) { return 0; }
      s += (inT ? 10 : 0) + (inX ? 2 : 0);
      if (new RegExp("\\b" + w).test(t)) { s += 6; }
    }
    // The whole phrase in a title counts most, and more the earlier it
    // comes: a guide about it names it first.
    var at = t.indexOf(phrase);
    if (phrase.length > 3 && at >= 0) { s += 25 + Math.max(0, 20 - Math.floor(at / 4)); }
    else if (phrase.length > 3 && x.indexOf(phrase) >= 0) { s += 8; }
    if (!e.s) { s += 3; }
    return s;
  }
  function escape(s) { return s.replace(/[&<>"]/g, function (c) { return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]; }); }
  function mark(s, terms) {
    var out = escape(s);
    terms.forEach(function (w) { out = out.replace(new RegExp("(" + w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + ")", "gi"), "<mark>$1</mark>"); });
    return out;
  }
  function snippet(x, terms) {
    var lx = x.toLowerCase(), at = -1;
    for (var i = 0; i < terms.length && at < 0; i++) { at = lx.indexOf(terms[i]); }
    var start = Math.max(0, at - 60);
    return (start > 0 ? "…" : "") + x.slice(start, start + 180) + (x.length > start + 180 ? "…" : "");
  }
  function render() {
    var q = input.value.trim(), terms = words(q);
    active = -1;
    if (!terms.length) { close(); return; }
    shown = index.map(function (e) { return { e: e, s: score(e, terms, q.toLowerCase()) }; })
      .filter(function (r) { return r.s > 0; })
      .sort(function (a, b) { return b.s - a.s; }).slice(0, 8);
    list.innerHTML = shown.length ? shown.map(function (r, i) {
      var e = r.e;
      return '<li role="option" id="r' + i + '" aria-selected="false"><a href="' + escape(e.u) + '">' +
        '<span class="r-title">' + mark(e.s || e.p, terms) + "</span>" +
        (e.s ? '<span class="r-where">' + escape(e.p) + "</span>" : "") +
        '<span class="r-text">' + mark(snippet(e.x, terms), terms) + "</span></a></li>";
    }).join("") : '<li class="r-none" role="option" aria-disabled="true">Nothing in the manual matches “' + escape(q) + "”.</li>";
    list.hidden = false;
    input.setAttribute("aria-expanded", "true");
  }
  function close() { list.hidden = true; input.setAttribute("aria-expanded", "false"); input.removeAttribute("aria-activedescendant"); }
  function move(d) {
    if (!shown.length) { return; }
    active = (active + d + shown.length) % shown.length;
    Array.prototype.forEach.call(list.children, function (li, i) { li.setAttribute("aria-selected", String(i === active)); });
    input.setAttribute("aria-activedescendant", "r" + active);
    list.children[active].scrollIntoView({ block: "nearest" });
  }
  input.addEventListener("focus", load);
  input.addEventListener("input", function () { load().then(render); });
  input.addEventListener("keydown", function (e) {
    if (e.key === "ArrowDown") { e.preventDefault(); move(1); }
    else if (e.key === "ArrowUp") { e.preventDefault(); move(-1); }
    else if (e.key === "Enter") {
      var pick = active >= 0 ? shown[active] : shown[0];
      if (pick) { e.preventDefault(); location.href = pick.e.u; }
    } else if (e.key === "Escape") { close(); }
  });
  document.addEventListener("click", function (e) { if (!box.contains(e.target)) { close(); } });
  document.addEventListener("keydown", function (e) {
    var t = e.target, typing = t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable);
    if (!typing && (e.key === "/" || ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k"))) {
      e.preventDefault(); input.focus(); input.select();
    }
  });
})();
