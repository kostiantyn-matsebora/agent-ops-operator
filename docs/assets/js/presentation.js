// The presentation, for a page that explains a model one beat at a time.
//
// The page writes an ordinary ordered list named `{: .ao-presentation}`: one
// item per beat, the item's text is that beat's caption. This reads the list,
// builds the drawing, and removes the list.
//
// IT IS ONE FIGURE, AND THE FIGURE IS THE CONTROL. The drawing carries its own
// caption and clicking it pauses. There was a transport — a play button, a beat
// counter, ten scrub dots, a progress bar and a box showing each beat's
// manifest lines, in two bordered boxes under the picture. That was MORE
// MACHINERY THAN THE THING IT EXPLAINED, and the manifest it showed is already
// the strip's third panel.
//
// WITH NO SCRIPTING THE LIST IS THE WHOLE EXPLANATION — nine beats in order,
// each carrying the lines it concerns. That is a working panel rather than a
// fallback for one, and it is the same bargain the tab strip and the player
// make.
//
// WHAT IS HERE AND WHAT IS THE PAGE'S. The drawing is the theme's: its shape,
// its coordinates and the connectors drawn between them are geometry, not
// prose, and no markdown shape states them. Every WORD a beat says is the
// page's, and changing one is an edit to `index.md` and to nothing else.
//
// It names no integration mark. A vendor file named here would be product
// knowledge in the theme, which is the one thing a component may not carry.
//
// IT WATCHES NO THEME AND REGISTERS NOTHING WITH `themed.js`. Every colour it
// draws with is a palette token, so a toggle repaints it in the same style
// recalculation that repaints the page. There is no `-light` file to resolve,
// and a second painter watching `data-theme` is exactly what the one resolver
// exists to prevent — the strip and the player register because they name
// assets, and this does not.
(function () {
  'use strict';

  var content = document.getElementById('ao-content');
  if (!content) return;

  var lists = [].slice.call(content.querySelectorAll('ol.ao-presentation'));
  if (!lists.length) return;

  // The stage is authored at this size and SCALED to the width it is given.
  //
  // THE DRAWING IS INSET FROM THE CLIP EDGE ON EVERY SIDE IT REACHES.
  //
  // The install frame is the outermost element, and it was flush with the
  // drawing's own bounds on three of them — so in a box clipped by
  // `overflow: hidden` its 1px border landed on the clip edge itself. At the
  // TOP that took the frame's border and its whole label with it, which is why
  // "your cluster · one Helm install" had never once been seen. At the bottom
  // and the right it survived at one device pixel ratio and was eaten at the
  // next, which is a defect that only ever appears on somebody else's screen.
  //
  // The top slack is the biggest because the frame's label sits ABOVE the frame,
  // outside the drawing's own box. Nothing may end flush with this boundary.
  // 668 of DRAWING, then a gap, then the CAPTION'S OWN LANE. The lane is what
  // makes "beside the element the beat is about" possible at all: the drawing
  // is dense — six boxes, a chip row and seven connectors — and a caption
  // placed near an anchor collided with something on every beat. A search over
  // ten candidate positions still could not find clear space, because there is
  // none. Reserving the room is the fix a cost function cannot be.
  //
  // THE COST IS PICTURE WIDTH, and it is paid knowingly: the drawing renders at
  // about 1.05 rather than 1.39, so it is still larger than the 668 it was
  // authored at and no longer the whole column.
  var CAP_LANE = 190;
  var CAP_GUTTER = 20;
  var DRAW_W = 668 + CAP_GUTTER + CAP_LANE;
  // 208 once, and that was the ceiling the narrow column imposed rather than a
  // shape the drawing wanted: three bands in 208px left 43px between the first
  // and the second and NINE between the toolset and the reach chips it grants,
  // so the whole thing read as one flat strip.
  var DRAW_H = 245;
  var PAD_TOP = 28;
  var PAD_BOTTOM = 4;
  var PAD_RIGHT = 2;
  var STAGE_W = DRAW_W + PAD_RIGHT;
  var STAGE_H = PAD_TOP + DRAW_H + PAD_BOTTOM;
  // 5200 read as a stall rather than a beat: long enough that a reader who had
  // finished the sentence went looking for a control. A caption is six words —
  // it is read in about a second, and the drawing does the rest.
  var HOLD = 2600;

  // The drawing: five declared objects, the conversation they open, the reach
  // the wiring granted, and the install that frames them.
  var NODES = [
    { id: 'source', kind: 'SignalSource', name: 'cluster-events' },
    { id: 'pipeline', kind: 'Pipeline', name: 'k8s-ops' },
    { id: 'channel', kind: 'Channel', name: 'telegram' },
    { id: 'profile', kind: 'AgentProfile', name: 'k8s-engineer' },
    { id: 'toolset', kind: 'MCPToolset', name: 'agentops-observe' },
    { id: 'mcpconfig', kind: 'MCPConfig', name: 'k8s-api' },
  ];

  var REACH = [
    { text: 'read pods, events, logs', granted: true },
    { text: 'query metrics', granted: true },
    { text: 'delete anything', granted: false },
    { text: 'a shell', granted: false },
  ];

  // Each connector is drawn twice: a dotted TRACK that is always there, and the
  // line itself, revealed by running its dash offset back to zero. The length
  // is stated because a dasharray cannot be a percentage.
  var WIRES = [
    { id: 'w-source', d: 'M164 30 H248', len: 85 },
    { id: 'w-channel', d: 'M412 30 H496', len: 85 },
    { id: 'w-profile', d: 'M300 58 V96 H244 V118', len: 118 },
    { id: 'w-toolset', d: 'M360 58 V96 H416 V118', len: 118 },
    // Turns ABOVE the toolset's elbow rather than below it, so the two never
    // share a horizontal run. A crossing reads as a junction that is not there.
    { id: 'w-cfg', d: 'M406 58 V84 H578 V118', len: 234 },
    { id: 'w-reach', d: 'M416 163 V186', len: 23 },
    { id: 'w-conv', d: 'M82 58 V150', len: 92 },
  ];

  // What each beat TURNS ON, and what it LIGHTS. `on` accumulates — what a beat
  // established stays established — and `lit` is only ever the current beat's
  // subject, so scrubbing backwards shows exactly what that beat established.
  //
  // Keyed by position, because the page states the beats and this states what
  // they are about. A page writing more beats than there are entries simply
  // gets its captions with nothing new brought forward.
  var SCRIPT = [
    { on: ['source'], lit: ['source'] },
    { on: ['frame', 'pipeline'], lit: [] },
    { on: [], lit: ['pipeline'] },
    { on: ['w-source'], lit: ['source', 'w-source'] },
    { on: ['profile', 'w-profile'], lit: ['profile', 'w-profile'] },
    { on: ['toolset', 'reach', 'w-toolset', 'w-reach'], lit: ['toolset', 'w-toolset'] },
    { on: ['mcpconfig', 'w-cfg'], lit: ['mcpconfig', 'w-cfg'] },
    { on: ['channel', 'w-channel'], lit: ['channel', 'w-channel'] },
    { on: ['conv', 'w-conv'], lit: ['conv', 'w-conv'] },
    { on: [], lit: [] },
  ];

  var SVG_NS = 'http://www.w3.org/2000/svg';

  function el(tag, cls, parent) {
    var node = document.createElement(tag);
    if (cls) node.className = cls;
    if (parent) parent.appendChild(node);
    return node;
  }

  function svg(tag, parent) {
    var node = document.createElementNS(SVG_NS, tag);
    if (parent) parent.appendChild(node);
    return node;
  }

  /** The drawing, fully laid out and unemphasised. Returns its elements by id. */
  function buildStage(stage) {
    var parts = {};

    var frame = el('div', 'ao-pres-frame', stage);
    el('span', null, frame).textContent = 'your cluster · one Helm install';
    parts.frame = frame;

    var wire = svg('svg', stage);
    wire.setAttribute('class', 'ao-pres-wire');
    wire.setAttribute('viewBox', '0 0 660 ' + DRAW_H);
    wire.setAttribute('aria-hidden', 'true');
    WIRES.forEach(function (w) {
      var track = svg('path', wire);
      track.setAttribute('class', 'ao-pres-track');
      track.setAttribute('d', w.d);
    });
    WIRES.forEach(function (w) {
      var path = svg('path', wire);
      path.setAttribute('class', 'ao-pres-draw');
      path.setAttribute('d', w.d);
      path.style.setProperty('--ao-pres-len', w.len);
      parts[w.id] = path;
    });

    NODES.forEach(function (n) {
      var node = el('div', 'ao-pres-node', stage);
      node.setAttribute('data-node', n.id);
      el('span', 'ao-pres-kind', node).textContent = n.kind;
      el('span', 'ao-pres-name', node).textContent = n.name;
      parts[n.id] = node;
    });

    var reach = el('div', 'ao-pres-reach', stage);
    REACH.forEach(function (r) {
      var chip = el('span', 'ao-pres-chip ' + (r.granted ? 'is-granted' : 'is-denied'), reach);
      chip.textContent = r.text;
    });
    parts.reach = reach;

    var conv = el('div', 'ao-pres-conv', stage);
    var kind = el('span', 'ao-pres-kind', conv);
    el('span', 'ao-pres-dot', kind);
    kind.appendChild(document.createTextNode('Conversation · running'));
    el('span', 'ao-pres-name', conv).textContent = 'cluster-events-7c1d4e';
    el('i', null, conv);
    el('i', null, conv);
    parts.conv = conv;

    return parts;
  }

  lists.forEach(function (list) {
    var items = [].slice.call(list.children).filter(function (li) {
      return li.tagName === 'LI';
    });
    if (items.length < 2) return;

    // The caption is the item's own text, and the stanza is whatever fenced
    // block the item carries. Reading them out before anything is built keeps
    // the page the single source of both.
    var beats = items.map(function (li) {
      var pre = li.querySelector('pre');
      var holder = pre && (pre.closest('div[class*="language-"]') || pre);
      // The <code>, not the <pre>. `copy.js` puts a control on every <pre> it
      // finds, and a copy button on a three-line excerpt that changes under
      // the reader offers the wrong thing — the manifest to copy is the
      // strip's third panel, whole. Carrying the `highlight` class keeps the
      // block's own token colours, which are scoped to it.
      // Taken out before the caption is read, so the block's own lines cannot
      // end up in the beat's sentence. Put back, because the reduced-motion
      // path shows the list and a beat without its lines is a beat cut in half.
      if (holder) holder.parentNode.removeChild(holder);
      var beat = { text: li.textContent.replace(/\s+/g, ' ').trim() };
      if (holder) li.appendChild(holder);
      return beat;
    });

    var wrap = el('div', 'ao-pres');
    list.parentNode.insertBefore(wrap, list);

    var viewport = el('div', 'ao-pres-viewport', wrap);
    var stage = el('div', 'ao-pres-stage', viewport);
    var parts = buildStage(stage);

    // THE CAPTION IS IN THE DRAWING, not a row under it. It is appended to the
    // STAGE, so it is positioned in the drawing's own coordinates and is scaled
    // by the same transform — it moves and sizes WITH the picture instead of
    // being a line of page text that happens to sit below one.
    //
    // It went through a bordered rail with a play button, a beat counter, ten
    // scrub dots and a progress bar, and a box above it showing each beat's
    // manifest lines. All of it was more machinery than the drawing it
    // explained, and the manifest is already the strip's third panel.
    //
    // THERE IS NO BEAT COUNTER. "1 / 10" answered a question nobody reading a
    // landing page has: it is not a form to complete or a queue to get through,
    // and a number counting down beside a sentence invites waiting for the end
    // rather than reading the sentence.
    var text = el('div', 'ao-pres-caption', stage);

    // THE FIGURE IS THE CONTROL. A reader pauses by clicking what they are
    // looking at, which is the one target nobody has to find — and it costs no
    // button, no glyph and no second thing to style, label and keep reachable.
    //
    // A real `tabindex` and a key handler, not a `<div>` with a click: the
    // transport it replaces was operable from the keyboard and this must be
    // too.
    wrap.setAttribute('role', 'group');
    wrap.setAttribute('aria-label', 'How it works, one beat at a time');
    wrap.tabIndex = 0;

    // NO LIVE REGION. One that announced every advance would interrupt a
    // screen-reader user every 5.2 seconds on the page they landed on. The
    // beats stay readable as the list below instead — by being read, rather
    // than by being read at.

    var current = 0;
    var timer = null;
    // Whether the presentation is sitting as the reduced-motion still. Read by
    // the play button, cleared for good by `engage`.
    var stillNow = false;
    var started = 0;

    // WHERE THE BEAT IS HAPPENING. A stepped diagram puts its sentence beside
    // the thing it is currently about — that is the whole reason the drawing is
    // stepped rather than drawn once. A fixed caption makes the reader find the
    // change for themselves on every beat.
    //
    // IN THE LANE, VERTICALLY ALIGNED TO THE ANCHOR. The caption's column is
    // reserved, so there is nothing to collide with and nothing to search: only
    // its height moves, tracking the middle of whatever the beat lights.

    /** The element a beat is about: what it LIGHTS, else what it turns on.
        Never the frame — it is the whole drawing, so "beside it" is nowhere,
        and never a connector, which has no box to sit beside. */
    function anchorFor(n) {
      var beat = SCRIPT[n] || {};
      var ids = (beat.lit || []).concat(beat.on || []);
      for (var i = 0; i < ids.length; i++) {
        var e = parts[ids[i]];
        if (e && e.offsetWidth && !e.classList.contains('ao-pres-frame')) return e;
      }
      // A beat about the whole picture — the install, and the closing line.
      return parts.pipeline;
    }

    function place(n) {
      var a = anchorFor(n);
      var h = text.offsetHeight || 34;
      var mid = a.offsetTop + a.offsetHeight / 2 - h / 2;
      text.style.top = Math.max(0, Math.min(DRAW_H - h, mid)) + 'px';
    }

    /** Paint the stage as of beat n, from scratch. Idempotent, so scrubbing
        back is the same code path as playing forward. */
    function goTo(n) {
      current = n;
      var on = {};
      var lit = {};
      for (var i = 0; i <= n && i < SCRIPT.length; i++) {
        SCRIPT[i].on.forEach(function (id) { on[id] = true; });
      }
      if (SCRIPT[n]) SCRIPT[n].lit.forEach(function (id) { lit[id] = true; });

      Object.keys(parts).forEach(function (id) {
        parts[id].classList.toggle('is-on', !!on[id]);
        parts[id].classList.toggle('is-lit', !!lit[id]);
      });

      text.textContent = beats[n].text;
      place(n);
    }

    function advance() {
      goTo((current + 1) % beats.length);
      started = Date.now();
      timer = setTimeout(advance, HOLD);
    }

    function start() {
      if (timer) return;
      started = Date.now();
      timer = setTimeout(advance, HOLD);
      wrap.classList.remove('is-paused');
    }

    function pause() {
      clearTimeout(timer);
      timer = null;
      wrap.classList.add('is-paused');
    }

    // CLICKING THE FIGURE TOGGLES IT, and while it is sitting as the
    // reduced-motion still that same click is what starts it. One target, one
    // handler, both configurations.
    //
    // A click that lands on a LINK or a selection the reader is making is not
    // a request to pause, so neither is intercepted.
    function toggle() {
      if (stillNow) { engage(); return; }
      if (timer) pause(); else start();
    }
    wrap.addEventListener('click', function (e) {
      if (e.target.closest('a')) return;
      var sel = window.getSelection();
      if (sel && String(sel).length) return;
      toggle();
    });
    wrap.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar') {
        e.preventDefault();
        toggle();
      }
    });

    // ENGAGING IS ONE-WAY, and it is the READER'S OWN action rather than
    // something the page did at them — which is the whole of what the
    // preference asks. There is no path back to the still: pausing afterwards
    // leaves the ordinary paused transport, because taking the beat list back
    // out from under a reader who has just pressed play would move the page for
    // the person who asked for less movement.
    //
    // It starts at beat ONE, not at what the still was composed as. The still
    // is every element lit at once, which is the LAST beat's state — resuming
    // from there would play the ending and wrap around to the beginning.
    function engage() {
      stillNow = false;
      wrap.classList.remove('is-still');
      list.parentNode.removeChild(list);
      goTo(0);
      start();
    }

    // SCALE, NEVER REFLOW. `transform` does not reduce the stage's layout
    // width, so the viewport is given the scaled height in the same step —
    // half of that pair is a presentation with a scrollbar inside it.
    function fit() {
      // FILLS THE COLUMN. Capped at 1 while the column was 45rem and the
      // stage was authored to just fit it. At 58rem the cap left the drawing
      // at 668px pinned to the left of a 928px box. Unbounded is safe because
      // the COLUMN is bounded by `--ao-measure-wide`.
      var k = viewport.clientWidth / STAGE_W;
      // Scale THEN drop, so the top slack scales with everything else: a
      // constant offset would over-pad the drawing at every narrow width.
      stage.style.transform = 'translate(0, ' + (PAD_TOP * k) + 'px) scale(' + k + ')';
      viewport.style.height = Math.ceil(STAGE_H * k) + 'px';
    }
    if (window.ResizeObserver) new ResizeObserver(fit).observe(viewport);
    window.addEventListener('resize', fit);
    fit();

    list.parentNode.removeChild(list);

    // REDUCED MOTION IS THE COMPOSED DRAWING PLUS THE LIST, AND A WAY IN.
    // Nothing moves, the whole model is on the stage at once, and the beats
    // stay readable in order — the same content the no-scripting reader gets.
    //
    // THE PLAY BUTTON SURVIVES, AND THAT IS THE POINT OF THIS BRANCH. The
    // transport used to be removed with the motion it drove, which answered
    // `reduce` by DELETING the control: a reader with the system setting on
    // could not play the page's central explanation even deliberately, and it
    // was reported from a second machine as a missing animation — which is also
    // what a broken script looks like. The preference says do not move things
    // AT me. It does not say never let me ask.
    var still = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (still) {
      goTo(beats.length - 1);
      Object.keys(parts).forEach(function (id) { parts[id].classList.add('is-on'); });
      pause();
      stillNow = true;
      wrap.classList.add('is-still');
      wrap.parentNode.insertBefore(list, wrap.nextSibling);
    } else {
      // Already detached at the top of this function — this branch just
      // leaves it that way. The duplicate removeChild here threw on
      // `list.parentNode` being null, since line 404 had already removed it.
      goTo(0);
      start();
    }
  });
})();
