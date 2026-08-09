import { useMemo, useState } from 'react'
import {
  Label, PageSection, Pagination, Stack, StackItem, Title, Toolbar,
  ToolbarContent, ToolbarItem, FormSelect, FormSelectOption, SearchInput, Switch,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../App'
import { useConversations } from '../api/hooks'
import { PlainText } from '../components/Text'
import { Crumbs } from '../components/Crumbs'

// The list. Filtering, sorting and pagination are all SERVER-side: an event
// storm makes thousands of conversations, and shipping them all so the browser
// can hide most is how a viewer becomes an API-server problem.

const PHASE_COLOR: Record<string, 'blue' | 'green' | 'orange' | 'grey' | 'red'> = {
  Working: 'blue',
  Queued: 'orange',
  Pending: 'orange',
  Idle: 'green',
  Failed: 'red',
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
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [perPage, setPerPage] = useState(50)

  const params = useMemo(() => {
    const p = new URLSearchParams()
    if (phase) p.set('phase', phase)
    if (pipeline) p.set('pipeline', pipeline)
    if (profile) p.set('profile', profile)
    if (errored) p.set('errored', 'true')
    if (search) p.set('q', search)
    p.set('limit', String(perPage))
    p.set('offset', String((page - 1) * perPage))
    return p
  }, [phase, pipeline, profile, errored, search, page, perPage])

  const { data, isLoading, error } = useConversations(params)
  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Could not load conversations">{String(error)}</ErrorState>

  const facets = data.facets ?? {}

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
                  <Th>Title</Th>
                  <Th>Phase</Th>
                  <Th>Pipeline</Th>
                  <Th>Runs</Th>
                  <Th>Queued</Th>
                  <Th>Last activity</Th>
                  <Th>Console</Th>
                </Tr>
              </Thead>
              <Tbody>
                {data.items.map((c) => (
                  <Tr key={c.name}>
                    <Td dataLabel="Title">
                      <Link to={`/conversations/${c.name}`}>
                        <PlainText>{c.title || c.name}</PlainText>
                      </Link>
                      <div>
                        <small>
                          <PlainText>{c.name}</PlainText>
                        </small>
                      </div>
                    </Td>
                    <Td dataLabel="Phase">
                      <Label color={PHASE_COLOR[c.phase ?? ''] ?? 'grey'}>
                        <PlainText>{c.phase}</PlainText>
                      </Label>
                      {c.errored && <Label status="danger">last run failed</Label>}
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
      </PageSection>
    </>
  )
}
