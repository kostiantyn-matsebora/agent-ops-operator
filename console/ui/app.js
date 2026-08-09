'use strict';
// agent-ops console SPA.
//
// Two invariants keep this file small and safe:
//   1. Snapshots decide what is on screen; the SSE stream only says "something
//      changed, re-read". A missed event therefore costs one stale second, and
//      reconnect-after-sleep is the same code path as first load.
//   2. Everything from the cluster or the wire is inserted as TEXT. Agent output
//      arrives as the chat HTML subset the manager renders for transports; we
//      flatten it to plain text rather than trusting a tag whitelist in a page
//      that also shows cluster state.

const $ = (sel) => document.querySelector(sel);
const el = (tag, cls, text) => {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text !== undefined) n.textContent = text;
  return n;
};

const state = {
  view: 'topology',
  topology: null,
  consoleChannel: '',
  unjoined: [],
  kind: 'pipelines',
  rows: [],
  selectedNode: null,
  selectedCR: null,
  conversations: [],
  conversation: null,
  transcript: [],
  stream: null,
};

// ---- api --------------------------------------------------------------------

async function api(path, opts) {
  const res = await fetch(path, Object.assign({ headers: { 'Content-Type': 'application/json' } }, opts));
  if (res.status === 401) { showLogin(); throw new Error('unauthenticated'); }
  if (!res.ok) {
    let msg = res.statusText;
    try { msg = (await res.json()).error || msg; } catch (_) {}
    throw new Error(msg);
  }
  return res.status === 204 ? null : res.json();
}

// ---- chat text --------------------------------------------------------------

// flatten turns the manager's chat-HTML subset into plain text. The console
// renders no markup from the wire; when messages become semantic (the pending
// adapter-rendered-messages change) this stops being a guess.
function flatten(text) {
  const noTags = String(text == null ? '' : text).replace(/<[^>]*>/g, '');
  const map = { '&lt;': '<', '&gt;': '>', '&amp;': '&', '&quot;': '"', '&#39;': "'" };
  return noTags.replace(/&(lt|gt|amp|quot|#39);/g, (m) => map[m]);
}

// ---- login ------------------------------------------------------------------

function showLogin(hint) {
  $('#login').classList.remove('hidden');
  $('#app').classList.add('hidden');
  if (hint) $('#login-hint').textContent = hint;
  if (state.stream) { state.stream.close(); state.stream = null; }
}

function showApp() {
  $('#login').classList.add('hidden');
  $('#app').classList.remove('hidden');
  connectStream();
  refresh();
}

$('#login-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  $('#login-error').textContent = '';
  try {
    await api('/api/login', { method: 'POST', body: JSON.stringify({ token: $('#token').value }) });
    $('#token').value = '';
    showApp();
  } catch (err) {
    $('#login-error').textContent = err.message;
  }
});

$('#logout').addEventListener('click', async () => {
  try { await api('/api/logout', { method: 'POST' }); } catch (_) {}
  showLogin();
});

// ---- navigation -------------------------------------------------------------

document.querySelectorAll('header nav button').forEach((b) => {
  b.addEventListener('click', () => {
    state.view = b.dataset.view;
    document.querySelectorAll('header nav button').forEach((x) => x.classList.toggle('active', x === b));
    ['topology', 'inventory', 'conversations'].forEach((v) => {
      $('#view-' + v).classList.toggle('hidden', v !== state.view);
    });
    refresh();
  });
});

// ---- stream -----------------------------------------------------------------

let refreshTimer = null;
function scheduleRefresh() {
  if (refreshTimer) return;
  refreshTimer = setTimeout(() => { refreshTimer = null; refresh(); }, 250);
}

function connectStream() {
  if (state.stream) state.stream.close();
  const es = new EventSource('/api/stream');
  state.stream = es;
  es.addEventListener('resync', scheduleRefresh);
  es.addEventListener('delta', scheduleRefresh);
  es.addEventListener('message', (e) => {
    const msg = JSON.parse(e.data);
    if (!state.conversation || msg.thread !== state.conversation.consoleThread) return;
    const idx = state.transcript.findIndex((m) => m.id === msg.id);
    if (idx >= 0) state.transcript[idx] = msg; else state.transcript.push(msg);
    renderTranscript();
  });
  es.onopen = () => { $('#status').textContent = 'live'; };
  es.onerror = () => {
    $('#status').textContent = 'reconnecting…';
    // EventSource retries on its own; the reconnect emits `resync`, which
    // re-reads the snapshot — no client-side event replay to get wrong.
  };
}

// ---- refresh ----------------------------------------------------------------

async function refresh() {
  try {
    if (state.view === 'topology') await loadTopology();
    if (state.view === 'inventory') await loadInventory();
    if (state.view === 'conversations') await loadConversations();
  } catch (err) {
    if (err.message !== 'unauthenticated') $('#status').textContent = err.message;
  }
}

// ---- topology ---------------------------------------------------------------

const COLUMNS = [
  ['signaladapters', 'signal adapters'],
  ['signalsources', 'sources'],
  ['pipelines', 'pipelines'],
  ['agentprofiles', 'profiles'],
  ['channels', 'channels'],
  ['channeladapters', 'channel adapters'],
];

async function loadTopology() {
  const data = await api('/api/topology');
  state.topology = data.topology;
  state.consoleChannel = data.consoleChannel || '';
  state.unjoined = data.unjoinedPipelines || [];
  renderGraph();
  renderNodeDetail();
}

function renderGraph() {
  const host = $('#graph');
  host.textContent = '';
  const nodes = state.topology.nodes || [];
  const edges = state.topology.edges || [];
  if (!nodes.length) {
    host.appendChild(el('p', 'notice', 'No agentops resources found in this namespace yet.'));
    return;
  }

  const W = 170, H = 46, GAPX = 90, GAPY = 22, TOP = 34, LEFT = 10;
  const pos = {};
  const perColumn = COLUMNS.map(([kind]) => nodes.filter((n) => n.kind === kind));
  perColumn.forEach((list, col) => {
    list.forEach((n, i) => {
      pos[n.id] = { x: LEFT + col * (W + GAPX), y: TOP + i * (H + GAPY) };
    });
  });
  const rows = Math.max(1, ...perColumn.map((l) => l.length));
  const width = LEFT + COLUMNS.length * (W + GAPX);
  const height = TOP + rows * (H + GAPY) + 20;

  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('width', width);
  svg.setAttribute('height', height);
  const svgEl = (tag, attrs, cls) => {
    const n = document.createElementNS('http://www.w3.org/2000/svg', tag);
    for (const k in attrs) n.setAttribute(k, attrs[k]);
    if (cls) n.setAttribute('class', cls);
    return n;
  };

  COLUMNS.forEach(([, title], col) => {
    const t = svgEl('text', { x: LEFT + col * (W + GAPX), y: 16 }, 'col-title');
    t.textContent = title;
    svg.appendChild(t);
  });

  edges.forEach((e) => {
    const a = pos[e.from], b = pos[e.to];
    if (!a || !b) return;
    const x1 = a.x + W, y1 = a.y + H / 2, x2 = b.x, y2 = b.y + H / 2;
    const back = x2 < x1; // served-by edges on the signal side point left
    const d = back
      ? `M ${a.x} ${y1} C ${a.x - 40} ${y1}, ${b.x + W + 40} ${y2}, ${b.x + W} ${y2}`
      : `M ${x1} ${y1} C ${x1 + 45} ${y1}, ${x2 - 45} ${y2}, ${x2} ${y2}`;
    svg.appendChild(svgEl('path', { d }, 'edge' + (e.dangling ? ' dangling' : '')));
  });

  nodes.forEach((n) => {
    const p = pos[n.id];
    if (!p) return;
    const g = svgEl('g', { transform: `translate(${p.x},${p.y})` },
      'node ' + n.health + (n.detached ? ' detached' : ''));
    g.appendChild(svgEl('rect', { width: W, height: H }));
    const kindText = svgEl('text', { x: 10, y: 17 }, 'kind');
    kindText.textContent = n.kind === 'signaladapters' || n.kind === 'channeladapters' ? 'adapter' : n.kind.replace(/s$/, '');
    g.appendChild(kindText);
    const nameText = svgEl('text', { x: 10, y: 34 });
    nameText.textContent = n.name;
    g.appendChild(nameText);
    if (n.kind === 'pipelines' && (n.active || n.recent)) {
      const badge = svgEl('text', { x: W - 10, y: 17, 'text-anchor': 'end' }, 'badge');
      badge.textContent = n.active ? `▶ ${n.active}` : `· ${n.recent}`;
      g.appendChild(badge);
    }
    g.addEventListener('click', () => { state.selectedNode = n; renderNodeDetail(); });
    svg.appendChild(g);
  });

  host.appendChild(svg);
}

function renderNodeDetail() {
  const box = $('#node-detail');
  box.textContent = '';
  const n = state.selectedNode;
  if (!n) {
    box.appendChild(el('h2', null, 'Topology'));
    box.appendChild(el('p', 'sub', 'Select a node to inspect it.'));
    if (state.consoleChannel) {
      box.appendChild(el('p', 'sub', 'Console channel: ' + state.consoleChannel));
    }
    if (state.unjoined.length) {
      box.appendChild(el('h2', null, 'Pipelines not joined'));
      box.appendChild(el('p', 'sub',
        'These pipelines do not post to the console channel, so their conversations are observed only.'));
      state.unjoined.forEach((p) => box.appendChild(el('div', 'row', p)));
      box.appendChild(el('p', 'sub', 'To join one, add the console channel to its wiring:'));
      box.appendChild(el('pre', null,
        `kubectl patch pipeline <name> --type=json \\\n  -p '[{"op":"add","path":"/spec/channelRefs/-","value":{"name":"${state.consoleChannel || 'console'}"}}]'`));
    }
    return;
  }
  box.appendChild(el('h2', null, n.name));
  box.appendChild(el('p', 'sub', (Singular(n.kind) || n.kind) + (n.detached ? ' · not referenced by any pipeline' : '')));
  if (n.reason) {
    const c = el('div', 'cond bad');
    c.appendChild(el('div', 't', n.reason));
    if (n.message) c.appendChild(el('div', 'm', n.message));
    box.appendChild(c);
  }
  if (n.kind === 'pipelines') {
    box.appendChild(el('p', 'sub', `${n.active} active · ${n.recent} conversation(s)`));
  }
  const open = el('button', 'link', 'Open resource');
  open.addEventListener('click', () => selectCR(n.kind, n.name));
  box.appendChild(open);
}

function Singular(kind) {
  return {
    agentprofiles: 'AgentProfile', agentruntimes: 'AgentRuntime', channels: 'Channel',
    channeladapters: 'ChannelAdapter', conversations: 'Conversation', pipelines: 'Pipeline',
    signaladapters: 'SignalAdapter', signalsources: 'SignalSource',
  }[kind];
}

// ---- inventory --------------------------------------------------------------

async function loadInventory() {
  const kinds = await api('/api/kinds');
  const list = $('#kind-list');
  list.textContent = '';
  kinds.forEach((k) => {
    const li = el('li', k.kind === state.kind ? 'active' : '');
    li.appendChild(el('span', null, k.title));
    li.appendChild(el('span', 'count', String(k.count)));
    li.addEventListener('click', () => { state.kind = k.kind; state.selectedCR = null; loadInventory(); });
    list.appendChild(li);
  });
  state.rows = await api('/api/kinds/' + state.kind);
  const rows = $('#inventory-rows');
  rows.textContent = '';
  if (!state.rows.length) rows.appendChild(el('p', 'notice', 'No ' + Singular(state.kind) + ' resources.'));
  state.rows.forEach((r) => {
    const row = el('div', 'row');
    const name = el('div', 'name');
    name.appendChild(el('span', 'dot ' + r.health));
    name.appendChild(document.createTextNode(r.name));
    row.appendChild(name);
    if (r.summary) row.appendChild(el('div', 'meta', r.summary));
    const bad = (r.conditions || []).find((c) => c.status !== 'True');
    if (bad) row.appendChild(el('div', 'meta', bad.type + '=' + bad.status + ' · ' + (bad.reason || '')));
    row.addEventListener('click', () => selectCR(state.kind, r.name));
    rows.appendChild(row);
  });
  renderCRDetail();
}

async function selectCR(kind, name) {
  state.kind = kind;
  state.view = 'inventory';
  document.querySelectorAll('header nav button').forEach((x) => x.classList.toggle('active', x.dataset.view === 'inventory'));
  ['topology', 'inventory', 'conversations'].forEach((v) => $('#view-' + v).classList.toggle('hidden', v !== 'inventory'));
  state.selectedCR = await api(`/api/kinds/${kind}/${encodeURIComponent(name)}`);
  await loadInventory();
}

function renderCRDetail() {
  const box = $('#cr-detail');
  box.textContent = '';
  const o = state.selectedCR;
  if (!o) {
    box.appendChild(el('h2', null, 'Resource'));
    box.appendChild(el('p', 'sub', 'Select a resource to see its spec, status and conditions.'));
    return;
  }
  box.appendChild(el('h2', null, o.metadata.name));
  box.appendChild(el('p', 'sub', Singular(o.kind) + ' · created ' + (o.metadata.creationTimestamp || '—')));
  const conds = (o.status && o.status.conditions) || [];
  conds.forEach((c) => {
    const div = el('div', 'cond ' + (c.status === 'True' ? 'ok' : 'bad'));
    div.appendChild(el('div', 't', c.type + ' = ' + c.status + (c.reason ? ' (' + c.reason + ')' : '')));
    if (c.message) div.appendChild(el('div', 'm', c.message));
    box.appendChild(div);
  });
  box.appendChild(el('h2', null, 'spec'));
  box.appendChild(el('pre', null, JSON.stringify(o.spec || {}, null, 2)));
  box.appendChild(el('h2', null, 'status'));
  box.appendChild(el('pre', null, JSON.stringify(o.status || {}, null, 2)));
}

// ---- conversations ----------------------------------------------------------

async function loadConversations() {
  const page = await api('/api/conversations');
  state.conversations = page.items || [];
  const list = $('#conv-list');
  list.textContent = '';
  if (!state.conversations.length) list.appendChild(el('li', null, 'No conversations yet.'));
  if (page.total > page.shown) {
    // say what was left out rather than quietly showing a prefix
    list.appendChild(el('li', 'meta', `showing ${page.shown} most recent of ${page.total}`));
  }
  state.conversations.forEach((c) => {
    const li = el('li', state.conversation && state.conversation.name === c.name ? 'active' : '');
    li.appendChild(el('div', null, c.title || c.name));
    const bits = [c.phase || 'Idle'];
    if (c.pipeline) bits.push(c.pipeline); else bits.push('unattributed');
    bits.push(c.joined ? 'joined' : 'observed');
    li.appendChild(el('div', 'meta', bits.join(' · ')));
    li.addEventListener('click', () => openConversation(c.name));
    list.appendChild(li);
  });
  if (state.conversation) await openConversation(state.conversation.name, true);
  else renderConversation();
}

async function openConversation(name, quiet) {
  const data = await api('/api/conversations/' + encodeURIComponent(name));
  state.conversation = data.conversation;
  state.transcript = data.transcript || [];
  renderConversation();
  if (!quiet) loadConversations();
}

function renderConversation() {
  const box = $('#conv-detail');
  box.textContent = '';
  const c = state.conversation;
  if (!c) {
    box.appendChild(el('p', 'notice', 'Select a conversation.'));
    return;
  }
  const head = el('div', 'head');
  head.appendChild(el('h2', null, c.title || c.name));
  const meta = [c.phase || 'Idle', 'profile ' + (c.profile || '—')];
  if (c.pipeline) meta.push('pipeline ' + c.pipeline);
  if (c.runtimePod) meta.push(c.runtimePod);
  if (c.inflight) meta.push('running ' + c.inflight.runId);
  head.appendChild(el('div', 'meta', meta.join(' · ')));
  box.appendChild(head);

  const scroll = el('div', 'scroll');
  scroll.id = 'conv-scroll';
  box.appendChild(scroll);

  if (c.joined) {
    const form = el('form', 'composer');
    const input = el('input');
    input.placeholder = 'Reply to the agent…';
    const button = el('button', null, 'Send');
    button.type = 'submit';
    form.appendChild(input);
    form.appendChild(button);
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const text = input.value.trim();
      if (!text) return;
      input.value = '';
      try {
        const msg = await api(`/api/conversations/${encodeURIComponent(c.name)}/messages`,
          { method: 'POST', body: JSON.stringify({ text }) });
        if (msg) { state.transcript.push(msg); renderTranscript(); }
      } catch (err) {
        $('#status').textContent = err.message;
      }
    });
    box.appendChild(form);
  } else {
    const notice = el('div', 'notice');
    notice.appendChild(el('div', null,
      'Observed only — this conversation has no console thread, so there is nothing to reply to here.'));
    notice.appendChild(el('div', null,
      'Add the console channel to its pipeline’s channels[] to join it; existing conversations keep their current bindings.'));
    box.appendChild(notice);
  }
  renderTranscript();
}

function renderTranscript() {
  const scroll = $('#conv-scroll');
  if (!scroll) return;
  scroll.textContent = '';
  const c = state.conversation;

  // Durable first: runs[] is the record that survives a console restart.
  (c.runs || []).forEach((r) => {
    const run = el('div', 'run');
    const h = el('div', 'h');
    h.appendChild(el('span', 'id', r.runId));
    h.appendChild(el('span', 'st', r.status + (r.exitCode != null ? ' · exit ' + r.exitCode : '')));
    run.appendChild(h);
    if (r.result) run.appendChild(el('div', 'body', flatten(r.result)));
    scroll.appendChild(run);
  });

  if (!c.joined && !(c.runs || []).length) {
    scroll.appendChild(el('p', 'notice', 'Nothing recorded yet.'));
  }

  state.transcript.forEach((m) => {
    const div = el('div', 'msg ' + m.kind + (m.pending ? ' pending' : ''));
    if (m.kind === 'relay' && m.sender) div.appendChild(el('div', 'who', m.sender));
    if (m.kind === 'local') div.appendChild(el('div', 'who', m.pending ? 'you · sending…' : 'you'));
    div.appendChild(el('div', 'body', flatten(m.text)));
    scroll.appendChild(div);
  });
  scroll.scrollTop = scroll.scrollHeight;
}

// ---- boot -------------------------------------------------------------------

(async function boot() {
  try {
    const s = await fetch('/api/session').then((r) => r.json());
    if (s.authenticated) showApp();
    else showLogin(s.configured ? 'Enter the console token.' : 'This console has no token configured — set the console Channel’s uiToken credential.');
  } catch (_) {
    showLogin();
  }
})();
