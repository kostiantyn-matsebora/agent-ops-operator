// Two tab components, one file, one look.
//
// 1. PLATFORM TABS over adjacent code fences — the page names the shell with
//    the fence language and writes no markup at all.
// 2. PANEL TABS over a markdown list named `{: .ao-tabs}` — the page writes
//    every word and every image, and this supplies the strip and the selection.
//
// Nothing here knows what any command does or what any panel is about.

// --- 1. platform tabs --------------------------------------------------------
(function () {
  var LABELS = {
    sh: 'Linux / macOS',
    bash: 'Linux / macOS',
    shell: 'Linux / macOS',
    zsh: 'Linux / macOS',
    powershell: 'Windows',
    ps1: 'Windows',
  };
  var STORE = 'agentops-docs-platform';

  var content = document.getElementById('ao-content');
  if (!content) return;

  function langOf(el) {
    if (!el || el.nodeType !== 1 || !el.className) return null;
    var m = /(?:^|\s)language-([a-z0-9]+)/.exec(el.className);
    return m && LABELS[m[1]] ? m[1] : null;
  }

  // Runs of adjacent highlighted blocks, at ANY depth — the install commands
  // live inside numbered steps, so walking only the top level would miss them.
  var groups = [];
  var seen = [];
  [].forEach.call(content.querySelectorAll('div[class*="language-"]'), function (b) {
    if (seen.indexOf(b) >= 0 || !langOf(b)) return;
    var run = [b];
    seen.push(b);
    var n = b.nextElementSibling;
    while (langOf(n)) {
      run.push(n);
      seen.push(n);
      n = n.nextElementSibling;
    }
    // Two or more, and at least two DISTINCT labels — a lone block, or two
    // Linux blocks in a row, is not a platform choice.
    if (run.length > 1) {
      var labels = run.map(function (x) { return LABELS[langOf(x)]; });
      if (labels.some(function (l) { return l !== labels[0]; })) groups.push(run);
    }
  });
  if (!groups.length) return;

  var all = [];

  groups.forEach(function (run) {
    var wrap = document.createElement('div');
    wrap.className = 'ao-tabs';
    var list = document.createElement('div');
    list.className = 'ao-tablist';
    list.setAttribute('role', 'tablist');
    wrap.appendChild(list);

    run[0].parentNode.insertBefore(wrap, run[0]);

    var tabs = run.map(function (block, i) {
      var label = LABELS[langOf(block)];
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'ao-tab';
      btn.textContent = label;
      btn.setAttribute('role', 'tab');
      btn.dataset.platform = label;
      list.appendChild(btn);

      block.classList.add('ao-tabpanel');
      block.setAttribute('role', 'tabpanel');
      wrap.appendChild(block);
      return { btn: btn, panel: block, label: label };
    });

    all.push(tabs);

    list.addEventListener('click', function (e) {
      var btn = e.target.closest('.ao-tab');
      if (btn) select(btn.dataset.platform);
    });
    // Left/right arrows move between tabs, as a tablist should.
    list.addEventListener('keydown', function (e) {
      if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
      var i = tabs.findIndex(function (t) { return t.btn === document.activeElement; });
      if (i < 0) return;
      var next = tabs[(i + (e.key === 'ArrowRight' ? 1 : tabs.length - 1)) % tabs.length];
      next.btn.focus();
      select(next.label);
      e.preventDefault();
    });
  });

  // One choice for the whole page, remembered — a reader on Windows picks once.
  function select(label) {
    all.forEach(function (tabs) {
      var match = tabs.some(function (t) { return t.label === label; });
      tabs.forEach(function (t, i) {
        var on = match ? t.label === label : i === 0;
        t.btn.classList.toggle('is-current', on);
        t.btn.setAttribute('aria-selected', on ? 'true' : 'false');
        t.btn.tabIndex = on ? 0 : -1;
        t.panel.hidden = !on;
      });
    });
    try { localStorage.setItem(STORE, label); } catch (e) { /* private mode */ }
  }

  var stored = null;
  try { stored = localStorage.getItem(STORE); } catch (e) { /* ignore */ }
  if (!stored) {
    stored = /win/i.test(navigator.platform || '') ? 'Windows' : 'Linux / macOS';
  }
  select(stored);
})();

// --- 2. panel tabs -----------------------------------------------------------
//
// `{: .ao-tabs}` on an ordinary markdown list. Each item's leading <strong> is
// the tab label and the rest of the item is the panel — so WITH NO JAVASCRIPT
// the page is a labelled list with every panel visible, which is the whole
// content rather than a fallback for it.
//
// It also resolves the THEME VARIANT of anything a panel points at. A page names
// one file, always the `-light` one, and this rewrites it when the document
// resolves dark. That keeps "there are two themes" out of the page, where it
// would be theme knowledge in content.
//
// Images and links alike, screenshots and diagrams alike: the first panel holds
// a diagram whose full-size link is a second themed asset, and a light poster
// opened from the dark theme is the same fault one click later.
(function () {
  var content = document.getElementById('ao-content');
  if (!content) return;

  var lists = [].slice.call(content.querySelectorAll('ul.ao-tabs'));
  if (!lists.length) return;

  var themed = [];

  function slug(s) {
    return s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  }

  // The label is consumed by the strip, so the panel drops it — along with the
  // dash that separated it from the description. The first text node is found
  // rather than assumed: kramdown wraps a loose list's items in <p> and leaves
  // whitespace nodes around them, so "the panel's first child" is not the text.
  function stripLabel(panel, strong) {
    strong.parentNode.removeChild(strong);
    var walker = document.createTreeWalker(panel, NodeFilter.SHOW_TEXT, null);
    var node;
    while ((node = walker.nextNode())) {
      if (!node.nodeValue.trim()) continue;
      node.nodeValue = node.nodeValue.replace(/^\s*[—–-]\s*/, '');
      return;
    }
  }

  lists.forEach(function (list) {
    var items = [].slice.call(list.children).filter(function (li) {
      return li.tagName === 'LI' && li.querySelector('strong');
    });
    if (items.length < 2) return;

    var base = list.id || 'tab';
    var wrap = document.createElement('div');
    wrap.className = 'ao-tabs ao-tabs--panels';
    var strip = document.createElement('div');
    strip.className = 'ao-tablist';
    strip.setAttribute('role', 'tablist');
    wrap.appendChild(strip);
    list.parentNode.insertBefore(wrap, list);

    var tabs = items.map(function (li, i) {
      var strong = li.querySelector('strong');
      var label = strong.textContent.trim();
      var id = base + '-' + slug(label);

      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'ao-tab';
      btn.textContent = label;
      btn.id = 'tab-' + id;
      btn.setAttribute('role', 'tab');
      btn.setAttribute('aria-controls', id);
      strip.appendChild(btn);

      var panel = document.createElement('div');
      panel.className = 'ao-tabpanel';
      panel.id = id;
      panel.setAttribute('role', 'tabpanel');
      panel.setAttribute('aria-labelledby', btn.id);
      while (li.firstChild) panel.appendChild(li.firstChild);
      stripLabel(panel, strong);
      wrap.appendChild(panel);

      // Only the first panel is on screen at load, so only it is worth
      // fetching: a reader who never opens Queues never downloads it.
      [].forEach.call(panel.querySelectorAll('img'), function (img) {
        if (i > 0) img.setAttribute('loading', 'lazy');
        themed.push({ el: img, attr: 'src' });
      });
      [].forEach.call(panel.querySelectorAll('a[href]'), function (a) {
        themed.push({ el: a, attr: 'href' });
      });

      return { btn: btn, panel: panel, id: id };
    });

    list.parentNode.removeChild(list);

    function select(i, focus) {
      tabs.forEach(function (t, j) {
        var on = j === i;
        t.btn.classList.toggle('is-current', on);
        t.btn.setAttribute('aria-selected', on ? 'true' : 'false');
        t.btn.tabIndex = on ? 0 : -1;
        t.panel.hidden = !on;
      });
      if (focus) {
        tabs[i].btn.focus();
        tabs[i].btn.scrollIntoView({ block: 'nearest', inline: 'nearest' });
      }
    }

    strip.addEventListener('click', function (e) {
      var btn = e.target.closest('.ao-tab');
      if (!btn) return;
      var i = tabs.findIndex(function (t) { return t.btn === btn; });
      if (i < 0) return;
      select(i, true);
      // Deep-linkable, without yanking the page to the widget.
      if (window.history && history.replaceState) {
        history.replaceState(null, '', '#' + tabs[i].id);
      }
    });

    strip.addEventListener('keydown', function (e) {
      var i = tabs.findIndex(function (t) { return t.btn === document.activeElement; });
      if (i < 0) return;
      var to = null;
      if (e.key === 'ArrowRight') to = (i + 1) % tabs.length;
      else if (e.key === 'ArrowLeft') to = (i + tabs.length - 1) % tabs.length;
      else if (e.key === 'Home') to = 0;
      else if (e.key === 'End') to = tabs.length - 1;
      if (to === null) return;
      select(to, true);
      e.preventDefault();
    });

    var wanted = tabs.findIndex(function (t) {
      return '#' + t.id === location.hash;
    });
    select(wanted < 0 ? 0 : wanted, false);
  });

  // The variant follows the resolved theme, and repaints on a toggle so the
  // open panel is never the wrong one.
  //
  // The suffix is matched, never assumed: a reference with no `-light`/`-dark`
  // stem is left exactly as the page wrote it, so an ordinary link in a panel
  // is not rewritten into one that 404s.
  var VARIANT = /-(?:light|dark)(\.(?:png|svg))$/;

  function paint() {
    var dark = document.documentElement.getAttribute('data-theme') === 'dark';
    themed.forEach(function (t) {
      var was = t.el.getAttribute(t.attr);
      var want = was.replace(VARIANT, (dark ? '-dark' : '-light') + '$1');
      if (want !== was) t.el.setAttribute(t.attr, want);
    });
  }
  paint();
  new MutationObserver(paint).observe(document.documentElement, {
    attributes: true, attributeFilter: ['data-theme'],
  });
})();
