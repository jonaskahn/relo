/* Scroll story: one progress value drives the iPad wake, the card grid,
   and the harness bag. A pure function of progress, so scrubbing back
   reverses every scene. Static fallback lives in CSS. */
(function () {
  var root = document.documentElement;
  if (root.classList.contains('static-story')) return;
  if (!window.Motion || typeof window.Motion.scroll !== 'function') {
    root.classList.add('static-story');
    return;
  }

  var pin = document.querySelector('.story-pin');
  var ipad = document.getElementById('ipad');
  var boot = document.getElementById('boot');
  var cards = Array.prototype.slice.call(document.querySelectorAll('.conn'));
  var bagScene = document.getElementById('bagScene');
  var bag = document.getElementById('bag');
  var flyers = Array.prototype.slice.call(document.querySelectorAll('.fly'));
  var caps = Array.prototype.slice.call(document.querySelectorAll('.cap'));
  if (!pin || !ipad || !boot) return;

  function clamp01(x) { return x < 0 ? 0 : x > 1 ? 1 : x; }
  function band(p, a, b) { return clamp01((p - a) / (b - a)); }
  function ease(t) { return t * t * (3 - 2 * t); }

  var FLY_FROM = {
    left: [-260, 0], right: [260, 0], top: [0, -260], bottom: [0, 260]
  };

  function setCap(index) {
    caps.forEach(function (cap, i) {
      cap.classList.toggle('is-active', i === index);
    });
  }

  function render(p) {
    /* Scene 1: the iPad rises and wakes past the boot mark. */
    var rise = ease(band(p, 0.0, 0.12));
    ipad.style.transform = 'translateY(' + ((1 - rise) * 48).toFixed(1) + 'px) scale(' + (0.96 + 0.04 * rise).toFixed(3) + ')';
    ipad.style.opacity = String(band(p, 0.0, 0.06));
    boot.style.opacity = String(1 - band(p, 0.1, 0.18));

    /* Scene 2: nine connection cards fly in and settle into the screen. */
    cards.forEach(function (card, i) {
      var start = 0.18 + i * 0.018;
      var t = ease(band(p, start, start + 0.07));
      var dir = FLY_FROM[card.dataset.from || 'left'];
      var dx = dir[0] * (1 - t);
      var dy = dir[1] * (1 - t);
      var leave = band(p, 0.56 + i * 0.006, 0.64 + i * 0.006);
      card.style.opacity = String(Math.max(0, t * (1 - leave)));
      card.style.transform = 'translate(' + dx.toFixed(1) + '%,' + (dy - leave * 60).toFixed(1) + '%) scale(' + (1 - leave * 0.2).toFixed(3) + ')';
    });

    /* Scene 3: the iPad recedes as the cards leave. */
    var recede = ease(band(p, 0.54, 0.64));
    if (recede > 0) {
      ipad.style.transform = 'scale(' + (1 - 0.1 * recede).toFixed(3) + ')';
    }
    ipad.style.opacity = String(Math.min(band(p, 0.0, 0.06), 1 - band(p, 0.58, 0.66)));

    /* Scene 4: the bag settles; harness marks gather into it. */
    var bagT = ease(band(p, 0.64, 0.72));
    bagScene.style.opacity = String(bagT);
    var bagScale = 0.92 + 0.08 * bagT;
    bag.style.transform = 'scale(' + bagScale.toFixed(3) + ')';

    flyers.forEach(function (fly, i) {
      var start = 0.68 + (i % 6) * 0.02 + Math.floor(i / 6) * 0.03;
      var t = ease(band(p, start, start + 0.07));
      var spread = 46 - i * 2;
      var ang = (i / flyers.length) * Math.PI * 2;
      var sx = Math.cos(ang) * spread;
      var sy = Math.sin(ang) * spread * 0.7;
      var x = sx * (1 - t);
      var y = sy * (1 - t) - 10 * t;
      var fade = 1 - band(p, start + 0.06, start + 0.08);
      fly.style.opacity = String(Math.max(0, Math.min(1, t * 3) * fade));
      fly.style.transform = 'translate(calc(-50% + ' + x.toFixed(1) + 'vw),' + y.toFixed(1) + 'vh) scale(' + (1 - t * 0.55).toFixed(3) + ')';
    });

    /* Captions follow the active scene. */
    var cap = 0;
    if (p >= 0.62) cap = 3;
    else if (p >= 0.36) cap = 2;
    else if (p >= 0.14) cap = 1;
    if (p >= 0.88) cap = 4;
    setCap(cap);
  }

  render(0);
  var cancel = window.Motion.scroll(function (progress) {
    render(progress);
  }, { target: pin, offset: ['start start', 'end end'] });

  if (cancel && typeof cancel === 'function') {
    window.addEventListener('pagehide', cancel, { once: true });
  }
})();
