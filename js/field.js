/* The field: one canvas of drifting nodes behind the whole page, lit by
   pulses that travel the way a request does. It is atmosphere, not
   information, so it stays quiet: a few hundred dots, one accent, no
   interaction. Each section nudges the density, and the loop stops whenever
   the page is hidden or the visitor asked for less motion. */
(function () {
  var root = document.documentElement;
  var reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  /* Colours come from the design tokens, so the field follows the theme and
     the accent without knowing anything about them. */
  function token(name) {
    var value = getComputedStyle(root).getPropertyValue(name).trim();
    if (!value) return [255, 255, 255];
    if (value.charAt(0) === "#") {
      var hex = value.slice(1);
      if (hex.length === 3) hex = hex[0] + hex[0] + hex[1] + hex[1] + hex[2] + hex[2];
      return [parseInt(hex.slice(0, 2), 16), parseInt(hex.slice(2, 4), 16), parseInt(hex.slice(4, 6), 16)];
    }
    var parts = value.match(/[\d.]+/g) || [];
    return [Number(parts[0]) || 0, Number(parts[1]) || 0, Number(parts[2]) || 0];
  }

  var accent = token("--accent");
  var ink = token("--ink");

  function rgba(c, a) {
    return "rgba(" + c[0] + "," + c[1] + "," + c[2] + "," + a.toFixed(3) + ")";
  }

  var canvas = document.getElementById("field");
  if (!canvas) return;
  var ctx = canvas.getContext("2d");
  if (!ctx) return;

  var W = 0, H = 0, dpr = 1;
  var spacing = 64;
  var dots = [];
  var pulses = [];
  var scroll = 0;
  var mode = "hero";
  var clock = 0;
  var running = false;

  /* Each section asks for a density and a tempo, nothing more. */
  var MODES = {
    hero: { spacing: 64, alpha: 0.5, speed: 0.05, perRow: 1 },
    story: { spacing: 54, alpha: 0.66, speed: 0.09, perRow: 2 },
    providers: { spacing: 60, alpha: 0.58, speed: 0.065, perRow: 1 },
    install: { spacing: 58, alpha: 0.6, speed: 0.075, perRow: 1 }
  };

  function layout() {
    dpr = Math.min(window.devicePixelRatio || 1, 2);
    W = window.innerWidth;
    H = window.innerHeight;
    canvas.width = Math.floor(W * dpr);
    canvas.height = Math.floor(H * dpr);
    canvas.style.width = W + "px";
    canvas.style.height = H + "px";
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    buildDots();
  }

  function buildDots() {
    var conf = MODES[mode] || MODES.hero;
    spacing = conf.spacing;
    var cols = Math.ceil(W / spacing) + 2;
    var rows = Math.ceil(H / spacing) + 2;
    dots = [];
    for (var r = 0; r < rows; r++) {
      for (var c = 0; c < cols; c++) {
        dots.push({
          row: r,
          /* Depth drives size and brightness, so the grid reads as a volume
             rather than a sheet. */
          z: 0.4 + ((r * 7 + c * 13) % 10) / 16,
          phase: ((r * 31 + c * 17) % 100) / 100 * Math.PI * 2,
          x: c * spacing,
          y: r * spacing
        });
      }
    }
    pulses = [];
    for (var i = 0; i < rows; i++) {
      for (var k = 0; k < conf.perRow; k++) {
        pulses.push({ row: i, x: -Math.random() * W, speed: 0.5 + Math.random() * 0.6 });
      }
    }
  }

  function respawn(pulse) {
    pulse.x = -Math.random() * W * 0.6 - 40;
    pulse.speed = 0.5 + Math.random() * 0.6;
  }

  function draw() {
    ctx.clearRect(0, 0, W, H);
    var conf = MODES[mode] || MODES.hero;

    for (var p = 0; p < pulses.length; p++) {
      pulses[p].x += pulses[p].speed * conf.speed * 60;
      if (pulses[p].x > W + 80) respawn(pulses[p]);
    }

    var pulseRows = {};
    for (var q = 0; q < pulses.length; q++) {
      var pulse = pulses[q];
      if (!pulseRows[pulse.row]) pulseRows[pulse.row] = [];
      pulseRows[pulse.row].push(pulse);
    }

    for (var i = 0; i < dots.length; i++) {
      var dot = dots[i];
      var drift = Math.sin(clock * 0.0005 + dot.phase) * 4;
      var x = dot.x + drift * 0.4;
      var y = dot.y - scroll * 26 * dot.z + drift;

      /* A pulse lights the dots it passes, and its wake fades behind it. */
      var lit = 0;
      var rowPulses = pulseRows[dot.row];
      if (rowPulses) {
        for (var s = 0; s < rowPulses.length; s++) {
          var gap = Math.abs(rowPulses[s].x - dot.x);
          if (gap < 150) lit += (1 - gap / 150) * 0.7;
        }
      }

      var alpha = (0.075 + lit * 0.42) * conf.alpha * dot.z;
      if (alpha <= 0.006) continue;

      var size = 1 + dot.z * 0.9 + lit * 1.1;
      ctx.fillStyle = lit > 0.3 ? rgba(accent, Math.min(0.75, alpha * 1.6)) : rgba(ink, alpha);
      ctx.fillRect(x, y, size, size);
    }
  }

  var raf = 0;
  var last = 0;

  function tick(now) {
    if (!running) return;
    clock += last ? Math.min(now - last, 48) : 16;
    last = now;
    draw();
    raf = window.requestAnimationFrame(tick);
  }

  function play() {
    if (running || reduced) return;
    running = true;
    last = 0;
    raf = window.requestAnimationFrame(tick);
  }

  function pause() {
    running = false;
    if (raf) window.cancelAnimationFrame(raf);
    raf = 0;
  }

  /* A still frame is still a field: without motion we draw once and stop. */
  function paintStill() {
    scroll = 0;
    draw();
  }

  layout();
  if (reduced) paintStill();
  else play();

  var relayout;
  window.addEventListener("resize", function () {
    window.clearTimeout(relayout);
    relayout = window.setTimeout(function () {
      layout();
      if (reduced) paintStill();
    }, 160);
  });

  document.addEventListener("visibilitychange", function () {
    if (document.hidden) pause();
    else if (!reduced) play();
  });

  window.addEventListener("scroll", function () {
    var max = Math.max(1, document.documentElement.scrollHeight - window.innerHeight);
    scroll = Math.min(1, window.scrollY / max);
  }, { passive: true });

  var sections = document.querySelectorAll("[data-field]");
  if ("IntersectionObserver" in window && sections.length) {
    var watcher = new IntersectionObserver(function (entries) {
      for (var i = 0; i < entries.length; i++) {
        if (!entries[i].isIntersecting) continue;
        var next = entries[i].target.getAttribute("data-field");
        if (next && next !== mode && MODES[next]) {
          mode = next;
          buildDots();
        }
      }
    }, { rootMargin: "-45% 0px -45% 0px" });
    for (var i = 0; i < sections.length; i++) watcher.observe(sections[i]);
  }

  /* The theme toggle repaints the tokens; the field follows them. */
  new MutationObserver(function () {
    accent = token("--accent");
    ink = token("--ink");
    if (reduced) paintStill();
  }).observe(root, { attributes: true, attributeFilter: ["data-theme"] });
})();
