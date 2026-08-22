import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Yaml, tokenizeLine } from './Yaml'

// The bug this component exists to fix: the previous viewer was a single-line
// input, so every newline collapsed and a Kubernetes object rendered as one
// unreadable ribbon. YAML is whitespace-significant — a viewer that eats the
// whitespace has destroyed the only thing that made it readable.

const DOC = `apiVersion: agentops.dev/v1alpha1
kind: MCPToolset
metadata:
  name: k8s-admin
  namespace: agent-ops
spec:
  tools:
    - mcp__kubernetes__resources_create_or_update
    - mcp__kubernetes__pods_delete
  enabled: true
  replicas: 3
`

describe('tokenizeLine', () => {
  it('separates keys from values', () => {
    expect(tokenizeLine('  name: k8s-admin')).toEqual([
      { kind: 'text', text: '  ' },
      { kind: 'key', text: 'name' },
      { kind: 'punct', text: ':' },
      { kind: 'text', text: ' ' },
      { kind: 'string', text: 'k8s-admin' },
    ])
  })

  it('classifies booleans and numbers apart from strings', () => {
    expect(tokenizeLine('  enabled: true').at(-1)).toEqual({ kind: 'literal', text: 'true' })
    expect(tokenizeLine('  replicas: 3').at(-1)).toEqual({ kind: 'number', text: '3' })
    expect(tokenizeLine('  name: 3-not-a-number').at(-1)).toEqual({
      kind: 'string',
      text: '3-not-a-number',
    })
  })

  it('handles sequence items and comments', () => {
    expect(tokenizeLine('    - mcp__kubernetes__pods_delete')).toEqual([
      { kind: 'text', text: '    ' },
      { kind: 'punct', text: '- ' },
      { kind: 'string', text: 'mcp__kubernetes__pods_delete' },
    ])
    expect(tokenizeLine('# a note')).toEqual([{ kind: 'comment', text: '# a note' }])
  })

  it('marks block scalars so a multi-line value is not mistaken for a string', () => {
    expect(tokenizeLine('  message: |-').at(-1)).toEqual({ kind: 'punct', text: '|-' })
  })
})

describe('Yaml', () => {
  it('preserves every line and its indentation', () => {
    render(<Yaml value={DOC} />)
    const pre = screen.getByTestId('yaml')
    // the whole document survives, newlines included
    expect(pre.textContent).toContain('kind: MCPToolset')
    expect(pre.textContent).toContain('  name: k8s-admin')
    expect(pre.textContent).toContain('    - mcp__kubernetes__pods_delete')
  })

  it('numbers the lines', () => {
    render(<Yaml value={DOC} />)
    const pre = screen.getByTestId('yaml')
    // 10 content lines, so a gutter that counts to 10
    expect(pre.textContent).toMatch(/1apiVersion/)
    expect(pre.textContent).toMatch(/10/)
  })

  it('renders as text, never as markup', () => {
    const { container } = render(<Yaml value={'note: <b>not bold</b>\n'} />)
    expect(container.querySelector('b')).toBeNull()
    expect(screen.getByTestId('yaml').textContent).toContain('<b>not bold</b>')
  })
})
