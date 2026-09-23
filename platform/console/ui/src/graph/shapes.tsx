// Per-class mark: an outline and a glyph, drawn at the origin.
//
// Kiali gives each element class its own silhouette so a graph is readable
// before any label is. The outlines are sized for a mark scaled by MARK_SCALE,
// and the glyphs sit inside them on the same origin — one path each, so a node
// is a <g> of two paths and nothing fights over sizing or fill.

export type ShapeKind =
  | 'hexagon' | 'plaque' | 'rect' | 'chip' | 'cloud' | 'server' | 'hub'
  | 'circle' | 'stadium' | 'diamond' | 'diamond2' | 'cylinder' | 'note' | 'box'

export const MARK_SCALE = 1.95
/** How far from a node's centre an edge stops. */
export const MARK_RADIUS = 38
export const LABEL_Y = 54
export const CAPTION_Y = 70

const SHAPES: Record<ShapeKind, string> = {
  hexagon: 'M-18 0 L-9 -15 L9 -15 L18 0 L9 15 L-9 15 Z',
  plaque: 'M-18 -12 h36 v24 h-36 z M-18 -6 h-3 M-18 6 h-3',
  rect: 'M-22 -13 h44 a3 3 0 0 1 3 3 v20 a3 3 0 0 1 -3 3 h-44 a3 3 0 0 1 -3 -3 v-20 a3 3 0 0 1 3 -3 z',
  chip: 'M-17 -11 h34 v22 h-34 z',
  cloud: 'M-14 6 a7 7 0 0 1 1 -14 a10 10 0 0 1 19 -2 a8 8 0 0 1 8 16 z',
  server: 'M-17 -14 h34 v10 h-34 z M-17 4 h34 v10 h-34 z',
  hub: 'M-26 -17 h52 a4 4 0 0 1 4 4 v26 a4 4 0 0 1 -4 4 h-52 a4 4 0 0 1 -4 -4 v-26 a4 4 0 0 1 4 -4 z',
  circle: 'M-15 0 a15 15 0 1 0 30 0 a15 15 0 1 0 -30 0',
  stadium: 'M-12 -12 h24 a12 12 0 0 1 0 24 h-24 a12 12 0 0 1 0 -24 z',
  diamond: 'M0 -17 L17 0 L0 17 L-17 0 Z',
  diamond2: 'M0 -17 L17 0 L0 17 L-17 0 Z M-8 0 h16',
  cylinder: 'M-16 -10 a16 5 0 0 0 32 0 a16 5 0 0 0 -32 0 v20 a16 5 0 0 0 32 0 v-20',
  note: 'M-13 -15 h18 l8 8 v22 h-26 z M5 -15 v8 h8',
  box: 'M0 -15 l13 7 v16 l-13 7 -13 -7 v-16 z M0 -1 l13 -7 M0 -1 l-13 -7 M0 -1 v16',
}

const GLYPHS = {
  source: 'M2 -7 L-4 1 h4 l-1 6 6 -8 h-4 z',
  adapter: 'M-4 -6 v4 M4 -6 v4 M-6 -2 h12 v2 a6 6 0 0 1 -12 0 z M0 6 v3',
  pipeline: 'M-8 -4 h5 a3 3 0 0 1 3 3 v2 a3 3 0 0 0 3 3 h5 M6 2 l2 2 -2 2',
  profile: 'M0 -6 a3 3 0 1 1 0 6 a3 3 0 0 1 0 -6 M-6 7 a6 6 0 0 1 12 0 z',
  runtime: 'M-5 -5 h10 v10 h-10 z M-2 -2 h4 v4 h-4 z',
  toolset: 'M5 -5 a3 3 0 0 1 -4 4 l-5 5 -1 -1 5 -5 a3 3 0 0 1 4 -4 z',
  mcpconfig: 'M-6 -5 h12 v4 h-12 z M-6 1 h12 v4 h-12 z',
  channel: 'M-6 -5 h12 v8 h-6 l-3 3 v-3 h-3 z',
  conversation: 'M-5 -2 h10 M-5 2 h6',
  pod: 'M-4 -4 h8 v8 h-8 z',
  manager: 'M-7 -5 h14 v10 h-14 z M-7 -1 h14 M-3 3 h.01 M0 3 h.01',
  image: 'M-6 -6 h12 v12 h-12 z M-2 -2 h4 v4 h-4 z M0 -9 v3 M0 6 v3 M-9 0 h3 M6 0 h3',
  model: 'M0 -5 l1.5 3.5 3.5 1.5 -3.5 1.5 -1.5 3.5 -1.5 -3.5 -3.5 -1.5 3.5 -1.5 z',
  mcpserver: 'M-6 -7 h12 M-6 -3 h12 M-6 3 h12 M-6 7 h12',
  container: 'M-6 -4 h12 M-6 0 h12 M-6 4 h12',
  gateway: 'M-7 0 h14 M3 -4 l4 4 -4 4 M-3 -4 l-4 4 4 4',
  job: 'M0 -6 a6 6 0 1 0 0.01 0 M0 -3 v3 l2 2',
  external: 'M0 -6.5 a6.5 6.5 0 1 0 0.01 0 M-6.5 0 h13 M0 -6.5 c-3.4 3.2 -3.4 9.8 0 13 M0 -6.5 c3.4 3.2 3.4 9.8 0 13',
  repository: 'M-3 -6 a2 2 0 1 0 0.01 0 M-3 6 a2 2 0 1 0 0.01 0 M5 -2 a2 2 0 1 0 0.01 0 M-3 -4 v8 M-3 0 c0 -3 8 0 8 -2',
} as const

export interface NodeStyle {
  shape: ShapeKind
  /** Drawn on a grid centred at the origin, about ±9 across. */
  glyph: string
  label: string
}

export const NODE_STYLES: Record<string, NodeStyle> = {
  // Model
  signaladapters: { shape: 'plaque', glyph: GLYPHS.adapter, label: 'Signal adapter' },
  signalsources: { shape: 'hexagon', glyph: GLYPHS.source, label: 'Signal source' },
  pipelines: { shape: 'rect', glyph: GLYPHS.pipeline, label: 'Pipeline' },
  agentprofiles: { shape: 'circle', glyph: GLYPHS.profile, label: 'Profile' },
  agentruntimes: { shape: 'stadium', glyph: GLYPHS.runtime, label: 'Runtime' },
  mcptoolsets: { shape: 'diamond', glyph: GLYPHS.toolset, label: 'Toolset' },
  mcpconfigs: { shape: 'diamond2', glyph: GLYPHS.mcpconfig, label: 'MCP config' },
  channels: { shape: 'cylinder', glyph: GLYPHS.channel, label: 'Channel' },
  channeladapters: { shape: 'plaque', glyph: GLYPHS.adapter, label: 'Channel adapter' },
  conversations: { shape: 'note', glyph: GLYPHS.conversation, label: 'Conversation' },
  // Components
  manager: { shape: 'hub', glyph: GLYPHS.manager, label: 'Manager' },
  'signal-adapter': { shape: 'plaque', glyph: GLYPHS.adapter, label: 'Signal adapter' },
  'channel-adapter': { shape: 'plaque', glyph: GLYPHS.adapter, label: 'Channel adapter' },
  gateway: { shape: 'plaque', glyph: GLYPHS.gateway, label: 'Gateway' },
  'runtime-image': { shape: 'stadium', glyph: GLYPHS.image, label: 'Runtime image' },
  sidecar: { shape: 'chip', glyph: GLYPHS.container, label: 'Sidecar' },
  housekeeping: { shape: 'box', glyph: GLYPHS.job, label: 'Housekeeping' },
  model: { shape: 'cloud', glyph: GLYPHS.model, label: 'Model' },
  'mcp-server': { shape: 'server', glyph: GLYPHS.mcpserver, label: 'MCP server' },
  repository: { shape: 'circle', glyph: GLYPHS.repository, label: 'Repository' },
  external: { shape: 'cloud', glyph: GLYPHS.external, label: 'External system' },
  workload: { shape: 'box', glyph: GLYPHS.pod, label: 'Workload' },
  // Infrastructure
  pod: { shape: 'box', glyph: GLYPHS.pod, label: 'Pod' },
  container: { shape: 'chip', glyph: GLYPHS.container, label: 'Container' },
}

/** A pod's glyph is what it runs: a pod is a pod, but each one has a purpose. */
export const ROLE_GLYPH: Record<string, string> = {
  manager: GLYPHS.manager,
  'signal-adapter': GLYPHS.adapter,
  'channel-adapter': GLYPHS.adapter,
  gateway: GLYPHS.gateway,
  'runtime-image': GLYPHS.image,
  housekeeping: GLYPHS.job,
  'mcp-server': GLYPHS.mcpserver,
}

export function styleFor(cls: string): NodeStyle {
  return NODE_STYLES[cls] ?? { shape: 'rect', glyph: GLYPHS.pod, label: cls }
}

export function shapePath(shape: ShapeKind): string {
  return SHAPES[shape]
}

export function plural(label: string): string {
  return /[^aeiou]y$/.test(label) ? `${label.slice(0, -1)}ies` : `${label}s`
}
