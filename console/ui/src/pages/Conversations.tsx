import { useEffect, useMemo, useState } from 'react'
import {
  Button, Label, PageSection, Pagination, Stack, StackItem, Title, Toolbar,
  ToolbarContent, ToolbarItem, FormSelect, FormSelectOption, SearchInput, Switch,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../App'
import {
  useCloseConversations, useConversations, useDeleteConversations,
  useMarkRead, useReopenConversation, useSession,
} from '../api/hooks'
import { PlainText } from '../components/Text'
import { Crumbs } from '../components/Crumbs'
import { CloseSelectedModal, selectableNames, workingCount } from './CloseConversations'
import { DeleteSelectedModal, deletableNames } from './DeleteConversations'
import { ApiError } from '../api/client'

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
  const [profile, setProfile] = useState('')
  const [errored, setErrored] = useState(false)
  // Unread is a FILTER like every other one — evaluated server-side, so a
  // narrowed list still pages correctly.
  const [unread, setUnread] = useState(false)
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
              click from every page rather than repeated per view. */}
          <Title headingLevel="h1">Conversations</Title>
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
                {data.items.map((c, rowIndex) => (
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
                      {/* Unread is marked twice over — weight for the scan, a
                          label for anyone who cannot see weight. Theme tokens
                          only: a literal colour here would be the one place the
                          console's palette lives outside theme.css. */}
                      <Link
                        to={`/conversations/${c.name}`}
                        style={c.unread ? { fontWeight: 700, color: 'var(--ao-brand-strong)' } : undefined}
                      >
                        <PlainText>{c.title || c.name}</PlainText>
                      </Link>
                      {c.unread && (
                        <Label isCompact color="blue" data-testid={`unread-${c.name}`} style={{ marginLeft: 6 }}>
                          unread
                        </Label>
                      )}
                      <div>
                        <small>
                          <PlainText>{c.name}</PlainText>
                        </small>
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
                      {c.pipeline ? <PlainText>{c.pipeline}</PlainText> : <small>unattributed</small>}
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
