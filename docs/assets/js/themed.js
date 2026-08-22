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

  paint();
  new MutationObserver(paint).observe(document.documentElement, {
    attributes: true, attributeFilter: ['data-theme'],
  });
})();
