// ONE resolver for every themed asset on the site.
//
// A page names ONE file, always the `-light` one, and this rewrites it when the
// document resolves dark. That keeps "there are two themes" out of the page,
// where it would be theme knowledge in content.
//
// It lives on its own because two components need it — the tab strip resolves
// what its panels point at, the player resolves a recording and its poster —
// and two painters watching one `data-theme` attribute would disagree on a
// toggle, resolving one asset and leaving the other on the wrong variant.
//
// Registration is BY ELEMENT REFERENCE, so a component that moves a node into a
// panel afterwards changes nothing.
(function () {
  // The suffix is MATCHED, never assumed: a reference with no `-light`/`-dark`
  // stem is left exactly as the page wrote it, so an ordinary link in a panel is
  // not rewritten into one that 404s.
  var VARIANT = /-(?:light|dark)(\.(?:png|svg|mp4))$/;

  var targets = [];

  function paint() {
    var dark = document.documentElement.getAttribute('data-theme') === 'dark';
    targets.forEach(function (target) {
      var was = target.el.getAttribute(target.attr);
      if (was === null) return;
      var want = was.replace(VARIANT, (dark ? '-dark' : '-light') + '$1');
      if (want !== was) target.el.setAttribute(target.attr, want);
    });
  }

  window.agentops = window.agentops || {};

  /** Resolve `attr` on `el` against the reader's theme, now and on every change. */
  window.agentops.themed = function (el, attr) {
    targets.push({ el: el, attr: attr });
    paint();
  };

  // CONTENT IMAGES THE PAGE NAMED DIRECTLY.
  //
  // Until now the only registrations came from the tab strip and the player, so
  // a themed asset a page named OUTSIDE either — a diagram in ordinary prose —
  // had no resolver at all. It rendered the light file on a dark page, silently.
  // The landing diagram was only correct because it happens to sit in a panel.
  //
  // Registration is by element reference, so a component that later moves one of
  // these into a panel changes nothing, and a second registration is a no-op:
  // paint() rewrites only when the resolved name differs from what is there.
  // THE WHOLE DOCUMENT, not the content column. This file opens by calling
  // itself the ONE resolver for every themed asset on the site, and scanning
  // only `#ao-content` made that false the moment the CHROME carried one: the
  // masthead's source mark sat outside, so it rendered the light file on a dark
  // page — the same silent failure this block was added to fix for diagrams.
  [].forEach.call(document.querySelectorAll('img[src]'), function (img) {
    if (VARIANT.test(img.getAttribute('src'))) window.agentops.themed(img, 'src');
  });

  paint();
  new MutationObserver(paint).observe(document.documentElement, {
    attributes: true, attributeFilter: ['data-theme'],
  });
})();
