import { useEffect, useMemo, useState } from 'react'
import {
  Button, Label, PageSection, Pagination, Stack, StackItem, Title, Toolbar,
  ToolbarContent, ToolbarItem, FormSelect, FormSelectOption, SearchInput, Switch,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../components/States'
import {
  useCloseConversations, useConversations, useDeleteConversations,
  useMarkRead, usePipelineIcon, useReopenConversation, useSession,
} from '../api/hooks'
import { PlainText } from '../components/Text'
import { Crumbs } from '../components/Crumbs'
import { HelpChip, PipelinesChip } from './Vocabulary'
import { Icon, stripLeadingIcon } from '../components/Icon'
import { CloseSelectedModal, selectableNames, workingCount } from './CloseConversations'
import { DeleteSelectedModal, deletableNames } from './DeleteConversations'
import { ApiError } from '../api/client'
import type { ConversationSummary } from '../api/types'

/**
 * Client-side, CURRENT-PAGE-ONLY grouping: a row whose `causedBy.parent` is
 * also on this page is nested directly under it, depth-first, so a
 * grandchild sits under its own parent rather than back under the root.
 *
 * There is no server support for this — the server's own ordering (newest
 * activity first) is otherwise untouched — and a member whose root is not on
 * this page is left exactly where the server put it: the task calls for a
 * flatten toggle, not a second fetch to always find the root.
 */
function groupedRows(items: ConversationSummary[]): { row: ConversationSummary; depth: number }[] {
  const byName = new Map(items.map((c) => [c.name, c]))
  const isNested = (c: ConversationSummary) => Boolean(c.causedBy && byName.has(c.causedBy.parent))
  const childrenOf = new Map<string, ConversationSummary[]>()
  for (const c of items) {
    if (!isNested(c)) continue
    const parent = c.causedBy!.parent
    childrenOf.set(parent, [...(childrenOf.get(parent) ?? []), c])
  }
  const out: { row: ConversationSummary; depth: number }[] = []
  const walk = (c: ConversationSummary, depth: number) => {
    out.push({ row: c, depth })
    for (const child of childrenOf.get(c.name) ?? []) walk(child, depth + 1)
  }
  for (const c of items) {
    if (!isNested(c)) walk(c, 0)
  }
  return out
}

// The list. Filtering, sorting and pagination are all SERVER-side: an event
// storm makes thousands of conversations, and shipping them all so the browser
// can hide most is how a viewer becomes an API-server problem.

const PHASE_COLOR: Record<string, 'blue' | 'green' | 'orange' | 'grey' | 'red'> = {
  Working: 'blue',
  Queued: 'orange',
  Pending: 'orange',
  Idle: 'green',
  Failed: 'red',
  // A STATE, not an absence: the conversation is still here with its answers
  // and its workspace, and it can be reopened.
  Closed: 'grey',
}

function age(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  if (seconds < 86400) return `${(seconds / 3600).toFixed(1)}h`
  return `${(seconds / 86400).toFixed(1)}d`
}

export function ConversationsPage() {
  const [phase, setPhase] = useState('')
  const [pipeline, setPipeline] = useState('')
  // The route's declared icon, by name, from the shared lookup.
  const iconFor = usePipelineIcon()
  const [profile, setProfile] = useState('')
  const [errored, setErrored] = useState(false)
  // Unread is a FILTER like every other one — evaluated server-side, so a
  // narrowed list still pages correctly.
  const [unread, setUnread] = useState(false)
  // OFF by default, matching today's behavior exactly. ON nests a row whose
  // root is present on this page directly under it; a member whose root is
  // not on this page is unaffected either way.
  const [groupByRoot, setGroupByRoot] = useState(false)
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [perPage, setPerPage] = useState(50)
  // Selection is over the rows ON SCREEN. There is deliberately no "select
  // everything matching the filter": a mis-set filter would then close far more
  // than was ever visible.
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [closeOpen, setCloseOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const params = useMemo(() => {
    const p = new URLSearchParams()
    if (phase) p.set('phase', phase)
    if (pipeline) p.set('pipeline', pipeline)
    if (profile) p.set('profile', profile)
    if (errored) p.set('errored', 'true')
    if (unread) p.set('unread', 'true')
    if (search) p.set('q', search)
    p.set('limit', String(perPage))
    p.set('offset', String((page - 1) * perPage))
    return p
  }, [phase, pipeline, profile, errored, unread, search, page, perPage])

  const { data, isLoading, error } = useConversations(params)
  const session = useSession()
  const close = useCloseConversations()
  const del = useDeleteConversations()
  const markRead = useMarkRead()
  const reopen = useReopenConversation()

  // What is selected must never outlive what was on screen when it was picked.
  const scope = params.toString()
  useEffect(() => {
    setSelected(new Set())
  }, [scope])

  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Could not load conversations">{String(error)}</ErrorState>

  const facets = data.facets ?? {}
  // The action is hidden, not merely disabled, when this console cannot write:
  // the server refuses it regardless, and a control that only ever fails is
  // worse than none. `canWrite` folds in the missing-identity case too.
  const canClose = session.data?.canWrite ?? false
  const selectable = selectableNames(data.items)
  // Deleting is offered only when the SELECTION is entirely closed: the
  // two-step is the safety property, so a mixed batch must not be one click.
  const deletable = deletableNames(data.items)
  const selectedAllClosed =
    selected.size > 0 && [...selected].every((n) => deletable.includes(n))
  const names = data.items.map((c) => c.name).filter((n) => selected.has(n))
  const allSelected = selectable.length > 0 && selectable.every((n) => selected.has(n))
  const rows = groupByRoot ? groupedRows(data.items) : data.items.map((row) => ({ row, depth: 0 }))

  function setRow(name: string, isSelected: boolean) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (isSelected) next.add(name)
      else next.delete(name)
      return next
    })
  }

  function toggleAll(isSelected: boolean) {
    // Scoped to `selectable`, which is this page's rows: select-all can never
    // reach a conversation the operator has not seen.
    setSelected(isSelected ? new Set(selectable) : new Set())
  }

  function runClose(includeWorking: boolean) {
    close.mutate(
      { names, includeWorking },
      { onSuccess: () => setSelected(new Set()) },
    )
  }

  function dismissClose() {
    setCloseOpen(false)
    close.reset()
  }

  // Marking read is NOT behind canWrite: it instructs no agent and starts no
  // work, and a read-only console that could show a backlog without ever
  // clearing it would be broken in the way the unread mark exists to fix.
  function runMarkRead() {
    markRead.mutate({ names }, { onSuccess: () => setSelected(new Set()) })
  }

  function runDelete() {
    del.mutate({ names }, { onSuccess: () => setSelected(new Set()) })
  }

  function dismissDelete() {
    setDeleteOpen(false)
    del.reset()
  }

  return (
    <>
      <Crumbs items={[{ label: 'Conversations' }]} />
      <PageSection>
        <Stack hasGutter>
        <StackItem>
          {/* "New conversation" lives in the masthead — a global action, one
              click from every page rather than repeated per view.
              These two are REFERENCE, not action: what can be addressed, and
              what may be typed. The manager answers the same questions as chat
              commands, but its answer is posted to a channel's general surface
              and this console has no view for one — so it shows the vocabulary
              it already holds instead of asking and losing the reply. */}
          <div style={{ display: 'flex', alignItems: 'baseline', gap: '0.75rem', flexWrap: 'wrap' }}>
            <Title headingLevel="h1">Conversations</Title>
            <PipelinesChip />
            <HelpChip />
          </div>
        </StackItem>
        <StackItem>
          <Toolbar>
            <ToolbarContent>
              <ToolbarItem>
                <SearchInput
                  aria-label="search conversations"
                  placeholder="name or title"
                  value={search}
                  onChange={(_e, v) => {
                    setSearch(v)
                    setPage(1)
                  }}
                  onClear={() => setSearch('')}
                />
              </ToolbarItem>
              <ToolbarItem>
                <FormSelect
                  aria-label="phase"
                  value={phase}
                  onChange={(_e, v) => {
                    setPhase(v)
                    setPage(1)
                  }}
                >
                  <FormSelectOption value="" label="Any phase" />
                  {(facets.phase ?? []).map((p) => (
                    <FormSelectOption key={p} value={p} label={p} />
                  ))}
                </FormSelect>
              </ToolbarItem>
              <ToolbarItem>
                <FormSelect
                  aria-label="pipeline"
                  value={pipeline}
                  onChange={(_e, v) => {
                    setPipeline(v)
                    setPage(1)
                  }}
                >
                  <FormSelectOption value="" label="Any pipeline" />
                  {(facets.pipeline ?? []).map((p) => (
                    <FormSelectOption key={p} value={p} label={p} />
                  ))}
                </FormSelect>
              </ToolbarItem>
              <ToolbarItem>
                <FormSelect
                  aria-label="profile"
                  value={profile}
                  onChange={(_e, v) => {
                    setProfile(v)
                    setPage(1)
                  }}
                >
                  <FormSelectOption value="" label="Any profile" />
                  {(facets.profile ?? []).map((p) => (
                    <FormSelectOption key={p} value={p} label={p} />
                  ))}
                </FormSelect>
              </ToolbarItem>
              <ToolbarItem>
                <Switch
                  id="errored"
                  label="Errored only"
                  isChecked={errored}
                  onChange={(_e, v) => {
                    setErrored(v)
                    setPage(1)
                  }}
                />
              </ToolbarItem>
              <ToolbarItem>
                <Switch
                  id="unread"
                  label="Unread only"
                  isChecked={unread}
                  onChange={(_e, v) => {
                    setUnread(v)
                    setPage(1)
                  }}
                />
              </ToolbarItem>
              <ToolbarItem>
                {/* A visual fold, not a filter — nothing leaves the page, a
                    member just moves under its root when that root is also
                    on it. Off is today's behavior, byte for byte. */}
                <Switch
                  id="group-by-root"
                  label="Group by root"
                  isChecked={groupByRoot}
                  onChange={(_e, v) => setGroupByRoot(v)}
                />
              </ToolbarItem>
              <ToolbarItem>
                <Button
                  variant="secondary"
                  isDisabled={names.length === 0 || markRead.isPending}
                  onClick={runMarkRead}
                  data-testid="mark-read"
                >
                  Mark read{names.length > 0 ? ` (${names.length})` : ''}
                </Button>
              </ToolbarItem>
              {canClose && (
                <ToolbarItem>
                  <Button
                    variant="secondary"
                    isDanger
                    isDisabled={names.length === 0}
                    onClick={() => setCloseOpen(true)}
                    data-testid="close-selected"
                  >
                    Close selected{names.length > 0 ? ` (${names.length})` : ''}
                  </Button>
                </ToolbarItem>
              )}
              {canClose && (
                <ToolbarItem>
                  {/* Enabled only for a selection that is entirely CLOSED. A
                      mixed batch is skipped server-side anyway, but offering
                      it would make the two-step feel like a nag rather than
                      the safety property it is. */}
                  <Button
                    variant="secondary"
                    isDanger
                    isDisabled={!selectedAllClosed}
                    onClick={() => setDeleteOpen(true)}
                    data-testid="delete-selected"
                  >
                    Delete selected{names.length > 0 ? ` (${names.length})` : ''}
                  </Button>
                </ToolbarItem>
              )}
              <ToolbarItem variant="pagination">
                <Pagination
                  itemCount={data.total}
                  page={page}
                  perPage={perPage}
                  onSetPage={(_e, p) => setPage(p)}
                  onPerPageSelect={(_e, pp) => {
                    setPerPage(pp)
                    setPage(1)
                  }}
                />
              </ToolbarItem>
            </ToolbarContent>
          </Toolbar>
        </StackItem>
        <StackItem>
          {data.items.length === 0 ? (
            <Empty title="No conversations match">
              {data.total > 0
                ? 'Every conversation was filtered out — clear a filter to see them.'
                : 'Nothing has originated yet.'}
            </Empty>
          ) : (
            <Table variant="compact" aria-label="conversations">
              <Thead>
                <Tr>
                  {/* The selection column is NOT gated on canWrite: marking
                      read needs a selection and is not a write to a
                      conversation, so a read-only console still gets one. */}
                  <Th
                    aria-label="select all on this page"
                    select={{
                      onSelect: (_e, isSelected) => toggleAll(isSelected),
                      isSelected: allSelected,
                      isHeaderSelectDisabled: selectable.length === 0,
                    }}
                  />
                  <Th>Title</Th>
                  <Th>Phase</Th>
                  <Th>Pipeline</Th>
                  <Th>Runs</Th>
                  <Th>Queued</Th>
                  <Th>Last activity</Th>
                  <Th>Console</Th>
                  <Th screenReaderText="reopen" />
                </Tr>
              </Thead>
              <Tbody>
                {rows.map(({ row: c, depth }, rowIndex) => (
                  <Tr key={c.name}>
                    <Td
                      select={{
                        rowIndex,
                        onSelect: (_e, isSelected) => setRow(c.name, isSelected),
                        isSelected: selected.has(c.name),
                        // A conversation already on its way out cannot be
                        // closed again — there would be nowhere to post.
                        isDisabled: c.deleting,
                      }}
                    />
                    <Td dataLabel="Title">
                      {/* Grouped: nested directly under the root row it names,
                          indented one step per level so a grandchild reads
                          under its own parent rather than back under the
                          root. */}
                      <div style={{ paddingLeft: depth * 24 }}>
                      {/* Unread is marked twice over — weight for the scan, a
                          label for anyone who cannot see weight. Theme tokens
                          only: a literal colour here would be the one place the
                          console's palette lives outside theme.css. */}
                      {/* THE ROUTE'S OWN ICON, not a generic one. A title
                          carries a lane emoji the manager wrote into it; the
                          Pipeline says what IT looks like, which is more use
                          in a list where every row is a conversation. The
                          baked-in one is stripped so the two do not stack. */}
                      <Link
                        to={`/conversations/${c.name}`}
                        style={c.unread ? { fontWeight: 700, color: 'var(--ao-brand-strong)' } : undefined}
                      >
                        <Icon icon={iconFor(c.pipeline)} />{' '}
                        <PlainText>{stripLeadingIcon(c.title || c.name)}</PlainText>
                      </Link>
                      {c.unread && (
                        <Label isCompact color="blue" data-testid={`unread-${c.name}`} style={{ marginLeft: 6 }}>
                          unread
                        </Label>
                      )}
                      {c.coordinator && (
                        <Label isCompact color="teal" style={{ marginLeft: 6 }}>
                          coordinator
                        </Label>
                      )}
                      {depth > 0 && c.causedBy && (
                        <Label isCompact color="purple" style={{ marginLeft: 6 }}>
                          <PlainText>{`member via ${c.causedBy.entry}`}</PlainText>
                        </Label>
                      )}
                      <div>
                        <small>
                          <PlainText>{c.name}</PlainText>
                        </small>
                      </div>
                      </div>
                    </Td>
                    <Td dataLabel="Phase">
                      {/* A conversation held by its close-topics finalizer is
                          on its way out, not idle. It says DELETING, because
                          that is the verb that put it there — /close sets a
                          phase and leaves the object alone. Without this the
                          list looks untouched after a delete and gets deleted
                          again. */}
                      {c.deleting ? (
                        <Label color="grey">deleting</Label>
                      ) : (
                        <Label color={PHASE_COLOR[c.phase ?? ''] ?? 'grey'}>
                          <PlainText>{c.phase}</PlainText>
                        </Label>
                      )}
                      {c.errored && <Label status="danger">last run failed</Label>}
                      {/* A queue that has stopped moving is either full or its
                          storage is gone, and those demand opposite responses.
                          The reason is the kubelet's own, so the row says which. */}
                      {c.blocked && (
                        <Label
                          status={c.blocked.storage ? 'danger' : 'warning'}
                          title={c.blocked.detail}
                        >
                          <PlainText>
                            {`${c.blocked.storage ? 'storage' : 'blocked'}: ${c.blocked.reason}`}
                          </PlainText>
                        </Label>
                      )}
                    </Td>
                    <Td dataLabel="Pipeline">
                      {/* Attribution is INFERRED from materialized bindings.
                          Blank means ambiguous, never "none". */}
                      {c.pipeline ? (
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem' }}>
                          <Icon icon={iconFor(c.pipeline)} />
                          <PlainText>{c.pipeline}</PlainText>
                        </span>
                      ) : (
                        <small>unattributed</small>
                      )}
                    </Td>
                    <Td dataLabel="Runs">{c.runCount}</Td>
                    <Td dataLabel="Queued">{c.queued}</Td>
                    <Td dataLabel="Last activity">{age(c.ageSeconds)}</Td>
                    <Td dataLabel="Console">
                      {c.joined ? <Label color="blue">joined</Label> : <small>observed</small>}
                    </Td>
                    <Td dataLabel="Reopen">
                      {/* Per row, and only where it means something. Closed is a
                          STATE, not an absence: the conversation is still here
                          with its answers and its workspace, and this is how it
                          comes back. No bulk equivalent — a batch would
                          re-materialise threads on surfaces nobody is watching. */}
                      {canClose && c.phase === 'Closed' && !c.deleting && (
                        <Button
                          variant="link"
                          isInline
                          isDisabled={reopen.isPending}
                          onClick={() => reopen.mutate(c.name)}
                          data-testid={`reopen-${c.name}`}
                        >
                          Reopen
                        </Button>
                      )}
                    </Td>
                  </Tr>
                ))}
              </Tbody>
            </Table>
          )}
        </StackItem>
        <StackItem>
          <small>
            {data.items.length} of {data.total} matching conversation(s).
          </small>
        </StackItem>
        </Stack>
        {deleteOpen && (
          <DeleteSelectedModal
            isOpen
            names={names}
            result={del.data}
            error={del.error ? (del.error as ApiError).message : undefined}
            busy={del.isPending}
            onConfirm={runDelete}
            onClose={dismissDelete}
          />
        )}
        {closeOpen && (
          <CloseSelectedModal
            isOpen
            names={names}
            working={workingCount(data.items, selected)}
            result={close.data}
            error={close.error ? (close.error as ApiError).message : undefined}
            busy={close.isPending}
            onConfirm={runClose}
            onClose={dismissClose}
          />
        )}
      </PageSection>
    </>
  )
}
