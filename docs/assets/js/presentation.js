// The presentation, for a page that explains a model one beat at a time.
//
// The page writes an ordinary ordered list named `{: .ao-presentation}`: one
// item per beat, the item's text is that beat's caption. This reads the list,
// builds the drawing, and removes the list.
//
// TWO ADJACENT LISTS UNDER ONE PARENT ARE TWO STORIES OF ONE FIGURE, switched
// by a tablist rather than drawn as two separate pictures. `index.md` writes
// the Orchestrator's ten beats then the Pipeline's ten, in that order, under
// the same "How it works" bullet — and that order is what puts Orchestrator
// first in the tablist. A single list is still the whole single-story
// component this always was; nothing about that path changed.
//
// IT IS ONE FIGURE, AND THE FIGURE IS THE CONTROL. The drawing carries its own
// caption and clicking it pauses. There was a transport — a play button, a beat
// counter, ten scrub dots, a progress bar and a box showing each beat's
// manifest lines, in two bordered boxes under the picture. That was MORE
// MACHINERY THAN THE THING IT EXPLAINED, and the manifest it showed is already
// the strip's third panel. THE TABLIST IS NOT THAT TRANSPORT — it chooses
// which STORY is showing, not a position within one, and it sits OUTSIDE the
// clickable figure so a tab press can never also toggle playback.
//
// WITH NO SCRIPTING EACH LIST IS ITS OWN WHOLE EXPLANATION — ten beats in
// order, each carrying the lines it concerns. That is a working panel rather
// than a fallback for one, and it is the same bargain the tab strip and the
// player make. Two lists with no script are simply two such explanations, one
// after the other.
//
// REDUCED MOTION IS THE SAME FIGURE, PAUSED FROM THE START. It carries a cue
// saying it can be pressed, never a second control and never a second copy of
// the beats below it — see the branch near the end of this file for why.
//
// WHAT IS HERE AND WHAT IS THE PAGE'S. The drawing is the theme's: its shape,
// its coordinates and the connectors drawn between them are geometry, not
// prose, and no markdown shape states them. Every WORD a beat says is the
// page's, and changing one is an edit to `index.md` and to nothing else. The
// TAB LABEL is the one exception, and it is the theme's for a reason stated
// where it is declared below: "Orchestrator" names a narrative role, never
// the CRD kind, which the drawing itself still prints as `Coordinator`.
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
  //
  // BOTH STORIES SHARE THIS ONE CANVAS. The Orchestrator's own geometry below
  // was derived against these same numbers, not a second canvas of its own —
  // same DRAW_W/DRAW_H, same 660-wide wire viewBox, same caption lane.
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

  var SVG_NS = 'http://www.w3.org/2000/svg';

  // ---- per-kind shapes, mirrored from the console ----------------------
  //
  // MIRRORED, NOT LINKED, from `platform/console/ui/src/graph/shapes.tsx`
  // (`SHAPES`, `GLYPHS`, `NODE_STYLES`) — a Jekyll site has no bundler to
  // import a `.tsx` module through, so this is a committed, documented copy,
  // one-directional exactly as `agentops.css`'s palette block is: changing a
  // shape is a two-file change, made here second. Only the subset this
  // drawing's kinds need is carried over; the console's own file is longer.
  //
  // THE COORDINATOR HUB IS A DELIBERATE DEVIATION FROM THE SOURCE. The
  // console's own `NODE_STYLES.coordinators` draws `rect`, same as a
  // Pipeline — correct there, where a glyph alone already tells the kinds
  // apart. Here the two kinds are the whole story being told, side by side,
  // one beat apart, so KIND_SHAPE below overrides it to `diamond`: the
  // universal flowchart mark for a decision, which is exactly the hub's job.
  // DO NOT "FIX" THIS BACK TO `rect` ON A FUTURE SYNC OF THE TABLE BELOW —
  // the override is intentional and is reasserted right where the table is.
  var SHAPES = {
    hexagon: { d: 'M-18 0 L-9 -15 L9 -15 L18 0 L9 15 L-9 15 Z', vb: '-18 -15 36 30' },
    rect: {
      d: 'M-22 -13 h44 a3 3 0 0 1 3 3 v20 a3 3 0 0 1 -3 3 h-44 a3 3 0 0 1 -3 -3 v-20 a3 3 0 0 1 3 -3 z',
      vb: '-22 -13 44 26',
    },
    circle: { d: 'M-15 0 a15 15 0 1 0 30 0 a15 15 0 1 0 -30 0', vb: '-15 -15 30 30' },
    diamond: { d: 'M0 -17 L17 0 L0 17 L-17 0 Z', vb: '-17 -17 34 34' },
    diamond2: { d: 'M0 -17 L17 0 L0 17 L-17 0 Z M-17 0 h-4 M17 0 h4', vb: '-21 -17 42 34' },
    cylinder: { d: 'M-16 -10 a16 5 0 0 0 32 0 a16 5 0 0 0 -32 0 v20 a16 5 0 0 0 32 0 v-20', vb: '-16 -15 32 30' },
    note: { d: 'M-13 -15 h18 l8 8 v22 h-26 z M5 -15 v8 h8', vb: '-13 -15 26 30' },
  };

  var GLYPHS = {
    source: 'M2 -7 L-4 1 h4 l-1 6 6 -8 h-4 z',
    pipeline: 'M-8 -4 h5 a3 3 0 0 1 3 3 v2 a3 3 0 0 0 3 3 h5 M6 2 l2 2 -2 2',
    // A Coordinator fans one thread into several — a trunk branching into
    // three, distinct from the pipeline glyph's single feed-through arrow.
    coordinator: 'M-7 0 h4 M-3 0 l5 -6 M-3 0 l5 0 M-3 0 l5 6',
    profile: 'M0 -6 a3 3 0 1 1 0 6 a3 3 0 0 1 0 -6 M-6 7 a6 6 0 0 1 12 0 z',
    // A capability is a bundle to be REFERENCED, never wired directly — a
    // key, never a person (the profile glyph): a small ring and a shaft.
    agentCapability: 'M-3 -7 a2 2 0 1 0 0.01 0 M-3 -5 v9 M-3 2 h3 M-3 5 h2',
    toolset: 'M5 -5 a3 3 0 0 1 -4 4 l-5 5 -1 -1 5 -5 a3 3 0 0 1 4 -4 z',
    mcpconfig: 'M-6 -5 h12 v4 h-12 z M-6 1 h12 v4 h-12 z',
    channel: 'M-6 -3 h12 v8 h-6 l-3 3 v-3 h-3 z',
    conversation: 'M-5 -2 h10 M-5 2 h6',
  };

  // Which outline each kind draws. Matches `NODE_STYLES` in shapes.tsx
  // exactly, with the ONE named exception above.
  var KIND_SHAPE = {
    source: 'hexagon',
    pipeline: 'rect',
    coordinator: 'diamond', // deliberate: shapes.tsx itself draws `rect` here
    profile: 'circle',
    agentCapability: 'circle',
    toolset: 'diamond',
    mcpconfig: 'diamond2',
    channel: 'cylinder',
    conversation: 'note',
  };

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

  function pos(node, r) {
    node.style.left = r.left + 'px';
    node.style.top = r.top + 'px';
    node.style.width = r.width + 'px';
    node.style.height = r.height + 'px';
  }

  function pos3(node, r) {
    node.style.left = r.left + 'px';
    node.style.top = r.top + 'px';
    node.style.width = r.width + 'px';
  }

  // The node's OUTLINE: an absolutely-positioned SVG, stretched non-uniformly
  // to fill the slot. "meet" (uniform scale, centred) was tried and measured
  // broken: shrinking the shape to its true proportions left text up to 3x
  // wider than the shape sat in. "none" (fill exactly) is what is actually
  // verified clean — the shape IS the box, so text sized for the box always
  // fits inside it. The shapes read as compact or wide rather than
  // textbook-proportioned, and that is the deliberate tradeoff.
  function shapeLayer(parent, kind) {
    var shape = SHAPES[KIND_SHAPE[kind]];
    var wrap = el('div', 'ao-pres-shape', parent);
    var s = svg('svg', wrap);
    s.setAttribute('viewBox', shape.vb);
    s.setAttribute('preserveAspectRatio', 'none');
    s.setAttribute('width', '100%');
    s.setAttribute('height', '100%');
    s.setAttribute('aria-hidden', 'true');
    var p = svg('path', s);
    p.setAttribute('d', shape.d);
  }

  // The small inline mark beside each kind label is the glyph ALONE, no
  // outline — the node's own shape (drawn by shapeLayer) is the outline now,
  // so repeating it at 12px would just be a second, illegible copy.
  function glyphSpan(parent, kind) {
    var g = el('span', 'ao-pres-glyph', parent);
    var s = svg('svg', g);
    s.setAttribute('viewBox', '-10 -10 20 20');
    s.setAttribute('width', '100%');
    s.setAttribute('height', '100%');
    s.setAttribute('aria-hidden', 'true');
    var p = svg('path', s);
    p.setAttribute('d', GLYPHS[kind]);
  }

  // ---- the two stories ---------------------------------------------------
  //
  // Each is the drawing's full geometry: the install frame, the declared
  // objects (with the shape each one's icon draws), the conversation it
  // opens, the reach the wiring granted, and the connectors between them.
  // `script` is keyed by POSITION against the matching list's beats, exactly
  // as the single-story version always kept SCRIPT keyed against `beats`: a
  // page writing more beats than there are entries simply gets its captions
  // with nothing new brought forward.

  // Pipeline: the real site's own geometry, carried over close to verbatim —
  // every `rect` and every wire `d` here is the value this file and
  // `agentops.css` already shipped, now stated in one place instead of split
  // across a script and a stylesheet so a shape layer has a height to
  // stretch into (implicit/auto before).
  var PIPELINE = {
    tabLabel: 'Pipeline',
    frame: { left: 212, top: -16, width: 456, height: 261, label: 'your cluster · one Helm install' },
    nodes: [
      { id: 'source', kind: 'SignalSource', name: 'cluster-events', icon: 'source', rect: { left: 0, top: 8, width: 164, height: 56 } },
      { id: 'pipeline', kind: 'Pipeline', name: 'k8s-ops', icon: 'pipeline', rect: { left: 248, top: 2, width: 164, height: 56 } },
      { id: 'channel', kind: 'Channel', name: 'telegram', icon: 'channel', rect: { left: 496, top: 8, width: 164, height: 56 } },
      { id: 'profile', kind: 'AgentProfile', name: 'k8s-engineer', icon: 'profile', rect: { left: 168, top: 118, width: 152, height: 56 } },
      { id: 'toolset', kind: 'MCPToolset', name: 'agentops-observe', icon: 'toolset', rect: { left: 340, top: 118, width: 152, height: 56 } },
      { id: 'mcpconfig', kind: 'MCPConfig', name: 'k8s-api', icon: 'mcpconfig', rect: { left: 502, top: 118, width: 152, height: 56 } },
    ],
    reach: {
      left: 340, top: 186, width: 314,
      chips: [
        { text: 'read pods, events, logs', granted: true },
        { text: 'query metrics', granted: true },
        { text: 'delete anything', granted: false },
        { text: 'a shell', granted: false },
      ],
    },
    conv: { left: 0, top: 150, width: 160, height: 78 },
    convIcon: 'conversation',
    convName: 'cluster-events-7c1d4e',
    wires: [
      { id: 'w-source', d: 'M164 30 H248' },
      { id: 'w-channel', d: 'M412 30 H496' },
      { id: 'w-profile', d: 'M300 58 V96 H244 V118' },
      { id: 'w-toolset', d: 'M360 58 V96 H416 V118' },
      // Turns ABOVE the toolset's elbow rather than below it, so the two
      // never share a horizontal run. A crossing reads as a junction that is
      // not there.
      { id: 'w-cfg', d: 'M406 58 V84 H578 V118' },
      { id: 'w-reach', d: 'M416 163 V186' },
      { id: 'w-conv', d: 'M82 58 V150' },
    ],
    script: [
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
    ],
  };

  // Orchestrator: the human-facing label for THIS narrative only — see the
  // header comment. The drawing's own hub still prints the kind it is:
  // `Coordinator`. A hub, a trunk, a branch per member; escalate is its own
  // path straight off the root, separate from every member branch, and
  // reached only when the root itself decides to — never the automatic wire
  // a Pipeline's `channelRefs` draws.
  var ORCHESTRATOR = {
    tabLabel: 'Orchestrator',
    frame: { left: 212, top: -16, width: 456, height: 261, label: 'your cluster · one Helm install' },
    nodes: [
      { id: 'source', kind: 'SignalSource', name: 'cluster-events', icon: 'source', rect: { left: 0, top: 8, width: 164, height: 56 } },
      { id: 'hub', kind: 'Coordinator', name: 'k8s-triage', icon: 'coordinator', rect: { left: 222, top: 80, width: 130, height: 72 } },
      { id: 'channel', kind: 'Channel', name: 'telegram', icon: 'channel', rect: { left: 480, top: 4, width: 150, height: 48 } },
      { id: 'member1', kind: 'AgentCapability', name: 'log-reader', icon: 'agentCapability', rect: { left: 480, top: 60, width: 150, height: 48 } },
      { id: 'member2', kind: 'AgentCapability', name: 'remediator', icon: 'agentCapability', rect: { left: 480, top: 116, width: 150, height: 48 } },
      { id: 'member3', kind: 'Coordinator', name: 'home-triage', icon: 'coordinator', rect: { left: 480, top: 172, width: 150, height: 48 } },
    ],
    reach: {
      left: 222, top: 165, width: 210,
      chips: [
        { text: 'read, investigate', granted: true },
        { text: 'propose a fix', granted: true },
        { text: 'act without a reply', granted: false },
        { text: 'skip the human', granted: false },
      ],
    },
    conv: { left: 0, top: 150, width: 160, height: 78 },
    convIcon: 'conversation',
    convName: 'cluster-events-9a2f1c',
    wires: [
      { id: 'w-source', d: 'M164 36 H190 V116 H222' },
      { id: 'w-channel', d: 'M287 80 V28 H480' },
      { id: 'w-member1', d: 'M352 116 H420 V84 H480' },
      { id: 'w-member2', d: 'M352 116 H420 V140 H480' },
      { id: 'w-member3', d: 'M352 116 H420 V196 H480' },
      { id: 'w-reach', d: 'M287 152 V165' },
      { id: 'w-conv', d: 'M287 152 V150 H80' },
    ],
    script: [
      { on: ['source'], lit: ['source'] },
      { on: ['frame', 'hub'], lit: [] },
      { on: [], lit: ['hub'] },
      { on: ['w-source'], lit: ['source', 'w-source'] },
      { on: ['member1', 'w-member1'], lit: ['member1', 'w-member1'] },
      { on: ['member2', 'reach', 'w-member2', 'w-reach'], lit: ['member2', 'w-member2'] },
      { on: ['member3', 'w-member3'], lit: ['member3', 'w-member3'] },
      { on: ['channel', 'w-channel'], lit: ['channel', 'w-channel'] },
      { on: ['conv', 'w-conv'], lit: ['conv', 'w-conv'] },
      { on: [], lit: [] },
    ],
  };

  // Tab order: ORCHESTRATOR first, Pipeline second — the array order IS the
  // tablist order, and it is also the order `index.md` writes its two lists
  // in, which is what pairs each list's beats to the right story below.
  var STORIES = [ORCHESTRATOR, PIPELINE];

  /** The drawing, fully laid out and unemphasised. Returns its elements by id. */
  function buildStage(stage, story) {
    var parts = {};

    var frame = el('div', 'ao-pres-frame', stage);
    pos(frame, story.frame);
    el('span', null, frame).textContent = story.frame.label;
    parts.frame = frame;

    var wire = svg('svg', stage);
    wire.setAttribute('class', 'ao-pres-wire');
    wire.setAttribute('viewBox', '0 0 660 ' + DRAW_H);
    wire.setAttribute('aria-hidden', 'true');
    story.wires.forEach(function (w) {
      var track = svg('path', wire);
      track.setAttribute('class', 'ao-pres-track');
      track.setAttribute('d', w.d);
    });
    story.wires.forEach(function (w) {
      var path = svg('path', wire);
      path.setAttribute('class', 'ao-pres-draw');
      path.setAttribute('d', w.d);
      // The dash-reveal length comes from the PATH ITSELF, never a typed
      // number: a hand-guessed length is exactly what cut a connector short
      // of its target box before — `getTotalLength()` cannot drift from the
      // path it measures, for either story.
      path.style.setProperty('--ao-pres-len', path.getTotalLength());
      parts[w.id] = path;
    });

    story.nodes.forEach(function (n) {
      var node = el('div', 'ao-pres-node', stage);
      node.setAttribute('data-node', n.id);
      pos(node, n.rect);
      shapeLayer(node, n.icon);
      var content = el('div', 'ao-pres-content', node);
      var head = el('div', 'ao-pres-head', content);
      glyphSpan(head, n.icon);
      el('span', 'ao-pres-kind', head).textContent = n.kind;
      el('span', 'ao-pres-name', content).textContent = n.name;
      parts[n.id] = node;
    });

    var reach = el('div', 'ao-pres-reach', stage);
    pos3(reach, story.reach);
    story.reach.chips.forEach(function (r) {
      var chip = el('span', 'ao-pres-chip ' + (r.granted ? 'is-granted' : 'is-denied'), reach);
      chip.textContent = r.text;
    });
    parts.reach = reach;

    var conv = el('div', 'ao-pres-conv', stage);
    pos(conv, story.conv);
    shapeLayer(conv, story.convIcon);
    var cc = el('div', 'ao-pres-content', conv);
    var kind = el('span', 'ao-pres-kind', cc);
    glyphSpan(kind, story.convIcon);
    el('span', 'ao-pres-dot', kind);
    kind.appendChild(document.createTextNode('Conversation · running'));
    el('span', 'ao-pres-name', cc).textContent = story.convName;
    el('i', null, cc);
    el('i', null, cc);
    parts.conv = conv;

    return parts;
  }

  // Lists grouped by their shared parent: two (or more) `{: .ao-presentation}`
  // lists sitting under the same bullet are ONE figure with several stories,
  // matched to STORIES by position. A lone list under its own parent is the
  // single-story component this always was.
  var groups = [];
  lists.forEach(function (list) {
    var g = null;
    for (var i = 0; i < groups.length; i++) {
      if (groups[i].parent === list.parentNode) { g = groups[i]; break; }
    }
    if (!g) { g = { parent: list.parentNode, lists: [] }; groups.push(g); }
    g.lists.push(list);
  });

  groups.forEach(function (group) {
    var groupLists = group.lists;
    var storyCount = Math.min(groupLists.length, STORIES.length);
    if (!storyCount) return;

    // The caption is each item's own text, and the stanza is whatever fenced
    // block the item carries. Reading them out before anything is built keeps
    // the page the single source of both.
    var tooShort = groupLists.slice(0, storyCount).some(function (list) {
      return [].slice.call(list.children).filter(function (li) {
        return li.tagName === 'LI';
      }).length < 2;
    });
    if (tooShort) return;
    var beatsByStory = groupLists.slice(0, storyCount).map(function (list) {
      var items = [].slice.call(list.children).filter(function (li) {
        return li.tagName === 'LI';
      });
      return items.map(function (li) {
        var pre = li.querySelector('pre');
        var holder = pre && (pre.closest('div[class*="language-"]') || pre);
        // The <code>, not the <pre>. `copy.js` puts a control on every <pre>
        // it finds, and a copy button on a three-line excerpt that changes
        // under the reader offers the wrong thing — the manifest to copy is
        // the strip's third panel, whole. Taken out before the caption is
        // read, so the block's own lines cannot end up in the beat's
        // sentence.
        if (holder) holder.parentNode.removeChild(holder);
        return { text: li.textContent.replace(/\s+/g, ' ').trim() };
      });
    });
    var anchorList = groupLists[0];
    var multi = storyCount > 1;

    var shell = el('div', 'ao-pres-shell');
    anchorList.parentNode.insertBefore(shell, anchorList);

    var tablist = null;
    var tabs = [];
    var panelId = 'ao-pres-panel-' + groups.indexOf(group);
    if (multi) {
      tablist = el('div', 'ao-tablist', shell);
      tablist.setAttribute('role', 'tablist');
      tablist.setAttribute('aria-label', 'Wiring story');
      for (var si = 0; si < storyCount; si++) {
        (function (i) {
          var btn = el('button', 'ao-tab' + (i === 0 ? ' is-current' : ''), tablist);
          btn.type = 'button';
          btn.setAttribute('role', 'tab');
          btn.setAttribute('aria-selected', i === 0 ? 'true' : 'false');
          btn.setAttribute('aria-controls', panelId);
          btn.tabIndex = i === 0 ? 0 : -1;
          btn.textContent = STORIES[i].tabLabel;
          tabs.push(btn);
          btn.addEventListener('click', function () { select(i); });
          // WAI-ARIA tabs: arrows move between tabs, and only the current
          // tab is in the Tab order.
          btn.addEventListener('keydown', function (e) {
            var to = -1;
            if (e.key === 'ArrowRight') to = (i + 1) % storyCount;
            else if (e.key === 'ArrowLeft') to = (i - 1 + storyCount) % storyCount;
            else if (e.key === 'Home') to = 0;
            else if (e.key === 'End') to = storyCount - 1;
            if (to < 0) return;
            e.preventDefault();
            select(to);
            tabs[to].focus();
          });
        })(si);
      }
    }

    var wrap = el('div', 'ao-pres', shell);
    var viewport = el('div', 'ao-pres-viewport', wrap);
    var stage = el('div', 'ao-pres-stage', viewport);

    groupLists.forEach(function (list) { list.parentNode.removeChild(list); });

    var BASE_LABEL = 'How it works, one beat at a time';
    wrap.id = panelId;
    wrap.setAttribute('role', multi ? 'tabpanel' : 'group');
    wrap.setAttribute('aria-label', BASE_LABEL);
    wrap.tabIndex = 0;

    var text = el('div', 'ao-pres-caption', stage);

    var storyIndex = 0;
    var parts = {};
    var beats = beatsByStory[0];
    var script = STORIES[0].script;
    var current = 0;
    var timer = null;
    var stillNow = false;
    var reduced = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    /** The element a beat is about: what it LIGHTS, else what it turns on.
        Never the frame — it is the whole drawing, so "beside it" is nowhere,
        and never a connector, which has no box to sit beside. */
    function anchorFor(n) {
      var beat = script[n] || {};
      var ids = (beat.lit || []).concat(beat.on || []);
      for (var i = 0; i < ids.length; i++) {
        var e = parts[ids[i]];
        if (e && e.offsetWidth && !e.classList.contains('ao-pres-frame')) return e;
      }
      // A beat about the whole picture — the install, and the closing line.
      return parts[STORIES[storyIndex].nodes[1].id];
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
      for (var i = 0; i <= n && i < script.length; i++) {
        script[i].on.forEach(function (id) { on[id] = true; });
      }
      if (script[n]) script[n].lit.forEach(function (id) { lit[id] = true; });

      Object.keys(parts).forEach(function (id) {
        parts[id].classList.toggle('is-on', !!on[id]);
        parts[id].classList.toggle('is-lit', !!lit[id]);
      });

      text.textContent = beats[n].text;
      place(n);
    }

    function advance() {
      goTo((current + 1) % beats.length);
      timer = setTimeout(advance, HOLD);
    }

    function start() {
      if (timer) return;
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
    // a request to pause, so neither is intercepted. The TABLIST sits outside
    // this element entirely, so a tab press never reaches this handler at all
    // — it has its own click listener, above, and nothing to opt out of.
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
    // leaves the ordinary paused figure, exactly as it would for any other
    // reader who has already said what they want.
    function engage() {
      stillNow = false;
      wrap.classList.remove('is-still');
      wrap.setAttribute('aria-label', BASE_LABEL);
      goTo(0);
      start();
    }

    function fit() {
      // FILLS THE COLUMN. Capped at 1 while the column was 45rem and the
      // stage was authored to just fit it. At 58rem the cap left the drawing
      // at 668px pinned to the left of a 928px box. Unbounded is safe because
      // the COLUMN is bounded by `--ao-measure-wide`.
      var k = viewport.clientWidth / STAGE_W;
      stage.style.transform = 'translate(0, ' + (PAD_TOP * k) + 'px) scale(' + k + ')';
      viewport.style.height = Math.ceil(STAGE_H * k) + 'px';
    }
    if (window.ResizeObserver) new ResizeObserver(fit).observe(viewport);
    window.addEventListener('resize', fit);

    /** (Re)build the stage for story `i` and start it from beat 0 — the same
        entry state a fresh mount uses, whether this is the first story shown
        or a tab switch away from another one. */
    function mount(i) {
      storyIndex = i;
      beats = beatsByStory[i];
      script = STORIES[i].script;
      stage.innerHTML = '';
      parts = buildStage(stage, STORIES[i]);
      text = el('div', 'ao-pres-caption', stage);

      clearTimeout(timer);
      timer = null;
      fit();

      if (reduced) {
        goTo(beats.length - 1);
        Object.keys(parts).forEach(function (id) { parts[id].classList.add('is-on'); });
        pause();
        stillNow = true;
        wrap.classList.add('is-still');
        wrap.setAttribute('aria-label', BASE_LABEL + ' — press to play');
      } else {
        stillNow = false;
        wrap.classList.remove('is-still');
        goTo(0);
        start();
      }
    }

    function select(i) {
      if (i === storyIndex) return;
      tabs.forEach(function (btn, bi) {
        btn.classList.toggle('is-current', bi === i);
        btn.setAttribute('aria-selected', bi === i ? 'true' : 'false');
        btn.tabIndex = bi === i ? 0 : -1;
      });
      mount(i);
    }

    mount(0);

    // REDUCED MOTION IS THE COMPOSED DRAWING, PAUSED, WITH A CUE INSTEAD OF A
    // SECOND CONTROL. Nothing moves, and the whole model is on the stage at
    // once — the preference says do not move things AT me, it does not say
    // never let me ask, so DELETING the ability to play at all is still
    // wrong. BUT ASKING IS THE SAME CLICK EVERY OTHER READER ALREADY HAS —
    // `toggle()` calls `engage()` exactly when `stillNow` is true — so there
    // is no separate control to keep alive here, only a cue that one exists.
    // `mount()` above applies this on every story, including a tab switch.
  });
})();
