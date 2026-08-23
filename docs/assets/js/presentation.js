// The presentation, for a page that explains a model one beat at a time.
//
// The page writes an ordinary ordered list named `{: .ao-presentation}`: one
// item per beat, the item's text is that beat's caption, and a fenced block
// under it is the manifest stanza that beat is about. This reads the list,
// builds the stage and the transport beside it, and removes the list.
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
  var DRAW_W = 668;
  var DRAW_H = 208;
  var PAD_TOP = 28;
  var PAD_BOTTOM = 4;
  var PAD_RIGHT = 2;
  var STAGE_W = DRAW_W + PAD_RIGHT;
  var STAGE_H = PAD_TOP + DRAW_H + PAD_BOTTOM;
  var HOLD = 5200;

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
    { id: 'w-profile', d: 'M300 58 V78 H244 V96', len: 100 },
    { id: 'w-toolset', d: 'M360 58 V78 H416 V96', len: 100 },
    // Turns ABOVE the toolset's elbow rather than below it, so the two never
    // share a horizontal run. A crossing reads as a junction that is not there.
    { id: 'w-cfg', d: 'M406 58 V68 H578 V96', len: 210 },
    { id: 'w-reach', d: 'M416 142 V150', len: 20 },
    { id: 'w-conv', d: 'M82 58 V132', len: 78 },
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
      var code = pre && (pre.querySelector('code') || pre);
      var stanza = code ? code.cloneNode(true) : null;
      if (stanza) stanza.classList.add('highlight');
      // Taken out before the caption is read, so the stanza's own lines cannot
      // end up in the beat's sentence. Kept, because the reduced-motion path
      // puts the list back and a beat without its lines is a beat cut in half.
      if (holder) holder.parentNode.removeChild(holder);
      var beat = { text: li.textContent.replace(/\s+/g, ' ').trim(), stanza: stanza };
      if (holder) li.appendChild(holder);
      return beat;
    });

    var wrap = el('div', 'ao-pres');
    list.parentNode.insertBefore(wrap, list);

    var viewport = el('div', 'ao-pres-viewport', wrap);
    var stage = el('div', 'ao-pres-stage', viewport);
    var parts = buildStage(stage);

    var stanzaBox = el('div', 'ao-pres-stanza', wrap);

    var rail = el('div', 'ao-pres-rail', wrap);
    var play = el('button', 'ao-pres-play', rail);
    play.type = 'button';

    // DRAWN, NOT TYPED. `\u25b6` and `\u258c\u258c` are font glyphs, and a font
    // centres them on its own baseline and side bearings rather than in the
    // circle they sit in — which is why they looked knocked off-centre. A path
    // is positioned by its own coordinates and cannot drift with the typeface.
    //
    // The triangle's box runs 4.5 to 9.5 against a centre of 6, so it sits a
    // whisker right of true centre: a triangle's mass is at its left, and
    // centring the BOX makes it look left. The bars need no such correction.
    var PLAY_D = 'M4.5 2.6 L9.6 6 L4.5 9.4 Z';
    var PAUSE_D = 'M3.9 2.8 h1.7 v6.4 h-1.7 Z M6.9 2.8 h1.7 v6.4 h-1.7 Z';
    var glyph = svg('svg', play);
    glyph.setAttribute('viewBox', '0 0 12 12');
    glyph.setAttribute('aria-hidden', 'true');
    var glyphPath = svg('path', glyph);
    glyphPath.setAttribute('fill', 'currentColor');

    var caption = el('div', 'ao-pres-caption', rail);
    var count = el('span', 'ao-pres-count', caption);
    var text = el('span', 'ao-pres-text', caption);

    // NO LIVE REGION. One that announced every advance would interrupt a
    // screen-reader user every 6.6 seconds on the page they landed on. Each
    // dot carries its beat's whole sentence instead, so the beats stay
    // reachable — by reading them, rather than by being read at.
    wrap.setAttribute('role', 'group');
    wrap.setAttribute('aria-label', 'How it works, one beat at a time');

    var dotsBox = el('div', 'ao-pres-dots', rail);
    var progress = el('div', 'ao-pres-progress', rail);
    var bar = el('i', null, progress);

    var dots = beats.map(function (beat, i) {
      var dot = el('button', null, dotsBox);
      dot.type = 'button';
      dot.setAttribute('aria-label', 'Beat ' + (i + 1) + ': ' + beat.text);
      dot.addEventListener('click', function () {
        // A reader who has taken control has stopped watching and started
        // reading, so selecting a beat stops the advance rather than pausing it
        // for one hold.
        pause();
        goTo(i);
      });
      return dot;
    });

    var current = 0;
    var timer = null;
    var raf = null;
    var started = 0;

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

      stanzaBox.textContent = '';
      if (beats[n].stanza) stanzaBox.appendChild(beats[n].stanza.cloneNode(true));

      count.textContent = (n + 1) + ' / ' + beats.length;
      text.textContent = beats[n].text;
      dots.forEach(function (dot, i) {
        dot.setAttribute('aria-current', String(i === n));
      });
    }

    function tick() {
      bar.style.width = Math.min(1, (Date.now() - started) / HOLD) * 100 + '%';
      raf = requestAnimationFrame(tick);
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
      raf = requestAnimationFrame(tick);
      glyphPath.setAttribute('d', PAUSE_D);
      play.setAttribute('aria-label', 'Pause the presentation');
    }

    function pause() {
      clearTimeout(timer);
      cancelAnimationFrame(raf);
      timer = null;
      bar.style.width = '0%';
      glyphPath.setAttribute('d', PLAY_D);
      play.setAttribute('aria-label', 'Play the presentation');
    }

    play.addEventListener('click', function () {
      if (timer) pause(); else start();
    });

    // SCALE, NEVER REFLOW. `transform` does not reduce the stage's layout
    // width, so the viewport is given the scaled height in the same step —
    // half of that pair is a presentation with a scrollbar inside it.
    function fit() {
      var k = Math.min(1, viewport.clientWidth / STAGE_W);
      // Scale THEN drop, so the top slack scales with everything else: a
      // constant offset would over-pad the drawing at every narrow width.
      stage.style.transform = 'translate(0, ' + (PAD_TOP * k) + 'px) scale(' + k + ')';
      viewport.style.height = Math.ceil(STAGE_H * k) + 'px';
    }
    if (window.ResizeObserver) new ResizeObserver(fit).observe(viewport);
    window.addEventListener('resize', fit);
    fit();

    list.parentNode.removeChild(list);

    // REDUCED MOTION IS THE COMPOSED DRAWING PLUS THE LIST. Nothing moves, the
    // whole model is on the stage at once, and the beats stay readable in
    // order — the same content the no-scripting reader gets.
    var still = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (still) {
      goTo(beats.length - 1);
      Object.keys(parts).forEach(function (id) { parts[id].classList.add('is-on'); });
      pause();
      // The transport goes with the motion it drove. What is left is the whole
      // model on one stage and the beats as the list the page wrote — the same
      // content a reader without scripting gets.
      wrap.removeChild(stanzaBox);
      wrap.removeChild(rail);
      wrap.parentNode.insertBefore(list, wrap.nextSibling);
    } else {
      goTo(0);
      start();
    }
  });
})();
