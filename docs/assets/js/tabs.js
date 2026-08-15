// Platform tabs over adjacent code blocks.
//
// A page writes two fences in a row — `sh` then `powershell` — and this pairs
// them into one tabbed widget. The page therefore carries NO markup and no tab
// component: it names the shell with the fence language, the same division as
// `{: .ao-cards}`. Nothing here knows what any command does.
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
