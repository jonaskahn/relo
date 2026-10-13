/* Relo site interactions. Everything here is progressive: without JavaScript
   the page still reads top to bottom and every command is selectable text. */
(function () {
  var root = document.documentElement;
  var body = document.body;
  var reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  function t(key) {
    return window.relo && window.relo.t ? window.relo.t(key) : key;
  }

  /* ---------- Theme: a circular reveal out of the button that changed it ---------- */

  var themeToggle = document.querySelector("[data-theme-toggle]");

  function currentTheme() {
    return root.dataset.theme === "dark" ? "dark" : "light";
  }

  function applyTheme(theme) {
    root.dataset.theme = theme;
    var dark = theme === "dark";
    if (themeToggle) {
      themeToggle.setAttribute("aria-pressed", dark ? "true" : "false");
      themeToggle.title = t(dark ? "light" : "dark");
    }
  }

  function setTheme(theme, origin) {
    /* The View Transitions API repaints the page inside a growing circle
       centred on the toggle, so the new theme arrives from the button rather
       than replacing the screen at once. */
    var supported = typeof document.startViewTransition === "function" && !reduced && origin;
    if (!supported) {
      applyTheme(theme);
      return;
    }
    var cx = origin.x;
    var cy = origin.y;
    var reach = Math.hypot(Math.max(cx, window.innerWidth - cx), Math.max(cy, window.innerHeight - cy));
    var swap = document.startViewTransition(function () { applyTheme(theme); });
    swap.ready.then(function () {
      root.animate(
        { clipPath: ["circle(0px at " + cx + "px " + cy + "px)", "circle(" + reach + "px at " + cx + "px " + cy + "px)"] },
        { duration: 620, easing: "cubic-bezier(0.16, 1, 0.3, 1)", pseudoElement: "::view-transition-new(root)" }
      );
    }).catch(function () {});
  }

  applyTheme(currentTheme());

  if (themeToggle) {
    themeToggle.addEventListener("click", function (event) {
      var next = currentTheme() === "dark" ? "light" : "dark";
      var rect = themeToggle.getBoundingClientRect();
      try { localStorage.setItem("relo-theme", next); } catch (e) {}
      setTheme(next, reduced ? null : { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 });
      if (event.detail > 0) themeToggle.blur();
    });
  }

  /* ---------- Menu: the drawer ---------- */

  var navToggle = document.getElementById("navToggle");
  var menu = document.getElementById("menu");
  var background = document.querySelectorAll("main, footer");

  function menuLinks() {
    return menu ? Array.prototype.slice.call(menu.querySelectorAll("a")) : [];
  }

  function setMenuLabel() {
    if (!navToggle) return;
    var open = navToggle.getAttribute("aria-expanded") === "true";
    navToggle.setAttribute("aria-label", t(open ? "close" : "menu"));
  }

  function setMenu(open) {
    if (!navToggle || !menu) return;
    navToggle.setAttribute("aria-expanded", open ? "true" : "false");
    setMenuLabel();
    menu.classList.toggle("open", open);
    menu.inert = !open;
    body.classList.toggle("menu-open", open);
    body.style.overflow = open ? "hidden" : "";
    background.forEach(function (node) { node.inert = open; });
    if (open) {
      var first = menuLinks()[0];
      if (first) first.focus({ preventScroll: true });
    } else {
      navToggle.focus({ preventScroll: true });
    }
  }

  if (navToggle && menu) {
    navToggle.addEventListener("click", function () {
      setMenu(navToggle.getAttribute("aria-expanded") !== "true");
    });
    menuLinks().forEach(function (link) {
      link.addEventListener("click", function () { setMenu(false); });
    });
    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && menu.classList.contains("open")) {
        setMenu(false);
        navToggle.focus();
      }
    });
    document.addEventListener("focusin", function (event) {
      if (!menu.classList.contains("open")) return;
      if (menu.contains(event.target)) return;
      var first = menuLinks()[0];
      if (first) first.focus({ preventScroll: true });
    });
  }

  /* ---------- Language: a glass popover over the real select ---------- */

  var select = document.getElementById("lang");
  var langBtn = document.getElementById("langBtn");
  var langPop = document.getElementById("langPop");
  var langLabel = document.getElementById("langLabel");

  function buildLang() {
    if (!select || !langPop) return;
    langPop.innerHTML = "";
    Array.prototype.slice.call(select.options).forEach(function (option) {
      var item = document.createElement("button");
      item.type = "button";
      item.className = "lang-item";
      item.setAttribute("role", "option");
      item.dataset.code = option.value;
      item.lang = option.lang || option.value;
      item.textContent = option.textContent;
      item.style.setProperty("--i", langPop.children.length);
      if (option.value === select.value) item.setAttribute("aria-selected", "true");
      item.addEventListener("click", function () {
        select.value = option.value;
        select.dispatchEvent(new Event("change"));
        closeLang();
        if (langBtn) langBtn.focus({ preventScroll: true });
      });
      langPop.appendChild(item);
    });
    syncLangLabel();
  }

  function syncLangLabel() {
    if (!langLabel || !select) return;
    var option = select.options[select.selectedIndex];
    if (!option) return;
    langLabel.textContent = option.textContent;
    langLabel.lang = option.lang || option.value;
  }

  function openLang() {
    if (!langPop || !langBtn) return;
    langPop.hidden = false;
    langPop.classList.add("is-open");
    langBtn.setAttribute("aria-expanded", "true");
    var active = langPop.querySelector('[aria-selected="true"]') || langPop.firstElementChild;
    if (active) active.focus({ preventScroll: true });
  }

  function closeLang() {
    if (!langPop || !langBtn) return;
    langPop.classList.remove("is-open");
    langBtn.setAttribute("aria-expanded", "false");
    window.setTimeout(function () { langPop.hidden = true; }, reduced ? 0 : 180);
  }

  if (langBtn && langPop) {
    langBtn.addEventListener("click", function () {
      if (langPop.hidden) openLang();
      else closeLang();
    });
    document.addEventListener("click", function (event) {
      if (langPop.hidden) return;
      if (langPop.contains(event.target) || langBtn.contains(event.target)) return;
      closeLang();
    });
    langPop.addEventListener("keydown", function (event) {
      var items = Array.prototype.slice.call(langPop.children);
      var index = items.indexOf(document.activeElement);
      if (event.key === "ArrowDown") {
        event.preventDefault();
        (items[index + 1] || items[0]).focus();
      } else if (event.key === "ArrowUp") {
        event.preventDefault();
        (items[index - 1] || items[items.length - 1]).focus();
      } else if (event.key === "Escape") {
        closeLang();
        langBtn.focus({ preventScroll: true });
      }
    });
  }

  /* ---------- Kinetic type ---------- */

  function splitLines(node, mode) {
    /* Re-splitting on every language change is the point: the animation is a
       property of the text, not of the markup. When the dictionary has just
       replaced the text, that new text is what we split. */
    var current = node.textContent;
    var untouched = node.dataset.split === "done" && current === node.dataset.rendered;
    var text = untouched ? node.dataset.raw : current;
    node.dataset.raw = text;
    if (!text) return;
    var parts = mode === "word" ? text.split(/(\s+)/) : text.split("");
    node.textContent = "";
    var index = 0;
    parts.forEach(function (part) {
      if (/^\s+$/.test(part)) {
        node.appendChild(document.createTextNode(part));
        return;
      }
      var span = document.createElement("span");
      span.className = mode === "word" ? "w" : "c";
      span.textContent = part;
      span.style.setProperty("--i", index++);
      node.appendChild(span);
    });
    node.dataset.rendered = node.textContent;
    node.dataset.split = "done";
  }

  function applyKinetic() {
    document.querySelectorAll("[data-kinetic]").forEach(function (node) {
      splitLines(node, "char");
    });
    document.querySelectorAll("[data-words]").forEach(function (node) {
      splitLines(node, "word");
    });
  }

  /* ---------- Section reveals ---------- */

  function reveal() {
    var targets = document.querySelectorAll(
      ".section-title, .section-lead, .display, .channels, .terminal, .logo-wall li, .chips li, .stats li, .pick-one, .foot-mark"
    );
    if (!("IntersectionObserver" in window) || reduced) {
      targets.forEach(function (node) { node.classList.add("is-in"); });
      return;
    }
    var watcher = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        entry.target.classList.add("is-in");
        watcher.unobserve(entry.target);
      });
      /* Any intersection counts, and a tall element must not have to show a
         fraction of itself before it is allowed to appear. */
    }, { rootMargin: "0px 0px -8% 0px", threshold: 0 });
    targets.forEach(function (node, index) {
      node.style.setProperty("--stagger", Math.min(index % 8, 7));
      watcher.observe(node);
    });
  }

  /* ---------- The ticker: what Relo does, in one line ---------- */

  var ticker = document.getElementById("ticker");
  var TRIPS = [
    "claude-code  →  relo  →  anthropic     212ms",
    "codex        →  relo  →  openai        180ms",
    "cursor       →  relo  →  google        236ms",
    "opencode     →  relo  →  deepseek      174ms",
    "cline        →  relo  →  groq           98ms",
    "raycast      →  relo  →  mistral       205ms"
  ];
  var tripIndex = 0;

  function nextTrip() {
    if (!ticker) return;
    ticker.textContent = TRIPS[tripIndex % TRIPS.length];
    ticker.classList.remove("is-in");
    void ticker.offsetWidth;
    ticker.classList.add("is-in");
    tripIndex++;
  }

  /* ---------- Copy ---------- */

  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text);
    }
    return new Promise(function (resolve, reject) {
      var area = document.createElement("textarea");
      area.value = text;
      area.setAttribute("readonly", "");
      area.style.position = "fixed";
      area.style.left = "-999px";
      document.body.appendChild(area);
      area.select();
      try {
        if (document.execCommand("copy")) resolve();
        else reject(new Error("copy failed"));
      } catch (error) {
        reject(error);
      } finally {
        area.remove();
      }
    });
  }

  function flash(button, ok) {
    var label = t(ok ? "copied" : "failed");
    button.dataset.copied = ok ? "true" : "false";
    button.setAttribute("aria-label", label);
    button.title = label;
    var block = button.closest(".command");
    if (block && ok) block.classList.add("is-copied");
    window.setTimeout(function () {
      delete button.dataset.copied;
      button.setAttribute("aria-label", t("copy"));
      button.title = t("copy");
      if (block) block.classList.remove("is-copied");
    }, 1600);
  }

  document.querySelectorAll("[data-copy-btn]").forEach(function (button) {
    button.addEventListener("click", function () {
      var block = button.closest(".command");
      var code = block && block.querySelector("[data-copy]");
      if (!code) return;
      copyText(code.textContent).then(function () {
        flash(button, true);
      }).catch(function () {
        flash(button, false);
      });
    });
  });

  /* ---------- The nav: condense, and show how far down the page you are ---------- */

  var nav = document.getElementById("nav");
  var progress = document.getElementById("pageProgress");
  var ticking = false;

  function onScroll() {
    if (nav) nav.classList.toggle("is-condensed", window.scrollY > 24);
    if (progress) {
      var max = Math.max(1, document.documentElement.scrollHeight - window.innerHeight);
      progress.style.setProperty("--p", Math.min(1, window.scrollY / max).toFixed(4));
    }
    ticking = false;
  }

  window.addEventListener("scroll", function () {
    if (ticking) return;
    ticking = true;
    window.requestAnimationFrame(onScroll);
  }, { passive: true });

  /* ---------- The install section: tabs, and a terminal that shows the work ---------- */

  var tabs = Array.prototype.slice.call(document.querySelectorAll('.os-tabs [role="tab"]'));
  var lens = document.getElementById("osLens");

  function moveLens(tab, animate) {
    if (!lens || !tab) return;
    var rect = tab.getBoundingClientRect();
    var parent = tab.parentElement.getBoundingClientRect();
    lens.style.setProperty("--x", (rect.left - parent.left) + "px");
    lens.style.setProperty("--w", rect.width + "px");
    lens.classList.toggle("no-anim", !animate);
  }

  function selectOs(name, animate, focus) {
    tabs.forEach(function (tab) {
      var on = tab.dataset.os === name;
      tab.setAttribute("aria-selected", on ? "true" : "false");
      tab.tabIndex = on ? 0 : -1;
      if (on && animate) {
        moveLens(tab, true);
        if (focus) tab.focus({ preventScroll: true });
      }
    });
    var panels = document.querySelectorAll(".os-panel");
    panels.forEach(function (panel) {
      var on = panel.id === "panel-" + name;
      if (on) {
        panel.hidden = false;
        panel.classList.remove("is-in");
        void panel.offsetWidth;
        panel.classList.add("is-in");
      } else {
        panel.hidden = true;
      }
    });
    if (name === "windows") replayScript = "ps1";
    else replayScript = "sh";
    startReplay(true);
  }

  document.querySelectorAll(".os-panel").forEach(function (panel) {
    if (!panel.hidden) panel.classList.add("is-in");
  });

  tabs.forEach(function (tab) {
    tab.addEventListener("click", function () { selectOs(tab.dataset.os, true, false); });
    tab.addEventListener("keydown", function (event) {
      var index = tabs.indexOf(tab);
      var next = null;
      if (event.key === "ArrowRight") next = tabs[(index + 1) % tabs.length];
      else if (event.key === "ArrowLeft") next = tabs[(index - 1 + tabs.length) % tabs.length];
      else if (event.key === "Home") next = tabs[0];
      else if (event.key === "End") next = tabs[tabs.length - 1];
      if (!next) return;
      event.preventDefault();
      selectOs(next.dataset.os, true, true);
    });
  });

  /* ---------- The terminal transcript ---------- */

  var termBody = document.getElementById("termBody");
  var termTitle = document.getElementById("termTitle");
  var termReplay = document.getElementById("termReplay");
  var replayScript = "sh";
  var replayTimer = 0;
  var replayRun = 0;

  function bar(pct) {
    var filled = Math.round(pct / 4);
    var out = "";
    for (var i = 0; i < 25; i++) out += i < filled ? "█" : "░";
    return out;
  }

  function transcript(kind) {
    if (kind === "ps1") {
      return [
        { kind: "cmd", text: "PS> irm https://relo.ifelse.on/install.ps1 | iex" },
        { kind: "blank" },
        { kind: "out", text: "  Relo.  one daemon · many tools · every provider", cls: "accent" },
        { kind: "out", text: "  Relo installer · windows/amd64", cls: "dim" },
        { kind: "out", text: "  ✓ No other Relo installation is in the way", cls: "ok" },
        { kind: "out", text: "  · Release 0.1.0", cls: "dim" },
        { kind: "bar", label: "relo-windows-amd64.exe" },
        { kind: "out", text: "  ✓ Checksum verified", cls: "ok" },
        { kind: "out", text: "  ▸ Checking the ports Relo serves" },
        { kind: "out", text: "  ✓ 10101 freed   · 10201 free   · 10202 free", cls: "ok" },
        { kind: "out", text: "  ✓ Added %LOCALAPPDATA%\\Relo\\bin to PATH", cls: "ok" },
        { kind: "out", text: "  ✓ Installed relo v0.1.0", cls: "ok" },
        { kind: "out", text: "  ✓ Console is up on http://127.0.0.1:10101", cls: "ok" }
      ];
    }
    return [
      { kind: "cmd", text: "curl -fsSL https://relo.ifelse.on/install.sh | sh" },
      { kind: "blank" },
      { kind: "out", text: "  Relo.  one daemon · many tools · every provider", cls: "accent" },
      { kind: "out", text: "  Relo installer · darwin/arm64", cls: "dim" },
      { kind: "out", text: "  ✓ No other Relo installation is in the way", cls: "ok" },
      { kind: "out", text: "  · Release 0.1.0", cls: "dim" },
      { kind: "bar", label: "relo-darwin-arm64" },
      { kind: "out", text: "  ✓ Checksum verified", cls: "ok" },
      { kind: "out", text: "  ▸ Stopping the running daemon" },
      { kind: "out", text: "  ▸ Checking the ports Relo serves" },
      { kind: "out", text: "  · 10101  free", cls: "dim" },
      { kind: "out", text: "  ✓ 10201  freed   · 10202  free", cls: "ok" },
      { kind: "out", text: "  ✓ Installed relo v0.1.0", cls: "ok" },
      { kind: "out", text: "  ▸ Starting the daemon" },
      { kind: "out", text: "  ✓ Console is up on http://127.0.0.1:10101", cls: "ok" }
    ];
  }

  function line(text, cls) {
    var node = document.createElement("span");
    node.className = "term-line" + (cls ? " " + cls : "");
    node.textContent = text;
    termBody.appendChild(node);
    termBody.scrollTop = termBody.scrollHeight;
    return node;
  }

  function startReplay(force) {
    if (!termBody) return;
    window.clearTimeout(replayTimer);
    replayRun++;
    var run = replayRun;
    termBody.innerHTML = "";
    if (termTitle) {
      termTitle.textContent = replayScript === "ps1"
        ? "powershell — relo.ifelse.on/install.ps1"
        : "zsh — relo.ifelse.on/install.sh";
    }

    var steps = transcript(replayScript);
    if (reduced || force === "static") {
      steps.forEach(function (step) {
        if (step.kind === "bar") line("  ▸ Downloading " + step.label + " " + bar(100) + " 100%  7.9/7.9 MB", "ok");
        else if (step.kind === "blank") line("");
        else line(step.text, step.cls);
      });
      return;
    }

    var index = 0;
    function advance() {
      if (run !== replayRun) return;
      if (index >= steps.length) {
        /* A slow blink at the end says "this is a terminal", not "this is a
           screenshot". */
        var cursor = line("  █", "accent");
        replayTimer = window.setTimeout(function () {
          if (run !== replayRun) return;
          cursor.className = "term-line accent is-blink";
        }, 400);
        return;
      }
      var step = steps[index++];
      if (step.kind === "blank") {
        line("");
        replayTimer = window.setTimeout(advance, 120);
      } else if (step.kind === "cmd") {
        var node = line("", "cmd");
        var chars = step.text.split("");
        var at = 0;
        (function type() {
          if (run !== replayRun) return;
          node.textContent += chars[at++];
          if (at < chars.length) replayTimer = window.setTimeout(type, 22);
          else replayTimer = window.setTimeout(advance, 340);
        })();
      } else if (step.kind === "bar") {
        var growing = line("", "");
        var pct = 0;
        (function fill() {
          if (run !== replayRun) return;
          pct += 4 + Math.random() * 7;
          if (pct >= 100) {
            growing.className = "term-line ok";
            growing.textContent = "  ✓ " + step.label + " " + bar(100) + " 100%  7.9/7.9 MB";
            replayTimer = window.setTimeout(advance, 260);
            return;
          }
          growing.textContent = "  ▸ " + step.label + " " + bar(pct) + " " +
            String(Math.round(pct)).padStart(3, " ") + "%  " +
            (pct * 7.9 / 100).toFixed(1) + "/7.9 MB  " + (2.4 + Math.random() * 2).toFixed(1) + " MB/s";
          replayTimer = window.setTimeout(fill, 90);
        })();
      } else {
        line(step.text, step.cls);
        replayTimer = window.setTimeout(advance, step.cls === "accent" ? 260 : 170);
      }
    }
    advance();
  }

  if (termReplay) {
    termReplay.addEventListener("click", function () {
      startReplay(true);
    });
  }

  /* The transcript runs when it is looked at, and stops when it is not. */
  var terminal = document.getElementById("terminal");
  if (terminal && "IntersectionObserver" in window) {
    var termWatcher = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) startReplay();
        else window.clearTimeout(replayTimer);
      });
    }, { threshold: 0.35 });
    termWatcher.observe(terminal);
  } else {
    startReplay("static");
  }

  /* ---------- The scroll cue still starts the story ---------- */

  var cue = document.getElementById("scrollCue");
  if (cue) {
    cue.addEventListener("click", function () {
      var story = document.querySelector(".story-pin");
      if (!story) return;
      var top = story.getBoundingClientRect().top + window.scrollY;
      var range = story.offsetHeight - window.innerHeight;
      window.scrollTo({ top: top + range * 0.08, behavior: reduced ? "auto" : "smooth" });
    });
  }

  /* ---------- Boot ---------- */

  function guessOs() {
    var platform = "";
    try {
      if (navigator.userAgentData && navigator.userAgentData.platform) {
        platform = navigator.userAgentData.platform;
      }
    } catch (e) {}
    if (!platform) platform = navigator.platform || "";
    if (!platform) platform = navigator.userAgent || "";
    platform = platform.toLowerCase();
    if (platform.indexOf("win") >= 0) return "windows";
    if (platform.indexOf("mac") >= 0 || platform.indexOf("darwin") >= 0) return "macos";
    return "linux";
  }

  document.querySelectorAll(".os-panel").forEach(function (panel) {
    Array.prototype.slice.call(panel.children).forEach(function (child, index) {
      child.style.setProperty("--i", index);
    });
  });

  applyKinetic();
  reveal();
  nextTrip();
  buildLang();
  selectOs(guessOs(), true, false);

  document.addEventListener("relo:lang", function () {
    applyKinetic();
    syncLangLabel();
    applyTheme(currentTheme());
    setMenuLabel();
  });

  if (ticker && !reduced) window.setInterval(nextTrip, 2600);

  window.addEventListener("resize", function () {
    var active = document.querySelector('.os-tabs [aria-selected="true"]');
    if (active) moveLens(active, false);
  });

  window.setTimeout(function () {
    var active = document.querySelector('.os-tabs [aria-selected="true"]');
    if (active) moveLens(active, false);
  }, 400);
})();
