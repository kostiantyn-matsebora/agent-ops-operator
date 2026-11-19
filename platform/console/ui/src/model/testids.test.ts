import { describe, expect, it } from 'vitest'
import { ANY_MEMBER_INVOCATION, ANY_ROW, TESTID, testid } from './testids'

// The stable test identifiers are a contract: unique, and the only way an
// end-to-end spec selects (console-looks: "A selector a spec relies on SHALL
// be a stable test identifier supplied by the model layer's contract, not a
// class or a component name").

describe('the identifiers', () => {
  it('are unique', () => {
    const values = Object.values(TESTID)
    expect(new Set(values).size).toBe(values.length)
  })

  it('a keyed identifier never collides with a fixed one', () => {
    const keyed = Object.values(testid).map((f) => f('x'))
    for (const k of keyed) expect(Object.values(TESTID)).not.toContain(k)
    expect(new Set(keyed).size).toBe(keyed.length)
  })

  it('the patterns match only what they name', () => {
    expect(ANY_ROW.test(testid.row('a'))).toBe(true)
    for (const fixed of Object.values(TESTID)) expect(ANY_ROW.test(fixed)).toBe(false)
    expect(ANY_MEMBER_INVOCATION.test(testid.memberInvocation('a'))).toBe(true)
    expect(ANY_MEMBER_INVOCATION.test(TESTID.memberInvocationToggle)).toBe(false)
  })
})

// The live specs, read as text — they run against a cluster, so this is the
// check that holds them to the contract on every unit run.
const specs = import.meta.glob('../../e2e-live/*.spec.ts', { eager: true, query: '?raw', import: 'default' }) as Record<string, string>

function code(body: string): string {
  return body.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:'"`])\/\/.*$/gm, '$1')
}

describe('the live specs select through the contract', () => {
  it('reads every spec', () => {
    expect(Object.keys(specs).length).toBeGreaterThan(20)
  })

  it('never by a literal test id, a class, an element id or a component’s element', () => {
    const found: string[] = []
    for (const [path, body] of Object.entries(specs)) {
      const text = code(body)
      const rules: [RegExp, string][] = [
        [/getByTestId\(\s*['"`]/, 'a literal test id'],
        [/locator\(\s*['"`]\s*[.#]/, 'a class or element id'],
        [/\[class[*^$~|]?=/, 'a class attribute'],
        [/\.pf-v\d/, 'a component library class'],
        [/locator\(\s*['"`](li|ul|div|span|details|summary|article|section|input)\b/, 'a component’s element'],
      ]
      for (const [re, what] of rules) if (re.test(text)) found.push(`${path.replace('../../', '')}: ${what}`)
    }
    expect(found).toEqual([])
  })
})
