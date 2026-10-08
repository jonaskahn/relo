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

  /* ---------- Intro ---------- */

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
      window.setTimeout(function () { span.style.opacity = "1"; }, 250 + index * 140);
    });

    window.setTimeout(function () {
      preloader.classList.add("out");
      root.classList.add("is-ready");
    }, 250 + letters.length * 140 + 450);

    window.setTimeout(function () {
      root.classList.add("is-in");
    }, 250 + letters.length * 140 + 450 + 700);

    window.setTimeout(finishIntro, 250 + letters.length * 140 + 450 + 2200);
  } else {
    finishIntro();
  }

  /* ---------- Nav colour over dark sections ---------- */

  var darkNodes = document.querySelectorAll("[data-dark]");

  if ("IntersectionObserver" in window && darkNodes.length) {
    var overlapping = new Set();
    var navObserver = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (entry.isIntersecting) overlapping.add(entry.target);
          else overlapping.delete(entry.target);
        });
        body.dataset.nav = overlapping.size ? "dark" : "light";
      },
      { rootMargin: "0px 0px -94% 0px" }
    );
    darkNodes.forEach(function (node) { navObserver.observe(node); });
  }

  /* ---------- Menu ---------- */

  var navToggle = document.getElementById("navToggle");
  var menu = document.getElementById("menu");
  var background = document.querySelectorAll("main, footer");
  setMenuLabel();

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
  }

  if (navToggle && menu) {
    navToggle.addEventListener("click", function () {
      setMenu(navToggle.getAttribute("aria-expanded") !== "true");
    });

    menu.querySelectorAll("a").forEach(function (link) {
      link.addEventListener("click", function () { setMenu(false); });
    });

    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && menu.classList.contains("open")) {
        setMenu(false);
        navToggle.focus();
      }
    });
  }

  /* ---------- Count-up ---------- */

  var stats = document.querySelectorAll(".stat-num[data-end]");

  function countUp(node) {
    var end = Number(node.dataset.end);
    var start = null;
    var duration = 2000;

    function frame(now) {
      if (start === null) start = now;
      var t = Math.min((now - start) / duration, 1);
      node.textContent = String(Math.round(end * (1 - Math.pow(1 - t, 3))));
      if (t < 1) window.requestAnimationFrame(frame);
    }

    node.textContent = "0";
    window.requestAnimationFrame(frame);
  }

  if (!reduced && "IntersectionObserver" in window && stats.length) {
    var statObserver = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) return;
          statObserver.unobserve(entry.target);
          countUp(entry.target);
        });
      },
      { threshold: 0.6 }
    );
    stats.forEach(function (node) { statObserver.observe(node); });
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
})();
