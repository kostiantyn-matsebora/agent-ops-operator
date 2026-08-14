// The on-this-page column, built from the rendered headings.
//
// It is JavaScript because GitHub Pages enables no table-of-contents plugin and
// the site refuses to become an Actions workflow for one. kramdown already
// gives every heading an id, so there is nothing to generate but the list.
(function () {
  var toc = document.getElementById('ao-toc');
  if (!toc) return;

  var nav = toc.querySelector('.ao-toc-nav');
  var content = document.getElementById('ao-content');
  if (!nav || !content) return;

  var headings = [].slice
    .call(content.querySelectorAll('h2[id], h3[id]'))
    // The column's own title is a heading in the page; it must not list itself.
    .filter(function (h) { return !toc.contains(h); });

  // One heading is not a contents — it is the page. Leave the rail hidden
  // rather than showing a list of one.
  if (headings.length < 2) return;

  var list = document.createElement('ul');
  list.className = 'ao-toc-list';

  var links = headings.map(function (h) {
    var li = document.createElement('li');
    li.className = 'ao-toc-item ao-toc-item--' + h.tagName.toLowerCase();
    var a = document.createElement('a');
    a.className = 'ao-toc-link';
    a.href = '#' + h.id;
    a.textContent = h.textContent;
    li.appendChild(a);
    list.appendChild(li);
    return a;
  });

  nav.appendChild(list);
  toc.hidden = false;

  var current = null;
  function mark(link) {
    if (current === link) return;
    if (current) {
      current.classList.remove('is-current');
      current.removeAttribute('aria-current');
    }
    current = link;
    if (current) {
      current.classList.add('is-current');
      current.setAttribute('aria-current', 'true');
    }
  }

  // Which heading is being read: the last one whose top has passed under the
  // masthead. Computed from positions rather than from intersection ratios,
  // which go quiet when a section is taller than the viewport — the case a
  // long page spends most of its time in.
  var offset = 0;
  var masthead = getComputedStyle(document.documentElement)
    .getPropertyValue('--ao-masthead-h');
  if (masthead) offset = parseFloat(masthead) * 16 || 0;

  function update() {
    var index = 0;
    for (var i = 0; i < headings.length; i++) {
      if (headings[i].getBoundingClientRect().top - offset - 8 <= 0) index = i;
      else break;
    }
    // At the very bottom the last section may never reach the top of the
    // viewport; without this its entry can never light up.
    if (window.innerHeight + window.scrollY >= document.body.scrollHeight - 2) {
      index = headings.length - 1;
    }
    mark(links[index]);
  }

  var ticking = false;
  function onScroll() {
    if (ticking) return;
    ticking = true;
    window.requestAnimationFrame(function () {
      ticking = false;
      update();
    });
  }

  window.addEventListener('scroll', onScroll, { passive: true });
  window.addEventListener('resize', onScroll, { passive: true });
  update();
})();
