// Sidebar group state.
//
// The include renders every group OPEN — that no-script render is the
// baseline and must stay correct on its own. This script only closes the
// groups this reader closed before (persisted in localStorage, keyed by
// group title), ALWAYS leaving the group holding the current page open,
// and persists each toggle. Storage denied: the all-open render stands.
(function () {
  'use strict';
  var STORE = 'agentops-docs-nav-closed';

  function readClosed() {
    try {
      var list = JSON.parse(localStorage.getItem(STORE) || '[]');
      return Array.isArray(list) ? list : [];
    } catch (e) { return []; }
  }

  function writeClosed(list) {
    try { localStorage.setItem(STORE, JSON.stringify(list)); } catch (e) { /* private mode */ }
  }

  function titleOf(group) {
    var summary = group.querySelector('summary');
    return summary ? summary.textContent.trim() : '';
  }

  function holdsCurrentPage(group) {
    if (group.querySelector('[aria-current="page"]')) return true;
    var links = group.querySelectorAll('a[href]');
    for (var i = 0; i < links.length; i++) {
      if (links[i].pathname === window.location.pathname) return true;
    }
    return false;
  }

  var groups = document.querySelectorAll('.ao-sidebar details.ao-nav-group');
  [].forEach.call(groups, function (group) {
    var closed = readClosed().indexOf(titleOf(group)) >= 0;
    // Apply the stored choice BEFORE listening, so restoring it is not
    // re-recorded as a fresh toggle. The current page's group stays open
    // whatever was stored — the choice itself is kept for other pages.
    if (closed && !holdsCurrentPage(group)) group.removeAttribute('open');
    group.addEventListener('toggle', function () {
      var title = titleOf(group);
      var list = readClosed().filter(function (t) { return t !== title; });
      if (!group.open) list.push(title);
      writeClosed(list);
    });
  });
}());
