// ONE COMPONENT, READ PER FILE. Run inside a component's `read` job of
// `.github/workflows/claude-review.yml`: one `file-reviewer` per changed file,
// each in its own context holding that file, its threads and the rules for
// its path — and nothing else — merged here into the component's reading.
//
// THE SCRIPT HOLDS THE LOOP, for the reason on record (#74, #77): a plan in a
// model turn is dropped with the turn. Every file reader's return is validated
// against the schema at the tool layer; one that returns prose is retried,
// then null, and null is a file named in `unread` — a gap by name, never a
// dropped file.
//
// TWO AT A TIME. The runtime pools agent() calls at min(16, max(2, CPUs-2)),
// which on a four-core runner is two, and that is not fought: the width of the
// review is the matrix across components. What this buys is a context that
// does not grow with the diff.
//
// args: { repo, number, base, component, kind, files: [{path, rules: [...], threads: [...]}],
//         specPaths: [...] }   — assembled by `.github/scripts/review-prompt.py component`
export const meta = {
  name: 'review-component',
  description: 'Read one component of a pull request per file, and merge the readings',
  whenToUse: 'Only from the review: the read job runs it with the component message as args',
  phases: [
    { title: 'Read', detail: 'one file-reviewer per changed file, two at a time' },
    { title: 'Merge', detail: 'findings, declares, references and verdicts into one reading' },
  ],
}

const input = typeof args === 'string' ? JSON.parse(args) : (args || {})
const { repo, number, base, component, kind, files = [], specPaths = [] } = input
if (!repo || !number || !component) throw new Error('review-component needs args {repo, number, base, component, files}')

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

phase('Read')
const readings = await pipeline(files, f => agent(
`REPO: ${repo}
PR NUMBER: ${number}
BASE REF: ${base}
FILE: ${f.path}
COMPONENT: ${component} (${kind})
OTHER CHANGED FILES IN THIS COMPONENT (names only — do not read them):
${files.filter(o => o.path !== f.path).map(o => '  ' + o.path).join('\n') || '  none'}
THREADS ON THIS FILE (your previous review — resolved and unresolved alike):
${f.threads && f.threads.length ? JSON.stringify(f.threads, null, 1) : '  none'}
DELTA SPECS OF THE CHANGE:
${specPaths.length ? specPaths.map(p => '  ' + p).join('\n') : '  none'}
RULE FILES TO READ FOR THIS PATH — these are what you judge against, and nothing else about the project's doctrine is in your context:
${(f.rules || []).map(r => '  ' + r).join('\n') || '  none'}`,
  { agentType: 'file-reviewer', label: f.path, phase: 'Read', schema: FILE_READING }))

phase('Merge')
const read = readings.map((r, i) => ({ file: files[i].path, reading: r }))
const unread = read.filter(x => !x.reading).map(x => x.file)
const ok = read.filter(x => x.reading).map(x => x.reading)
const changedNames = [...new Set(ok.flatMap(r => r.declares))]
log(`${ok.length} of ${files.length} file(s) read` + (unread.length ? `; unread: ${unread.join(', ')}` : ''))
return {
  component,
  findings: ok.flatMap(r => r.findings),
  changedNames,
  files: ok.map(r => ({ path: r.path, declares: r.declares, references: r.references })),
  threads: ok.flatMap(r => r.threads),
  unread,
}
