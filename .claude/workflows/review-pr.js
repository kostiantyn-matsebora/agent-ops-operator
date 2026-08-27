// THE REVIEW'S PLAN, AS A SCRIPT. `/review-pr` reads a pull request per
// changed component — one reader per component, concurrently, each in its own
// context — and hands every reading to one coordinator, which is the only role
// that posts. The script holds the loop and the wait: no model decides what to
// spawn next, and nothing can end a turn with a reader still running. Both
// happened when the plan was a prompt (pull requests #74 and #77).
//
// Roles: `.claude/agents/component-reviewer.md`, `.claude/agents/review-coordinator.md`.
// The review job restores this file and both roles from the base branch before
// running, so a pull request cannot rewrite the review that judges it.
//
// args: { repo: "owner/name", number: <pr>, base: "origin/master",
//         dryRun?: true }   dryRun runs the readers and skips the coordinator —
//                           nothing is posted; the readings are returned.
export const meta = {
  name: 'review-pr',
  description: 'Review a pull request per component, then consolidate and post once',
  whenToUse: 'The automated review of a pull request: /review-pr with {repo, number, base}',
  phases: [
    { title: 'Queue', detail: 'changed paths → components; every review thread once' },
    { title: 'Read', detail: 'one component-reviewer per component, concurrently' },
    { title: 'Consolidate', detail: 'reach, dedup, post inline findings and one summary' },
  ],
}

// The caller may hand args as an object or as a JSON string; both were seen.
const input = typeof args === 'string' ? JSON.parse(args) : (args || {})
const { repo, number, base = 'origin/master', dryRun = false } = input
if (!repo || !number) throw new Error('review-pr needs args {repo, number}')

const THREAD = {
  type: 'object', required: ['id', 'path', 'isResolved'],
  properties: {
    id: { type: 'string' }, path: { type: 'string' }, line: { type: ['integer', 'null'] },
    isResolved: { type: 'boolean' }, isOutdated: { type: 'boolean' },
    commentId: { type: ['integer', 'null'] }, author: { type: 'string' }, body: { type: 'string' },
  },
}
const QUEUE = {
  type: 'object', required: ['paths', 'queue', 'threads', 'specPaths'],
  properties: {
    paths: { type: 'array', items: { type: 'string' } },
    queue: { type: 'array', items: { type: 'object', required: ['group', 'kind', 'paths'],
      properties: { group: { type: 'string' }, kind: { type: 'string' }, paths: { type: 'array', items: { type: 'string' } } } } },
    threads: { type: 'array', items: THREAD },
    specPaths: { type: 'array', items: { type: 'string' } },
  },
}
// The reader's stated return, validated at the tool layer: a prose return is
// retried by the runtime, then null — and null is a named gap in the summary.
const FINDINGS = {
  type: 'object', required: ['component', 'findings', 'changedNames', 'threads'],
  properties: {
    component: { type: 'string' },
    findings: { type: 'array', items: { type: 'object', required: ['path', 'line', 'claim'],
      properties: { path: { type: 'string' }, line: { type: 'integer' }, claim: { type: 'string' },
        where: { type: 'array', items: { type: 'string' } }, rule: { type: 'string' }, fix: { type: 'string' } } } },
    changedNames: { type: 'array', items: { type: 'string' } },
    threads: { type: 'array', items: { type: 'object', required: ['id', 'verdict'],
      properties: { id: { type: 'string' }, verdict: { type: 'string', enum: ['fixed', 'standing', 'gone', 'detached'] } } } },
  },
}
const OUTCOME = {
  type: 'object', required: ['summaryPosted'],
  properties: { summaryPosted: { type: 'boolean' }, inline: { type: 'integer' },
    resolved: { type: 'integer' }, unreviewed: { type: 'array', items: { type: 'string' } } },
}

phase('Queue')
const [owner, name] = repo.split('/')
const q = await agent(
`REPO: ${repo}
PR NUMBER: ${number}
BASE REF: ${base}

Build the review's queue and fetch its threads. Run exactly these, then return the data.

1. Changed paths:  gh pr diff ${number} --name-only
2. The queue:      python3 .github/scripts/review-queue.py <those paths as arguments>
   (one entry per component: {group, kind, paths})
3. Every review thread, once:
   gh api graphql -f query='query($o:String!,$r:String!,$n:Int!){
     repository(owner:$o,name:$r){pullRequest(number:$n){
       reviewThreads(first:100){nodes{id isResolved isOutdated path line
         comments(first:1){nodes{databaseId author{login} body}}}}}}}' \\
     -f o=${owner} -f r=${name} -F n=${number}
   Flatten each node to {id, path, line, isResolved, isOutdated, commentId, author, body}.
4. specPaths: if the head branch is change/<name>, the files under openspec/changes/<name>/specs/ (git ls-files); else [].

Return {paths, queue, threads, specPaths}. Read nothing else; post nothing.`,
  { label: 'queue', phase: 'Queue', schema: QUEUE })
if (!q) throw new Error('the queue agent returned nothing')
log(`${q.queue.length} component(s) from ${q.paths.length} path(s), ${q.threads.length} thread(s)`)

phase('Read')
const readings = await pipeline(q.queue, entry => {
  const own = q.threads.filter(t => entry.paths.includes(t.path))
  return agent(
`REPO: ${repo}
PR NUMBER: ${number}
BASE REF: ${base}
COMPONENT: ${entry.group} (${entry.kind})
PATHS:
${entry.paths.map(p => '  ' + p).join('\n')}
THREADS ON THESE PATHS (your previous review — resolved and unresolved alike):
${own.length ? JSON.stringify(own, null, 1) : '  none'}
DELTA SPECS OF THE CHANGE:
${q.specPaths.length ? q.specPaths.map(p => '  ' + p).join('\n') : '  none'}`,
    { agentType: 'component-reviewer', label: entry.group, phase: 'Read', schema: FINDINGS })
})
const reviewed = readings.filter(Boolean).length
const unreviewed = q.queue.filter((_, i) => !readings[i]).map(e => e.group)
log(`${reviewed} of ${q.queue.length} component(s) read` + (unreviewed.length ? `; unreviewed: ${unreviewed.join(', ')}` : ''))

if (dryRun) return { queue: q.queue, readings, unreviewed, dryRun: true }

phase('Consolidate')
const outcome = await agent(
`REPO: ${repo}
PR NUMBER: ${number}
BASE REF: ${base}
CHANGED PATHS:
${q.paths.map(p => '  ' + p).join('\n')}
REVIEW THREADS:
${JSON.stringify(q.threads, null, 1)}
READINGS (one per component, null = unreviewed):
${JSON.stringify(q.queue.map((e, i) => ({ group: e.group, reading: readings[i] })), null, 1)}`,
  { agentType: 'review-coordinator', label: 'coordinator', phase: 'Consolidate', schema: OUTCOME })
if (!outcome) throw new Error('the coordinator returned nothing')
log(`summary posted: ${outcome.summaryPosted}; inline: ${outcome.inline ?? 0}; resolved: ${outcome.resolved ?? 0}`)
return { components: q.queue.length, reviewed, unreviewed, ...outcome }
