import { describe, expect, it } from 'vitest'
import { matchAgents } from './NewConversation'
import type { Agent } from '../api/types'

// The typeahead exists because a source is shareable: with several Pipelines
// serving one surface, an unaddressed task is refused, so the composer has to
// offer what can be addressed rather than expect a name to be recalled.

const agents: Agent[] = [
  { name: 'ha-control', profile: 'ha-user' },
  { name: 'ha-ops', profile: 'ha-admin' },
  { name: 'k8s-ops', profile: 'k8s-engineer' },
]

describe('matchAgents', () => {
  it('offers every agent on the bare prefix', () => {
    expect(matchAgents('/', agents)).toHaveLength(3)
  })

  it('narrows as the name is typed', () => {
    expect(matchAgents('/ha', agents)?.map((a) => a.name)).toEqual(['ha-control', 'ha-ops'])
    expect(matchAgents('/ha-o', agents)?.map((a) => a.name)).toEqual(['ha-ops'])
  })

  it('is case-insensitive, because a name is not a password', () => {
    expect(matchAgents('/HA-O', agents)?.map((a) => a.name)).toEqual(['ha-ops'])
  })

  it('shows NOTHING rather than an empty box when no name matches', () => {
    // An empty popup is worse than no popup: it covers the field and says
    // nothing. Same answer for a surface with no Ready pipelines at all.
    expect(matchAgents('/nope', agents)).toBeNull()
    expect(matchAgents('/', [])).toBeNull()
  })

  it('ignores a slash that is not addressing anyone', () => {
    // Mid-sentence slashes are paths and dates. Only the start of the message
    // addresses a pipeline, so only that position opens the menu.
    expect(matchAgents('check /var/log on node-1', agents)).toBeNull()
    expect(matchAgents('is 3/4 of the disk used?', agents)).toBeNull()
  })

  it('closes once the name is finished and the task has begun', () => {
    expect(matchAgents('/ha-ops ', agents)).toBeNull()
    expect(matchAgents('/ha-ops restart the api', agents)).toBeNull()
  })

  it('offers only what the server returned, which is Ready-filtered', () => {
    // The filtering itself is the BFF's job (handleAgents), so that the
    // typeahead and `/agents` can never give different answers. Here we only
    // pin that nothing is invented client-side.
    expect(matchAgents('/', [{ name: 'only-ready' }])?.map((a) => a.name)).toEqual(['only-ready'])
  })
})
