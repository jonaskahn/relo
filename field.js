/* The signal field: one canvas of drifting nodes behind the whole page, lit
   by pulses that travel the way a request does -- out of a tool, through the
   core, into a provider. Every section changes the field's mood, and the
   first load assembles "Relo." out of the same particles.

   One requestAnimationFrame drives everything, it stops whenever the page is
   hidden or the user asked for less motion, and it only ever touches fillRect
   and globalAlpha. */
(function () {
  var root = document.documentElement;
  var reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var body = document.body;

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

  /* ------------------------------------------------------------ the field -- */

  var canvas = document.getElementById("field");
  if (!canvas) return;
  var ctx = canvas.getContext("2d");
  if (!ctx) return;

  var W = 0, H = 0, dpr = 1;
  var SPACING = 48;
  var dots = [];
  var pulses = [];
  var pointer = { x: -9999, y: -9999, active: false };
  var scroll = 0;
  var mode = "hero";
  var clock = 0;
  var running = false;

  /* Each section asks the field for a different temperament. */
  var MODES = {
    hero: { speed: 0.055, alpha: 0.5, tilt: 0, converge: 0, perRow: 2 },
    story: { speed: 0.11, alpha: 0.72, tilt: 0.09, converge: 0, perRow: 4 },
    providers: { speed: 0.07, alpha: 0.62, tilt: 0.03, converge: 0, perRow: 3, orbit: 1 },
    install: { speed: 0.09, alpha: 0.66, tilt: 0.02, converge: 1, perRow: 3 },
    quiet: { speed: 0.03, alpha: 0.34, tilt: 0, converge: 0, perRow: 1 }
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
    /* Fewer nodes on small or busy machines: the field is atmosphere, not
       information, and it must never cost a frame of the story. */
    var heavy = W * H > 1600000 && (navigator.hardwareConcurrency || 4) >= 6;
    var step = heavy ? SPACING : SPACING * 1.3;
    var cols = Math.ceil(W / step) + 2;
    var rows = Math.ceil(H / step) + 2;
    dots = [];
    for (var r = 0; r < rows; r++) {
      for (var c = 0; c < cols; c++) {
        dots.push({
          row: r,
          col: c,
          /* Depth drives size, brightness and parallax, so the grid reads as
             a volume rather than a sheet. */
          z: 0.35 + ((r * 7 + c * 13) % 10) / 14,
          phase: ((r * 31 + c * 17) % 100) / 100 * Math.PI * 2,
          x: c * step,
          y: r * step,
          px: 0,
          py: 0,
          lit: 0
        });
      }
    }
    pulses = [];
    var rowsCount = rows;
    for (var i = 0; i < rowsCount; i++) {
      for (var k = 0; k < MODES[mode].perRow; k++) {
        pulses.push({
          row: i,
          x: -Math.random() * W,
          speed: 0.55 + Math.random() * 0.7,
          strength: 0.5 + Math.random() * 0.5
        });
      }
    }
  }

  function respawn(pulse) {
    pulse.x = -Math.random() * W * 0.6 - 40;
    pulse.speed = 0.55 + Math.random() * 0.7;
    pulse.strength = 0.5 + Math.random() * 0.5;
  }

  function draw() {
    ctx.clearRect(0, 0, W, H);

    var conf = MODES[mode] || MODES.hero;
    var tilt = conf.tilt + scroll * 0.06;
    var convergence = conf.converge;
    var coreX = W * 0.5;
    var coreY = H * 0.5;

    /* Advance the pulses first: they are what the dots respond to. */
    for (var p = 0; p < pulses.length; p++) {
      pulses[p].x += pulses[p].speed * (conf.speed * 60);
      if (pulses[p].x > W + 80) respawn(pulses[p]);
    }

    var pulseRows = {};
    for (var q = 0; q < pulses.length; q++) {
      var pulse = pulses[q];
      if (!pulseRows[pulse.row]) pulseRows[pulse.row] = [];
      pulseRows[pulse.row].push(pulse);
    }

    var rowHeight = 48;
    for (var i = 0; i < dots.length; i++) {
      var dot = dots[i];
      var depth = dot.z;
      var drift = Math.sin(clock * 0.0006 + dot.phase) * 5;

      /* Convergence bends every row toward the terminal on the install
         section, so the field visibly feeds the install command. */
      var bend = convergence ? (1 - Math.min(1, Math.abs(dot.y - coreY) / H)) * 46 : 0;
      var y = dot.y - scroll * 30 * depth + drift + (dot.x - coreX) * tilt * 0.14 * (1 - convergence);
      var x = dot.x + drift * 0.4 + bend * 0.6;

      if (convergence) {
        var pull = (x - coreX) * 0.06 * convergence;
        x -= pull;
        y += (coreY - y) * 0.08 * convergence;
      }

      var lit = 0;
      var rowPulses = pulseRows[dot.row];
      if (rowPulses) {
        for (var s = 0; s < rowPulses.length; s++) {
          var gap = Math.abs(rowPulses[s].x - dot.x);
          if (gap < 130) lit += (1 - gap / 130) * rowPulses[s].strength;
        }
      }
      dot.lit = lit;

      var near = 0;
      if (pointer.active) {
        var dx = pointer.x - x;
        var dy = pointer.y - y;
        var dist = Math.sqrt(dx * dx + dy * dy);
        if (dist < 190) near = 1 - dist / 190;
      }

      var alpha = (0.13 + lit * 0.55 + near * 0.6) * conf.alpha * depth;
      if (alpha <= 0.006) continue;

      var size = (1 + depth * 1.2 + lit * 1.3 + near * 1.6) * 1.15;
      ctx.fillStyle = lit > 0.35 || near > 0.4 ? rgba(accent, Math.min(1, alpha * 1.5)) : rgba(ink, Math.min(0.8, alpha));
      ctx.fillRect(x, y, size, size);
    }

    /* A soft bloom marks the core, the one place every request passes. */
    var glowX = coreX;
    var glowY = convergence ? coreY : H * 0.46;
    var gradient = ctx.createRadialGradient(glowX, glowY, 0, glowX, glowY, Math.max(W, H) * 0.24);
    gradient.addColorStop(0, rgba(accent, convergence ? 0.075 : 0.03));
    gradient.addColorStop(1, rgba(accent, 0));
    ctx.fillStyle = gradient;
    ctx.fillRect(0, 0, W, H);
  }

  /* ------------------------------------------------------------- the loop -- */

  var raf = 0;
  var last = 0;

  function tick(now) {
    if (!running) return;
    var delta = last ? Math.min(now - last, 48) : 16;
    last = now;
    clock += delta;
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

  /* ------------------------------------------------------- the curtain ---- */

  var curtain = document.getElementById("curtain");
  var veilCtx = curtain ? curtain.getContext("2d") : null;
  var veil = [];
  var veilStart = 0;

  function buildVeil() {
    if (!veilCtx) return false;
    var w = window.innerWidth;
    var h = window.innerHeight;
    var scale = Math.max(2, Math.min(w / 5, h / 5)) / 62;
    var sample = document.createElement("canvas");
    var size = Math.ceil(62 * scale * 4.2);
    sample.width = size;
    sample.height = Math.ceil(size / 2.6);
    var c = sample.getContext("2d");
    c.fillStyle = "#fff";
    c.font = "700 " + Math.floor(size / 2.9) + "px system-ui, -apple-system, 'Segoe UI', sans-serif";
    c.textAlign = "center";
    c.textBaseline = "middle";
    c.fillText("Relo.", sample.width / 2, sample.height / 2);

    var data = c.getImageData(0, 0, sample.width, sample.height).data;
    var step = Math.max(1, Math.floor(scale * 0.85));
    veil = [];
    var cx = w / 2;
    var cy = h / 2;
    for (var y = 0; y < sample.height; y += step) {
      for (var x = 0; x < sample.width; x += step) {
        if (data[(y * sample.width + x) * 4 + 3] < 140) continue;
        /* Four in ten lit pixels is plenty to read the word and keeps the
           particle count near 400. */
        if ((x + y) % 5 > 3) continue;
        veil.push({
          tx: cx + (x - sample.width / 2),
          ty: cy + (y - sample.height / 2),
          x: cx + (Math.random() - 0.5) * w * 1.1,
          y: cy + (Math.random() - 0.5) * h * 1.1,
          vx: 0,
          vy: 0
        });
      }
    }
    return veil.length > 0;
  }

  function drawVeil(progress, release) {
    var w = window.innerWidth;
    var h = window.innerHeight;
    var dprc = Math.min(window.devicePixelRatio || 1, 2);
    if (curtain.width !== Math.floor(w * dprc)) {
      curtain.width = Math.floor(w * dprc);
      curtain.height = Math.floor(h * dprc);
      curtain.style.width = w + "px";
      curtain.style.height = h + "px";
    }
    veilCtx.setTransform(dprc, 0, 0, dprc, 0, 0);
    veilCtx.clearRect(0, 0, w, h);

    var ease = 1 - Math.pow(1 - Math.min(1, progress), 3);
    for (var i = 0; i < veil.length; i++) {
      var part = veil[i];
      var x = part.x + (part.tx - part.x) * ease;
      var y = part.y + (part.ty - part.y) * ease;
      if (release > 0) {
        var dx = x - w / 2;
        var dy = y - h / 2;
        x += dx * release * 1.4;
        y += dy * release * 1.4;
      }
      veilCtx.fillStyle = rgba(accent, 0.85 * (1 - release));
      veilCtx.fillRect(x, y, 2.2, 2.2);
    }
  }

  function runCurtain() {
    if (!veilCtx || reduced) {
      if (curtain) curtain.remove();
      return;
    }
    if (!buildVeil()) {
      curtain.remove();
      return;
    }
    curtain.classList.add("is-on");
    veilStart = performance.now();
    var step = function (now) {
      var elapsed = now - veilStart;
      var assemble = Math.min(1, elapsed / 1100);
      var release = Math.max(0, Math.min(1, (elapsed - 1250) / 500));
      drawVeil(assemble, release);
      if (release >= 1) {
        veilCtx.clearRect(0, 0, curtain.width, curtain.height);
        curtain.classList.add("is-out");
        window.setTimeout(function () { curtain.remove(); }, 320);
        try { sessionStorage.setItem("relo-intro", "1"); } catch (e) {}
        return;
      }
      window.requestAnimationFrame(step);
    };
    window.requestAnimationFrame(step);
  }

  /* ---------------------------------------------------------- the wiring --- */

  layout();
  if (reduced) {
    paintStill();
  } else {
    if (!root.classList.contains("skip-intro") && !viewedBefore()) runCurtain();
    else if (curtain) curtain.remove();
    play();
  }

  function viewedBefore() {
    try { return sessionStorage.getItem("relo-intro") === "1"; } catch (e) { return true; }
  }

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

  window.addEventListener("pointermove", function (event) {
    pointer.x = event.clientX;
    pointer.y = event.clientY;
    pointer.active = true;
  }, { passive: true });

  window.addEventListener("pointerleave", function () { pointer.active = false; });

  window.addEventListener("scroll", function () {
    var max = Math.max(1, document.documentElement.scrollHeight - window.innerHeight);
    scroll = Math.min(1, window.scrollY / max);
  }, { passive: true });

  /* Sections announce their mood; the field cross-fades in about half a
     second because every mode reads the same loop. */
  var sections = document.querySelectorAll("[data-field]");
  if ("IntersectionObserver" in window && sections.length) {
    var watcher = new IntersectionObserver(function (entries) {
      for (var i = 0; i < entries.length; i++) {
        if (!entries[i].isIntersecting) continue;
        var next = entries[i].target.getAttribute("data-field");
        if (next && next !== mode && MODES[next]) {
          mode = next;
          SPACING = next === "story" ? 40 : 48;
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

  document.addEventListener("relo:lang", function () {
    if (reduced) paintStill();
  });
})();
