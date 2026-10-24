// agentops claude agent runtime — entrypoint.
//
// Selects between the two drivers that implement the AgentRuntime /work
// contract (docs/contracts/runtime.md) for this image:
//   `cli` — driver-cli.js, one `claude` CLI spawn per work unit
//   `sdk` — driver-sdk.js, a session held open per pod (the default)
//
// RUNTIME_CLAUDE_DRIVER selects which. This file carries no business logic
// of its own — driver selection and logging only — so it is deliberately the
// ONE place `require.main === module` driven startup lives for the whole
// runtime; both drivers are always `require()`d, never the process entry
// point themselves.
'use strict';

const DEFAULT_DRIVER = 'sdk';
const KNOWN_DRIVERS = new Set(['cli', 'sdk']);

// resolveDriver turns the raw RUNTIME_CLAUDE_DRIVER value into one of the
// two known drivers, plus whether that value deserves a warning.
//
// Unset/empty is simply the default, not a mistake — nothing is logged.
// Anything else unrecognised falls back to the same default, but IS worth a
// warning: it is far more likely to be a typo than a value this image
// doesn't know about yet, and silence there would hide it.
function resolveDriver(envValue) {
  const v = String(envValue || '').trim();
  if (!v) return { driver: DEFAULT_DRIVER, warning: false };
  if (KNOWN_DRIVERS.has(v)) return { driver: v, warning: false };
  return { driver: DEFAULT_DRIVER, warning: true };
}

// start resolves the driver, logs the decision, and delegates. driver-sdk is
// required lazily, inside the branch that actually selects it, so that
// `cli`-driver usage never pays for — or breaks on — a module the sdk driver
// alone needs.
function start() {
  const raw = process.env.RUNTIME_CLAUDE_DRIVER;
  const { driver, warning } = resolveDriver(raw);
  if (warning) {
    console.warn(`[runtime] unrecognized RUNTIME_CLAUDE_DRIVER=${raw} — falling back to ${DEFAULT_DRIVER}`);
  }
  console.log(`[runtime] driver=${driver}`);
  if (driver === 'cli') return require('./driver-cli').start();
  return require('./driver-sdk').start();
}

module.exports = { resolveDriver, DEFAULT_DRIVER };

if (require.main === module) {
  // javascript:S7785 wants top-level `await` here instead. Real top-level
  // await is an ESM feature and is a SyntaxError in a CommonJS module (this
  // file has no package.json, so CommonJS is Node's default) -- converting
  // this file and everything it requires to ESM is a far larger change than
  // this line justifies. NOSONAR rather than a real fix.
  void start(); // NOSONAR
}
