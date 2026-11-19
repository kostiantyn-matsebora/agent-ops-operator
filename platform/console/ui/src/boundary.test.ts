import { describe, expect, it } from 'vitest'

// THE LAYER BOUNDARY, ASSERTED RATHER THAN STATED (design D2).
//
//   1. model/ imports no component library, no stylesheet and no look.
//   2. A look never imports api/client.
//   3. A look never imports another look. looks/shared/ is the one folder two
//      looks may share, and it imports no look either.
//
// Walked over the real import graph of src/, read at build time the way
// Text.test.tsx reads it, so this needs no Node types.

type Sources = Record<string, string>

/** Packages that draw: a model file importing one is presentation in the model. */
const PRESENTATION = [/^@patternfly\//, /^react-dom(\/|$)/, /^react-markdown$/, /^rehype-/, /^remark-/, /^highlight\.js/, /^victory/]
const STYLESHEET = /\.(css|scss|sass|less)(\?|$)/

const SPECIFIER = [
  /(?:^|[\s;])(?:import|export)\s[^'"`;]*?\bfrom\s*['"]([^'"]+)['"]/g, // import x from 'y', export { x } from 'y'
  /(?:^|[\s;])import\s*['"]([^'"]+)['"]/g, // import 'side-effect.css'
  /\bimport\(\s*['"]([^'"]+)['"]\s*\)/g, // import('lazy')
]

/** Strips comments, so a sentence that mentions an import is not one. */
function code(body: string): string {
  return body.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:'"])\/\/.*$/gm, '$1')
}

export function specifiers(body: string): string[] {
  const text = code(body)
  return SPECIFIER.flatMap((re) => [...text.matchAll(re)].map((m) => m[1]))
}

function normalize(parts: string[]): string {
  const out: string[] = []
  for (const p of parts) {
    if (p === '..') out.pop()
    else if (p !== '.' && p !== '') out.push(p)
  }
  return out.join('/')
}

/** A relative specifier as a path under src/, or undefined for a package. */
function resolve(from: string, spec: string, files: Set<string>): string | undefined {
  if (!spec.startsWith('.')) return undefined
  const bare = spec.split('?')[0]
  const base = normalize([...from.split('/').slice(0, -1), ...bare.split('/')])
  for (const candidate of [base, `${base}.ts`, `${base}.tsx`, `${base}/index.ts`, `${base}/index.tsx`]) {
    if (files.has(candidate)) return candidate
  }
  return base
}

/** looks/<name>/… → name. A file directly under looks/ is the seam, not a look. */
function lookOf(path: string): string | undefined {
  const m = /^looks\/([^/]+)\//.exec(path)
  return m ? m[1] : undefined
}

const isTest = (path: string) => /\.test\.tsx?$/.test(path)

/** Every boundary violation in a set of sources keyed by their path under src/. */
export function violations(sources: Sources): string[] {
  const files = new Set(Object.keys(sources))
  const found: string[] = []
  for (const [file, body] of Object.entries(sources)) {
    const inModel = file.startsWith('model/')
    const look = lookOf(file)
    if (inModel && file.endsWith('.tsx') && !isTest(file)) found.push(`${file}: model file holds JSX`)
    for (const spec of specifiers(body)) {
      const target = resolve(file, spec, files)
      if (inModel) {
        if (PRESENTATION.some((re) => re.test(spec))) found.push(`${file}: model imports component library ${spec}`)
        if (STYLESHEET.test(spec)) found.push(`${file}: model imports stylesheet ${spec}`)
        if (target?.startsWith('looks/')) found.push(`${file}: model imports look ${spec}`)
      }
      if (look && target) {
        const other = lookOf(target)
        if (other && other !== look && other !== 'shared') found.push(`${file}: look ${look} imports look ${other} (${spec})`)
        if (target === 'api/client.ts') found.push(`${file}: look imports api/client`)
      }
    }
  }
  return found.sort()
}

const raw = import.meta.glob('./**/*.{ts,tsx}', { eager: true, query: '?raw', import: 'default' }) as Sources
const tree: Sources = Object.fromEntries(Object.entries(raw).map(([k, v]) => [k.replace(/^\.\//, ''), v]))

describe('the layer boundary', () => {
  it('reads the whole source tree', () => {
    // A glob that matched nothing would make every assertion below vacuous.
    expect(Object.keys(tree).length).toBeGreaterThan(100)
    expect(tree['looks/registry.ts']).toBeDefined()
  })

  it('holds across src/', () => {
    expect(violations(tree)).toEqual([])
  })

})

describe('the boundary check itself', () => {
  const ok = { 'api/client.ts': '', 'api/types.ts': '', 'looks/shared/LookSwitcher.tsx': '', 'looks/workspace/x.ts': '' }

  it('names a model file that imports a component library, a stylesheet or a look', () => {
    expect(
      violations({
        ...ok,
        'model/a.ts': "import { Button } from '@patternfly/react-core'",
        'model/b.ts': "import './b.css'",
        'model/c.ts': "import {\n  x,\n} from '../looks/workspace/x'",
        'model/d.ts': "export { y } from 'react-markdown'",
      }),
    ).toEqual([
      'model/a.ts: model imports component library @patternfly/react-core',
      'model/b.ts: model imports stylesheet ./b.css',
      'model/c.ts: model imports look ../looks/workspace/x',
      'model/d.ts: model imports component library react-markdown',
    ])
  })

  it('names a model file that holds JSX', () => {
    expect(violations({ 'model/Card.tsx': '' })).toEqual(['model/Card.tsx: model file holds JSX'])
  })

  it('lets a model import react, the api and other model files', () => {
    expect(
      violations({
        ...ok,
        'model/a.ts': "import { useState } from 'react'\nimport type { Session } from '../api/types'\nimport { b } from './b'",
        'model/b.ts': '',
        'model/a.test.tsx': "import { renderHook } from '@testing-library/react'",
      }),
    ).toEqual([])
  })

  it('names a look that imports another look, lazily or not, but not one that imports shared', () => {
    expect(
      violations({
        ...ok,
        'looks/classic/a.tsx': "import { x } from '../workspace/x'\nimport { L } from '../shared/LookSwitcher'",
        'looks/classic/b.tsx': "const m = import('../workspace/x')",
        'looks/shared/c.tsx': "import { x } from '../workspace/x'",
      }),
    ).toEqual([
      'looks/classic/a.tsx: look classic imports look workspace (../workspace/x)',
      'looks/classic/b.tsx: look classic imports look workspace (../workspace/x)',
      'looks/shared/c.tsx: look shared imports look workspace (../workspace/x)',
    ])
  })

  it('names every look that imports api/client, classic included', () => {
    expect(
      violations({
        ...ok,
        'looks/workspace/y.tsx': "import { api } from '../../api/client'\nimport type { S } from '../../api/types'",
        'looks/classic/App.tsx': "import { api } from '../../api/client'",
      }),
    ).toEqual(['looks/classic/App.tsx: look imports api/client', 'looks/workspace/y.tsx: look imports api/client'])
  })

  it('ignores an import mentioned only in a comment', () => {
    expect(violations({ ...ok, 'model/a.ts': "// import { Button } from '@patternfly/react-core'\n/* import 'x.css' */" })).toEqual([])
  })
})
