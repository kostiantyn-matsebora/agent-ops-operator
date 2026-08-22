import type { ReactNode } from 'react'
import { Card, CardBody, CardTitle, DescriptionList, DescriptionListDescription,
  DescriptionListGroup, DescriptionListTerm, Label } from '@patternfly/react-core'
import { Link } from 'react-router-dom'
import type { GraphNode } from '../api/types'
import { PlainText } from '../components/Text'
import { healthVariant } from '../api/hooks'

/**
 * The side panel for a selected node.
 *
 * It shows the reason VERBATIM. An unclaimed source's whole value on this graph
 * is the Wired=False message a reconciler wrote — paraphrasing it here would put
 * the console's words in the cluster's mouth.
 */
export function NodeDetails({ node, extras }: { node: GraphNode; extras?: ReactNode }) {
  return (
    <Card isCompact>
      <CardTitle>
        <PlainText>{node.name}</PlainText>
      </CardTitle>
      <CardBody>
        <DescriptionList isCompact>
          <DescriptionListGroup>
            <DescriptionListTerm>Kind</DescriptionListTerm>
            <DescriptionListDescription>{node.kind}</DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Health</DescriptionListTerm>
            <DescriptionListDescription>
              {node.health === 'none' ? (
                <span>reports none</span>
              ) : (
                <Label status={healthVariant(node.health)}>{node.health}</Label>
              )}
            </DescriptionListDescription>
          </DescriptionListGroup>
          {node.reason && (
            <DescriptionListGroup>
              <DescriptionListTerm>Reason</DescriptionListTerm>
              <DescriptionListDescription>
                <PlainText>{node.reason}</PlainText>
              </DescriptionListDescription>
            </DescriptionListGroup>
          )}
          {node.message && (
            <DescriptionListGroup>
              <DescriptionListTerm>Message</DescriptionListTerm>
              <DescriptionListDescription>
                <PlainText multiline>{node.message}</PlainText>
              </DescriptionListDescription>
            </DescriptionListGroup>
          )}
          {node.detached && (
            <DescriptionListGroup>
              <DescriptionListTerm>Detached</DescriptionListTerm>
              <DescriptionListDescription>
                nothing in the wiring references this
              </DescriptionListDescription>
            </DescriptionListGroup>
          )}
          {(node.active > 0 || node.recent > 0) && (
            <DescriptionListGroup>
              <DescriptionListTerm>Conversations</DescriptionListTerm>
              <DescriptionListDescription>
                {node.active} active, {node.recent} recent
              </DescriptionListDescription>
            </DescriptionListGroup>
          )}
          <DescriptionListGroup>
            <DescriptionListTerm>Object</DescriptionListTerm>
            <DescriptionListDescription>
              <Link to={`/config/${node.kind}/${node.name}`}>open</Link>
            </DescriptionListDescription>
          </DescriptionListGroup>
        </DescriptionList>
        {extras}
      </CardBody>
    </Card>
  )
}
