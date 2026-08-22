import { describe, expect, it } from 'vitest'
import { matchEntries } from './NewConversation'
import type { VocabularyEntry } from '../api/types'

// The typeahead exists because a source is shareable: with several Pipelines
// serving one surface, an unaddressed task is refused, so the composer has to
// offer what can be addressed rather than expect a name to be recalled.

const entries: VocabularyEntry[] = [
  { kind: 'pipeline', name: 'ha-control', position: 'general', profile: 'ha-user' },
  { kind: 'pipeline', name: 'ha-ops', position: 'general', profile: 'ha-admin' },
  { kind: 'pipeline', name: 'k8s-ops', position: 'general', profile: 'k8s-engineer' },
  // Valid only inside a conversation. This composer starts one, so neither of
  // these may ever be offered here.
  { kind: 'builtin', name: 'exit', position: 'thread', description: 'Release the runtime' },
  { kind: 'builtin', name: 'close', position: 'thread', description: 'End the conversation' },
]

describe('matchEntries', () => {
  it('offers every pipeline on the bare prefix', () => {
    expect(matchEntries('/', entries, 'general')).toHaveLength(3)
  })

  it('narrows as the name is typed', () => {
    expect(matchEntries('/ha', entries, 'general')?.map((a) => a.name)).toEqual(['ha-control', 'ha-ops'])
    expect(matchEntries('/ha-o', entries, 'general')?.map((a) => a.name)).toEqual(['ha-ops'])
  })

  it('is case-insensitive, because a name is not a password', () => {
    expect(matchEntries('/HA-O', entries, 'general')?.map((a) => a.name)).toEqual(['ha-ops'])
  })

  it('shows NOTHING rather than an empty box when no name matches', () => {
    // An empty popup is worse than no popup: it covers the field and says
    // nothing. Same answer for a surface with no Ready pipelines at all.
    expect(matchEntries('/nope', entries, 'general')).toBeNull()
    expect(matchEntries('/', [], 'general')).toBeNull()
  })

  it('ignores a slash that is not addressing anyone', () => {
    // Mid-sentence slashes are paths and dates. Only the start of the message
    // addresses a pipeline, so only that position opens the menu.
    expect(matchEntries('check /var/log on node-1', entries, 'general')).toBeNull()
    expect(matchEntries('is 3/4 of the disk used?', entries, 'general')).toBeNull()
  })

  it('closes once the name is finished and the task has begun', () => {
    expect(matchEntries('/ha-ops ', entries, 'general')).toBeNull()
    expect(matchEntries('/ha-ops restart the api', entries, 'general')).toBeNull()
  })

  it('offers only what the server returned, which is Ready-filtered', () => {
    // The filtering itself is the BFF's job (handleVocabulary), so that the
    // typeahead and the listing command can never give different answers. Here we only
    // pin that nothing is invented client-side.
    expect(
      matchEntries('/', [{ kind: 'pipeline', name: 'only-ready', position: 'general' }], 'general')
        ?.map((e) => e.name),
    ).toEqual(['only-ready'])
  })

  it('offers only what is valid WHERE the person is typing', () => {
    // The two composers take disjoint sets. Addressing a Pipeline inside a
    // thread is input for the agent, and the commands that end or release a
    // conversation have nothing to act on outside one — so offering the wrong
    // half puts a command in front of somebody at the one place it does
    // nothing.
    expect(matchEntries('/', entries, 'general')?.map((e) => e.name)).toEqual([
      'ha-control',
      'ha-ops',
      'k8s-ops',
    ])
    expect(matchEntries('/', entries, 'thread')?.map((e) => e.name)).toEqual(['exit', 'close'])
  })

  it('never offers a pipeline inside a thread', () => {
    expect(matchEntries('/ha', entries, 'thread')).toBeNull()
  })
})
