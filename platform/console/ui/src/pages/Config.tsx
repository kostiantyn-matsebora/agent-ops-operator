import {
  Card, CardBody, CardTitle, CodeBlock, CodeBlockCode, DescriptionList,
  DescriptionListDescription, DescriptionListGroup, DescriptionListTerm, Gallery, Label,
  LabelGroup, PageSection, Stack, StackItem, Tab, TabTitleText, Tabs, Title, Tooltip,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link, useParams } from 'react-router-dom'
import { useState } from 'react'
import { Empty, ErrorState, Loading } from '../components/States'
import { useDetail, useInventory, useKinds } from '../api/hooks'
import { PlainText } from '../components/Text'
import { Crumbs } from '../components/Crumbs'
import { Yaml } from '../components/Yaml'
import { ConditionChips, HealthChip, KeyValueChips, MetadataCard, age } from '../components/Metadata'
import { styleFor } from '../graph/shapes'
import type { InventoryRow } from '../api/types'

// Per-kind inventory → detail. Read-only, and that is a position rather than a
// gap: Pipelines are the wiring, the wiring is GitOps-managed, and a console
// that edits them competes with helmfile.

export function ConfigPage() {
  const { data, isLoading, error } = useKinds()
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorState title="Could not load kinds">{String(error)}</ErrorState>
  return (
    <>
      <Crumbs items={[{ label: 'Configuration' }]} />
      <PageSection>
        <Stack hasGutter>
          <StackItem>
            <Title headingLevel="h1">Configuration</Title>
          </StackItem>
          <StackItem>
            <Gallery hasGutter minWidths={{ default: '240px' }}>
              {data.map((k) => (
                <Card key={k.kind} isClickable>
                  <CardTitle>
                    <Link to={`/config/${k.kind}`}>
                      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                        <KindGlyph kind={k.kind} />
                        {k.title}
                      </span>
                    </Link>
                  </CardTitle>
                  <CardBody>
                    {k.count} object{k.count === 1 ? '' : 's'}
                    {!k.synced && (
                      <div>
                        <Label isCompact color="orange">not synced yet</Label>
                      </div>
                    )}
                  </CardBody>
                </Card>
              ))}
            </Gallery>
          </StackItem>
        </Stack>
      </PageSection>
    </>
  )
}

/** The same silhouette the graph uses, so the two views teach one vocabulary. */
function KindGlyph({ kind, size = 18 }: { kind: string; size?: number }) {
  const s = styleFor(kind)
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" style={{ flex: '0 0 auto' }}>
      <path
        d={s.glyph}
        fill="none"
        stroke="var(--pf-t--global--icon--color--subtle, #6a6e73)"
        strokeWidth={1.8}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

/**
 * Kind-specific columns. A Pipeline row is not a Channel row, and a generic
 * table of name+age would make every kind equally uninformative.
 */
function columnsFor(kind: string): string[] {
  switch (kind) {
    case 'pipelines':
      return ['profile', 'sources', 'channels', 'toolsets', 'toolsMode', 'mcpConfigs']
    case 'channels':
      return ['adapter', 'served']
    case 'signalsources':
      return ['adapter', 'served', 'wired']
    case 'agentprofiles':
      return ['runtime', 'repository']
    case 'channeladapters':
    case 'signaladapters':
      return ['image', 'servedBy']
    case 'agentruntimes':
      return ['image']
    case 'mcptoolsets':
      return ['tools']
    case 'mcpconfigs':
      return ['servers']
    case 'conversations':
      return ['phase', 'profile']
    default:
      return []
  }
}

/** Columns whose values are lists worth rendering as chips rather than prose. */
const CHIP_COLUMNS = new Set([
  'sources', 'channels', 'toolsets', 'mcpConfigs', 'tools', 'servers',
])
const STATUS_COLUMNS = new Set(['served', 'wired'])

function ColumnValue({ name, value }: { name: string; value?: string }) {
  if (!value) return <small>—</small>
  if (STATUS_COLUMNS.has(name)) {
    return (
      <Label isCompact status={value === 'True' ? 'success' : 'danger'}>
        {name}={value}
      </Label>
    )
  }
  if (CHIP_COLUMNS.has(name)) {
    const parts = value.split(',').map((v) => v.trim()).filter(Boolean)
    return (
      <LabelGroup numLabels={4}>
        {parts.map((p) => (
          <Label key={p} isCompact color="blue" textMaxWidth="16ch">
            <PlainText>{p}</PlainText>
          </Label>
        ))}
      </LabelGroup>
    )
  }
  return (
    <Tooltip content={<PlainText>{value}</PlainText>}>
      <span>
        <PlainText>{value.length > 34 ? `${value.slice(0, 33)}…` : value}</PlainText>
      </span>
    </Tooltip>
  )
}

export function ConfigKindPage() {
  const { kind = '' } = useParams()
  const { data, isLoading, error } = useInventory(kind)
  const cols = columnsFor(kind)
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorState title={`Could not load ${kind}`}>{String(error)}</ErrorState>

  return (
    <>
      <Crumbs items={[{ label: 'Configuration', to: '/config' }, { label: kind }]} />
      <PageSection>
        <Stack hasGutter>
          <StackItem>
            <Title headingLevel="h1">
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 10 }}>
                <KindGlyph kind={kind} size={22} />
                {kind}
              </span>
            </Title>
          </StackItem>
          <StackItem>
            {data.length === 0 ? (
              <Empty title={`No ${kind}`}>Nothing of this kind exists in the namespace.</Empty>
            ) : (
              <Table variant="compact" aria-label={kind}>
                <Thead>
                  <Tr>
                    <Th>Name</Th>
                    {cols.map((c) => (
                      <Th key={c}>{c}</Th>
                    ))}
                    <Th>Status</Th>
                    <Th>Labels</Th>
                    <Th>Age</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {data.map((row: InventoryRow) => (
                    <Tr key={row.name}>
                      <Td dataLabel="Name">
                        <Link to={`/config/${kind}/${row.name}`}>
                          <PlainText>{row.name}</PlainText>
                        </Link>
                        {row.findings > 0 && (
                          <Tooltip content="the console's own cross-reference checks flagged this">
                            <Label isCompact color="orange" style={{ marginLeft: 6 }}>
                              {row.findings}
                            </Label>
                          </Tooltip>
                        )}
                      </Td>
                      {cols.map((c) => (
                        <Td key={c} dataLabel={c}>
                          <ColumnValue name={c} value={row.columns?.[c]} />
                        </Td>
                      ))}
                      <Td dataLabel="Status">
                        {/* Conditions as chips: what an operator scans, with the
                            reason and message in the tooltip rather than a wall
                            of text in a cell. */}
                        <ConditionChips conditions={row.conditions} />
                      </Td>
                      <Td dataLabel="Labels">
                        <KeyValueChips values={row.labels} max={2} />
                      </Td>
                      <Td dataLabel="Age">{age(row.created)}</Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            )}
          </StackItem>
        </Stack>
      </PageSection>
    </>
  )
}

export function ConfigDetailPage() {
  const { kind = '', name = '' } = useParams()
  const { data, isLoading, error } = useDetail(kind, name)
  const [tab, setTab] = useState<string | number>(0)
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorState title="Not found">{String(error)}</ErrorState>

  return (
    <>
      <Crumbs
        items={[
          { label: 'Configuration', to: '/config' },
          { label: kind, to: `/config/${kind}` },
          { label: name },
        ]}
      />
      <PageSection>
        <Stack hasGutter>
          <StackItem>
            <Title headingLevel="h1">
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 10 }}>
                <KindGlyph kind={kind} size={24} />
                <PlainText>{name}</PlainText>
                <HealthChip health={data.health} />
              </span>
            </Title>
          </StackItem>
          <StackItem>
            <Tabs activeKey={tab} onSelect={(_e, k) => setTab(k)}>
              <Tab eventKey={0} title={<TabTitleText>Overview</TabTitleText>}>
                <Stack hasGutter>
                  <StackItem>
                    <MetadataCard
                      meta={data.object.metadata}
                      health={data.health}
                      conditions={data.conditions}
                    />
                  </StackItem>
                  <StackItem>
                    <Card>
                      <CardTitle>Conditions</CardTitle>
                      <CardBody>
                        {(data.conditions ?? []).length === 0 ? (
                          <Empty title="This kind reports no conditions">
                            Nothing writes conditions here, so there is nothing to wait for.
                          </Empty>
                        ) : (
                          <Table variant="compact" aria-label="conditions">
                            <Thead>
                              <Tr>
                                <Th>Type</Th>
                                <Th>Status</Th>
                                <Th>Reason</Th>
                                <Th>Message</Th>
                                <Th>Since</Th>
                              </Tr>
                            </Thead>
                            <Tbody>
                              {(data.conditions ?? []).map((c) => (
                                <Tr key={c.type}>
                                  <Td dataLabel="Type">{c.type}</Td>
                                  <Td dataLabel="Status">
                                    <Label isCompact status={c.status === 'True' ? 'success' : 'danger'}>
                                      {c.status}
                                    </Label>
                                  </Td>
                                  <Td dataLabel="Reason">
                                    <PlainText>{c.reason}</PlainText>
                                  </Td>
                                  <Td dataLabel="Message">
                                    <PlainText multiline>{c.message}</PlainText>
                                  </Td>
                                  <Td dataLabel="Since">{age(c.lastTransitionTime)}</Td>
                                </Tr>
                              ))}
                            </Tbody>
                          </Table>
                        )}
                      </CardBody>
                    </Card>
                  </StackItem>

                  {data.findings.length > 0 && (
                    <StackItem>
                      <Card>
                        <CardTitle>Console checks</CardTitle>
                        <CardBody>
                          {/* Marked as the console's OWN cross-reference, never
                              presented as something the cluster reported. */}
                          <Table variant="compact" aria-label="findings">
                            <Thead>
                              <Tr>
                                <Th>Check</Th>
                                <Th>Reason</Th>
                                <Th>Message</Th>
                              </Tr>
                            </Thead>
                            <Tbody>
                              {data.findings.map((f, i) => (
                                <Tr key={`${f.check}-${i}`}>
                                  <Td dataLabel="Check">{f.check}</Td>
                                  <Td dataLabel="Reason">
                                    <PlainText>{f.reason}</PlainText>
                                  </Td>
                                  <Td dataLabel="Message">
                                    <PlainText multiline>{f.message}</PlainText>
                                  </Td>
                                </Tr>
                              ))}
                            </Tbody>
                          </Table>
                        </CardBody>
                      </Card>
                    </StackItem>
                  )}

                  <StackItem>
                    <Card>
                      <CardTitle>Used by</CardTitle>
                      <CardBody>
                        {(data.usedBy ?? []).length === 0 ? (
                          <Empty title="Nothing references this">
                            No other object points at it.
                          </Empty>
                        ) : (
                          <LabelGroup numLabels={20}>
                            {(data.usedBy ?? []).map((r) => (
                              <Label key={`${r.kind}/${r.name}/${r.field}`} isCompact>
                                <Link to={`/config/${r.kind}/${r.name}`}>
                                  {r.kind}/{r.name}
                                </Link>{' '}
                                ({r.field})
                              </Label>
                            ))}
                          </LabelGroup>
                        )}
                      </CardBody>
                    </Card>
                  </StackItem>

                  {kind === 'pipelines' && (
                    <StackItem>
                      <ResolvedCard detail={data} />
                    </StackItem>
                  )}
                </Stack>
              </Tab>
              <Tab eventKey={1} title={<TabTitleText>YAML</TabTitleText>}>
                <Yaml value={data.yaml} title={`${kind}/${name} YAML`} />
              </Tab>
            </Tabs>
          </StackItem>
        </Stack>
      </PageSection>
    </>
  )
}

/**
 * Resolved capabilities — the "what can this agent actually reach" answer.
 *
 * Rendered VERBATIM from the manager. The console does not recompute
 * composition: a second implementation would eventually disagree with the one
 * that runs, and the console's whole claim is that it cannot disagree with the
 * system.
 */
function ResolvedCard({ detail }: { detail: NonNullable<ReturnType<typeof useDetail>['data']> }) {
  return (
    <Card>
      <CardTitle>Resolved capabilities</CardTitle>
      <CardBody>
        {detail.resolvedError && (
          <Empty title="The manager could not resolve this pipeline">
            <PlainText>{detail.resolvedError}</PlainText>
          </Empty>
        )}
        {detail.resolved && (
          <DescriptionList isCompact isHorizontal>
            <DescriptionListGroup>
              <DescriptionListTerm>Runtime</DescriptionListTerm>
              <DescriptionListDescription>
                <PlainText>{detail.resolved.runtime}</PlainText>
              </DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Composition mode</DescriptionListTerm>
              <DescriptionListDescription>
                <Label isCompact color="purple">{detail.resolved.toolsMode}</Label>
              </DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Allowed tools</DescriptionListTerm>
              <DescriptionListDescription>
                {detail.resolved.allowedTools.length === 0 ? (
                  // An empty allowlist means EMPTY. A pipeline that grants no
                  // tools is a configuration, not a defect to paper over.
                  <em>none — this wiring grants no tools</em>
                ) : (
                  <CodeBlock>
                    <CodeBlockCode>{detail.resolved.allowedTools.join('\n')}</CodeBlockCode>
                  </CodeBlock>
                )}
                <small>
                  This is the wiring's half. The runtime composes it with the agent definition's own{' '}
                  <code>tools:</code> per the mode above.
                </small>
              </DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>MCP servers</DescriptionListTerm>
              <DescriptionListDescription>
                {detail.resolved.mcpServers.length === 0 ? (
                  <em>none</em>
                ) : (
                  <LabelGroup>
                    {detail.resolved.mcpServers.map((s) => (
                      <Label key={s} isCompact color="blue">
                        <PlainText>{s}</PlainText>
                      </Label>
                    ))}
                  </LabelGroup>
                )}
              </DescriptionListDescription>
            </DescriptionListGroup>
            {(detail.resolved.unresolved ?? []).length > 0 && (
              <DescriptionListGroup>
                <DescriptionListTerm>Unresolved</DescriptionListTerm>
                <DescriptionListDescription>
                  <LabelGroup>
                    {(detail.resolved.unresolved ?? []).map((u) => (
                      <Label key={u} isCompact status="danger">
                        <PlainText>{u}</PlainText>
                      </Label>
                    ))}
                  </LabelGroup>
                </DescriptionListDescription>
              </DescriptionListGroup>
            )}
          </DescriptionList>
        )}
      </CardBody>
    </Card>
  )
}
