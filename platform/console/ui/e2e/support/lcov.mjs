// Fold every test's counters in .nyc_output/ into coverage/e2e/lcov.info.
// Run after `E2E_COVERAGE=1 playwright test` (npm run test:e2e:coverage does both).
import { mkdirSync, readFileSync, readdirSync, existsSync } from 'node:fs'
import { join } from 'node:path'
import libCoverage from 'istanbul-lib-coverage'
import libReport from 'istanbul-lib-report'
import reports from 'istanbul-reports'

const dir = join(process.cwd(), '.nyc_output')
const out = join(process.cwd(), 'coverage', 'e2e')

if (!existsSync(dir)) {
  // NO DATA IS A FAILURE, NEVER "NOTHING CHANGED": the station reads a missing file as a collection failure.
  console.error('no coverage counters in .nyc_output/: the build was not instrumented (E2E_COVERAGE=1) or no test loaded the app')
  process.exit(1)
}
const map = libCoverage.createCoverageMap({})
for (const f of readdirSync(dir).filter((n) => n.endsWith('.json'))) {
  map.merge(JSON.parse(readFileSync(join(dir, f), 'utf8')))
}
if (map.files().length === 0) {
  console.error('the counters held no file')
  process.exit(1)
}
mkdirSync(out, { recursive: true })
const context = libReport.createContext({ dir: out, coverageMap: map })
reports.create('lcovonly').execute(context)
console.log(`wrote ${join(out, 'lcov.info')} for ${map.files().length} files`)
