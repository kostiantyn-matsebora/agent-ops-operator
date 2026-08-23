// A copy control on every code block.
//
// The PAGE writes no markup for it, exactly as it writes none for the platform
// tabs: a fence is a fence, and the theme supplies the affordance. So a page
// author cannot forget one, and a generated block gets it for free.
//
// WITH NO JAVASCRIPT there is no button and the code is still there to select,
// which is the state every one of these pages was in until now.
(function () {
  var content = document.getElementById('ao-content');
  if (!content) return;

  var REVERT_MS = 1600;

  // The element that carries the block's own border, so the control sits ON the
  // block rather than beside it. Rouge nests div.language-x > div.highlight >
  // pre; a fence with no language is a bare pre and gets a wrapper of its own.
  function anchorFor(pre) {
    var parent = pre.parentNode;
    if (parent && parent.nodeType === 1 && parent.classList.contains('highlight')) {
      return parent;
    }
    var wrap = document.createElement('div');
    wrap.className = 'ao-codeblock';
    parent.insertBefore(wrap, pre);
    wrap.appendChild(pre);
    return wrap;
  }

  // navigator.clipboard needs a secure context. GitHub Pages is https and
  // localhost counts as secure, so the fallback is for a plain-http preview
  // somebody serves off another host — rare, and silently doing nothing there
  // would look like a broken button.
  function write(text) {
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(text);
    }
    return new Promise(function (resolve, reject) {
      var ta = document.createElement('textarea');
      ta.value = text;
      ta.setAttribute('readonly', '');
      ta.style.position = 'fixed';
      ta.style.top = '-1000px';
      document.body.appendChild(ta);
      ta.select();
      var ok = false;
      try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
      document.body.removeChild(ta);
      ok ? resolve() : reject(new Error('copy refused'));
    });
  }

  [].forEach.call(content.querySelectorAll('pre'), function (pre) {
    var anchor = anchorFor(pre);
    anchor.classList.add('ao-has-copy');

    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'ao-copy';
    btn.textContent = 'Copy';
    // The block itself is not announced, so the label says what is copied.
    btn.setAttribute('aria-label', 'Copy code to clipboard');

    var timer = null;
    btn.addEventListener('click', function () {
      // Read at CLICK time, never at build time: the tab panels move blocks
      // around and a cached string would outlive its own element.
      var text = (pre.textContent || '').replace(/\n+$/, '\n');
      write(text).then(function () {
        settle('Copied', 'is-done');
      }, function () {
        settle('Press Ctrl+C', 'is-failed');
      });
    });

    function settle(label, cls) {
      btn.textContent = label;
      btn.classList.add(cls);
      window.clearTimeout(timer);
      timer = window.setTimeout(function () {
        btn.textContent = 'Copy';
        btn.classList.remove('is-done', 'is-failed');
      }, REVERT_MS);
    }

    anchor.appendChild(btn);
  });
})();
