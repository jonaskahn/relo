(function () {
  var root = document.documentElement;
  var body = document.body;
  var reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  /* ---------- Theme ---------- */

  var themeToggle = document.querySelector("[data-theme-toggle]");
  var themeLabel = document.querySelector("[data-theme-label]");

  function currentTheme() {
    return root.dataset.theme === "dark" ? "dark" : "light";
  }

  function applyTheme(theme) {
    root.dataset.theme = theme;
    if (!themeToggle || !themeLabel) return;
    var dark = theme === "dark";
    themeToggle.setAttribute("aria-pressed", dark ? "true" : "false");
    themeLabel.textContent = relo.t(dark ? "light" : "dark");
  }

  applyTheme(currentTheme());
  document.addEventListener("relo:lang", function () {
    applyTheme(currentTheme());
    setMenuLabel();
  });

  if (themeToggle) {
    themeToggle.addEventListener("click", function () {
      var next = currentTheme() === "dark" ? "light" : "dark";
      try { localStorage.setItem("relo-theme", next); } catch (e) {}
      applyTheme(next);
    });
  }

  /* ---------- Intro: typed "Relo." ---------- */

  function finishIntro() {
    root.classList.add("is-ready", "is-in", "intro-done");
    try { sessionStorage.setItem("relo-intro", "1"); } catch (e) {}
  }

  var preWord = document.getElementById("preWord");
  var preloader = document.getElementById("preloader");
  var introRuns = preWord && preloader &&
    !root.classList.contains("skip-intro") && !root.classList.contains("intro-done");

  if (introRuns) {
    var letters = ["R", "e", "l", "o", "."];
    var spans = letters.map(function (letter, index) {
      var span = document.createElement("span");
      span.className = index === letters.length - 1 ? "l dot" : "l";
      span.textContent = letter;
      preWord.appendChild(span);
      return span;
    });
    var cursor = document.createElement("span");
    cursor.className = "pre-cursor";
    preWord.appendChild(cursor);

    spans.forEach(function (span, index) {
      window.setTimeout(function () { span.style.opacity = "1"; }, 250 + index * 120);
    });

    window.setTimeout(function () {
      preloader.classList.add("out");
      root.classList.add("is-ready");
    }, 250 + letters.length * 120 + 420);

    window.setTimeout(function () {
      preloader.remove();
    }, 250 + letters.length * 120 + 420 + 600);

    window.setTimeout(finishIntro, 250 + letters.length * 120 + 420 + 560);
  } else {
    finishIntro();
  }

  /* ---------- Menu: left drawer ---------- */

  var navToggle = document.getElementById("navToggle");
  var menu = document.getElementById("menu");
  var background = document.querySelectorAll("main, footer");
  setMenuLabel();

  function menuLinks() {
    return menu ? Array.prototype.slice.call(menu.querySelectorAll("a")) : [];
  }

  function setMenuLabel() {
    if (!navToggle) return;
    var open = navToggle.getAttribute("aria-expanded") === "true";
    navToggle.setAttribute("aria-label", relo.t(open ? "close" : "menu"));
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

    // Focus stays in the drawer while it is open.
    document.addEventListener("focusin", function (event) {
      if (!menu.classList.contains("open")) return;
      if (menu.contains(event.target)) return;
      var first = menuLinks()[0];
      if (first) first.focus({ preventScroll: true });
    });
  }

  /* ---------- Copy ---------- */

  function flash(button, ok) {
    var label = relo.t(ok ? "copied" : "failed");
    button.dataset.copied = ok ? "true" : "false";
    button.setAttribute("aria-label", label);
    button.title = label;
    window.setTimeout(function () {
      delete button.dataset.copied;
      button.setAttribute("aria-label", relo.t("copy"));
      button.title = relo.t("copy");
    }, 1600);
  }

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

  /* ---------- Scroll cue: starts the story ---------- */

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
})();
