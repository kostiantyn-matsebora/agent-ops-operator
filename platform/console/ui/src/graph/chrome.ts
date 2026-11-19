import type { ComponentProps, ComponentType, ReactNode } from 'react'
import type { HiddenSummary, ScopeDepth } from './filter'
import type { ConversationPanel, EdgePanel, FeedPanel, HopPanel, NodePanel } from './Panel'
import type { ConversationReplayBar, WindowReplayBar } from './ReplayBars'

// THE GRAPH'S CHROME, injected. The canvas, the layout, the filters, the replay
// and every pure rule are shared by every look; what differs is how the
// controls and panels AROUND the canvas are drawn. A look hands `GraphView` a
// set of these, and the graph itself imports no component library or look.
//
// Every member receives the same props whichever look draws it, so a function
// one look has is a function the other must have: the type is the checklist.

export const WINDOWS = [
  { seconds: 60, label: 'Last 1m' },
  { seconds: 300, label: 'Last 5m' },
  { seconds: 600, label: 'Last 10m' },
  { seconds: 1800, label: 'Last 30m' },
]

export type Mode = 'live' | 'replay' | 'conversation'

export interface ToolbarProps {
  pipelines: string[]
  /** A pipeline's declared icon, by name, for the selector's rows. */
  iconFor: (name: string) => string | undefined
  find: string
  hide: string
  onFind: (v: string) => void
  onHide: (v: string) => void
  mode: Mode
  onMode: (m: 'live' | 'replay') => void
}

export interface DisplayProps {
  hidden: HiddenSummary
  viewLabel: string
}

export type NoticeTone = 'info' | 'warning'
export interface NoticeProps {
  tone: NoticeTone
  title: string
  testId?: string
  /** A quieter notice: a hint, not a warning. */
  plain?: boolean
  children?: ReactNode
}

export type ChipTone = 'ok' | 'bad' | 'warn' | 'info' | 'quiet' | 'accent'
export interface ChipProps {
  tone?: ChipTone
  testId?: string
  children: ReactNode
}

export interface ScopeBarProps {
  name: string
  levels: ScopeDepth[]
  depth: ScopeDepth
  maxDepth: number
  /** Elements connected to the scope but past its depth. */
  beyond: number
  onDepth: (d: ScopeDepth) => void
  onReset: () => void
}

export interface FrameProps {
  toolbar: ReactNode
  /** The replay bar of the mode in force, if any. */
  replay: ReactNode
  /** Notices about what the picture cannot say. */
  notices: ReactNode
  /** The canvas, or its empty state. */
  canvas: ReactNode
  /** The Display panel. */
  display: ReactNode
  /** The feed, or whatever is selected. */
  panel: ReactNode
  sideShown: boolean
  onToggleSide: () => void
}

export interface GraphChrome {
  Frame: ComponentType<FrameProps>
  Toolbar: ComponentType<ToolbarProps>
  Display: ComponentType<DisplayProps>
  WindowReplay: ComponentType<ComponentProps<typeof WindowReplayBar>>
  ConversationReplay: ComponentType<ComponentProps<typeof ConversationReplayBar>>
  Feed: ComponentType<ComponentProps<typeof FeedPanel>>
  Node: ComponentType<ComponentProps<typeof NodePanel>>
  Edge: ComponentType<ComponentProps<typeof EdgePanel>>
  Hop: ComponentType<ComponentProps<typeof HopPanel>>
  Conversation: ComponentType<ComponentProps<typeof ConversationPanel>>
  Notice: ComponentType<NoticeProps>
  Chip: ComponentType<ChipProps>
  ScopeBar: ComponentType<ScopeBarProps>
  /** A pipeline's name with its icon. */
  PipelineName: ComponentType<{ name: string; icon?: string }>
  /** The canvas's own zoom controls; the viewport's plain ones when absent. */
  Zoom?: ComponentType<ZoomProps>
}

export interface ZoomProps {
  percent: number
  onIn: () => void
  onOut: () => void
  onFit: () => void
}
