// ONE COMPONENT, READ BY A FIXED SET OF WORKERS OVER A QUEUE OF FILES. Run
// inside a component's `read` job of `.github/workflows/claude-review.yml`:
// WORKERS `file-reviewer` subagents start at once, each with its own queue of
// the component's changed files, and each reads its rules ONCE and then takes
// its files one after another, returning one reading per file. The readings
// are merged here into the component's.
//
// WHY WORKERS AND NOT A SUBAGENT PER FILE. A subagent per file paid its
// orientation and its rule reading again for every file; a worker pays them
// once per queue. And a job per chunk of files paid ~30-40 s of checkout and
// session per chunk. The component is one job; the width inside it is the
// runtime's pool, which is two on a four-core runner.
//
// THE SCRIPT HOLDS THE LOOP, for the reason on record (#74, #77): a plan in a
// model turn is dropped with the turn. Every worker's return is validated
// against the schema at the tool layer; a file a worker did not return is
// named in `unread` — a gap by name, never a dropped file.
//
// args: { repo, number, base, component, kind, siblings: [...],
//         files: [{path, rules: [...], threads: [...]}], specPaths: [...] }
//         — assembled by `.github/scripts/review-prompt.py component`
export const meta = {
  name: 'review-component',
  description: 'Read one component of a pull request: workers over a queue of files, merged into one reading',
  whenToUse: 'Only from the review: the read job runs it with the component message as args',
  phases: [
    { title: 'Read', detail: 'workers, each over its queue of files, one file at a time' },
    { title: 'Merge', detail: 'findings, declares, references and verdicts into one reading' },
  ],
}

const input = typeof args === 'string' ? JSON.parse(args) : (args || {})
const { repo, number, base, component, kind, siblings = [], files = [], specPaths = [] } = input
if (!repo || !number || !base || !component) throw new Error('review-component needs args {repo, number, base, component, files}')

// The runtime's concurrent-agent pool on a four-core runner. More workers than
// that would queue behind each other; fewer would leave a slot idle.
const WORKERS = 2

const FILE_READING = {
  type: 'object', required: ['path', 'findings', 'declares', 'references', 'threads'],
  properties: {
    path: { type: 'string' },
    findings: { type: 'array', items: { type: 'object', required: ['path', 'line', 'claim'],
      properties: { path: { type: 'string' }, line: { type: 'integer' }, claim: { type: 'string' },
        where: { type: 'array', items: { type: 'string' } }, rule: { type: 'string' }, fix: { type: 'string' } } } },
    declares: { type: 'array', items: { type: 'string' } },
    references: { type: 'array', items: { type: 'string' } },
    threads: { type: 'array', items: { type: 'object', required: ['id', 'verdict'],
      properties: { id: { type: 'string' }, verdict: { type: 'string', enum: ['fixed', 'standing', 'gone', 'detached'] } } } },
  },
}
const WORKER_RETURN = { type: 'object', required: ['readings'], properties: { readings: { type: 'array', items: FILE_READING } } }

// Round-robin queues: every worker gets files from across the component, so
// one worker does not hold every large file.
const queues = Array.from({ length: Math.min(WORKERS, files.length) }, () => [])
files.forEach((f, i) => queues[i % queues.length].push(f))
const allPaths = files.map(f => f.path)

phase('Read')
const returns = await pipeline(queues, (queue, _, w) => {
  const rules = [...new Set(queue.flatMap(f => f.rules || []))]
  const others = allPaths.filter(p => !queue.some(f => f.path === p)).concat(siblings)
  return agent(
`REPO: ${repo}
PR NUMBER: ${number}
BASE REF: ${base}
COMPONENT: ${component} (${kind})
YOUR QUEUE — read these files ONE AT A TIME, in this order, and return one reading per file:
${queue.map((f, i) => `  ${i + 1}. ${f.path}`).join('\n')}
OTHER CHANGED FILES IN THIS COMPONENT (names only — do not read them):
${others.map(p => '  ' + p).join('\n') || '  none'}
THREADS ON YOUR FILES (your previous review — resolved and unresolved alike), by file:
${queue.map(f => `  ${f.path}: ${f.threads && f.threads.length ? JSON.stringify(f.threads) : 'none'}`).join('\n')}
DELTA SPECS OF THE CHANGE:
${specPaths.length ? specPaths.map(p => '  ' + p).join('\n') : '  none'}
RULE FILES TO READ ONCE, BEFORE YOUR FIRST FILE — these are what you judge every file against, and nothing else about the project's doctrine is in your context:
${rules.map(r => '  ' + r).join('\n') || '  none'}`,
    { agentType: 'file-reviewer', label: `worker ${w + 1}: ${queue.length} file(s)`, phase: 'Read', schema: WORKER_RETURN })
})

phase('Merge')
const readings = returns.filter(Boolean).flatMap(r => r.readings || [])
const byPath = new Map(readings.map(r => [r.path, r]))
const ok = allPaths.filter(p => byPath.has(p)).map(p => byPath.get(p))
const unread = allPaths.filter(p => !byPath.has(p))
log(`${ok.length} of ${files.length} file(s) read by ${queues.length} worker(s)` + (unread.length ? `; unread: ${unread.join(', ')}` : ''))
return {
  component,
  findings: ok.flatMap(r => r.findings),
  changedNames: [...new Set(ok.flatMap(r => r.declares))],
  files: ok.map(r => ({ path: r.path, declares: r.declares, references: r.references })),
  threads: ok.flatMap(r => r.threads),
  unread,
}
