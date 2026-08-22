import { Breadcrumb, BreadcrumbItem, PageBreadcrumb } from '@patternfly/react-core'
import { Link } from 'react-router-dom'
import { PlainText } from './Text'

// Breadcrumbs.
//
// Every detail view in this console is reached from somewhere — a graph node, a
// problem in the overview rollup, a queue row, an inventory table — and several
// of those cross pages. Browser Back handles one hop; a trail says where you
// ARE, which matters when the hop came from a link you did not choose.
//
// The last crumb is deliberately not a link: it is the page you are on.

export interface Crumb {
  label: string
  to?: string
}

export function Crumbs({ items }: { items: Crumb[] }) {
  return (
    <PageBreadcrumb hasBodyWrapper={false}>
      <Breadcrumb>
        {items.map((c, i) => {
          const last = i === items.length - 1
          return (
            <BreadcrumbItem key={`${c.label}-${i}`} isActive={last}>
              {c.to && !last ? (
                <Link to={c.to}>
                  <PlainText>{c.label}</PlainText>
                </Link>
              ) : (
                <PlainText>{c.label}</PlainText>
              )}
            </BreadcrumbItem>
          )
        })}
      </Breadcrumb>
    </PageBreadcrumb>
  )
}
