import { Alert, Button, Label, Split, SplitItem, Stack, StackItem, ToggleGroup, ToggleGroupItem, Tooltip } from '@patternfly/react-core'
import { AngleDoubleLeftIcon, AngleDoubleRightIcon } from '@patternfly/react-icons'
import { PipelineName } from '../looks/classic/components/PipelineName'
import type { ChipProps, FrameProps, GraphChrome, NoticeProps, ScopeBarProps, ZoomProps } from './chrome'
import type { ScopeDepth } from './filter'
import { ConversationPanel, EdgePanel, FeedPanel, HopPanel, NodePanel } from './Panel'
import { ConversationReplayBar, WindowReplayBar } from './ReplayBars'
import { DisplayPanel, GraphToolbar } from './Toolbar'

// THE CLASSIC LOOK'S CHROME: PatternFly controls around the shared canvas.

const COLOR = { ok: 'green', bad: 'red', warn: 'orange', info: 'blue', quiet: 'grey', accent: 'purple' } as const

function Frame({ toolbar, replay, notices, canvas, display, panel, sideShown, onToggleSide }: FrameProps) {
  return (
    <Stack hasGutter>
      <StackItem>{toolbar}</StackItem>
      {replay && <StackItem>{replay}</StackItem>}
      {notices}
      <StackItem>
        <Split hasGutter>
          <SplitItem isFilled style={{ minWidth: 0 }}>{canvas}</SplitItem>
          {/* The column beside the canvas folds to a strip holding the chevron
              that brings it back, so the fold and the unfold are one control in
              one place. Inside it each card folds to its title the same way. */}
          <SplitItem style={{ width: sideShown ? 388 : 28, flex: `0 0 ${sideShown ? 388 : 28}px` }}>
            <div style={{ display: 'flex', gap: 4, alignItems: 'flex-start' }}>
              <Button
                variant="plain"
                size="sm"
                aria-label={sideShown ? 'Hide side panels' : 'Show side panels'}
                aria-expanded={sideShown}
                title={sideShown ? 'Hide side panels' : 'Show side panels'}
                onClick={onToggleSide}
                icon={sideShown ? <AngleDoubleRightIcon /> : <AngleDoubleLeftIcon />}
              />
              {sideShown && (
                <Stack hasGutter style={{ flex: 1, minWidth: 0 }}>
                  <StackItem>{display}</StackItem>
                  <StackItem>{panel}</StackItem>
                </Stack>
              )}
            </div>
          </SplitItem>
        </Split>
      </StackItem>
    </Stack>
  )
}

function Notice({ tone, title, testId, plain, children }: NoticeProps) {
  return (
    <StackItem>
      {/* A cut that could conceal a broken component without saying so is the
          one way this view could mislead. */}
      <Alert variant={tone} isInline isPlain={plain} title={title} data-testid={testId}>{children}</Alert>
    </StackItem>
  )
}

function Chip({ tone = 'quiet', testId, children }: ChipProps) {
  return <Label isCompact color={COLOR[tone]} data-testid={testId}>{children}</Label>
}

function ScopeBar({ name, levels, depth, maxDepth, beyond, onDepth, onReset }: ScopeBarProps) {
  return (
    <span data-testid="scope-bar" style={{ display: 'inline-flex', gap: 6, alignItems: 'center' }}>
      <Label color="blue" isCompact>Scoped to {name}</Label>
      {levels.length > 0 && (
        <ToggleGroup aria-label="scope depth" isCompact>
          {levels.map((l: ScopeDepth) => (
            <ToggleGroupItem
              key={String(l)}
              text={l === 'all' ? 'All' : String(l)}
              aria-label={l === 'all' ? 'all connected' : `${l} hop`}
              isSelected={l === 'all' ? depth === 'all' || (typeof depth === 'number' && depth >= maxDepth) : depth === l}
              onChange={() => onDepth(l)}
            />
          ))}
        </ToggleGroup>
      )}
      {beyond > 0 && <Label isCompact color="grey">{beyond} connected beyond this depth</Label>}
      <Button variant="link" isInline onClick={onReset} aria-label="reset scope">Reset scope</Button>
    </span>
  )
}

function Zoom({ percent, onIn, onOut, onFit }: ZoomProps) {
  return (
    <div style={{ display: 'flex', gap: 4 }}>
      <Tooltip content="Zoom in"><Button variant="control" aria-label="zoom in" onClick={onIn}>+</Button></Tooltip>
      <Tooltip content="Zoom out"><Button variant="control" aria-label="zoom out" onClick={onOut}>−</Button></Tooltip>
      <Tooltip content="Fit to view"><Button variant="control" aria-label="fit to view" onClick={onFit}>⤢</Button></Tooltip>
      <span aria-label="zoom level" style={{ alignSelf: 'center', padding: '0 8px', fontSize: 12, color: 'var(--ao-text-subtle)' }}>
        {percent}%
      </span>
    </div>
  )
}

export const classicChrome: GraphChrome = {
  Frame, Notice, Chip, ScopeBar, Zoom, PipelineName,
  Toolbar: GraphToolbar,
  Display: DisplayPanel,
  WindowReplay: WindowReplayBar,
  ConversationReplay: ConversationReplayBar,
  Feed: FeedPanel,
  Node: NodePanel,
  Edge: EdgePanel,
  Hop: HopPanel,
  Conversation: ConversationPanel,
}
