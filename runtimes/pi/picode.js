'use strict';

// picode.js — the OPTIONAL `pi-code` package (npm:pi-code), which makes pi read
// a checkout's `.claude/` the way Claude Code does: CLAUDE.md and its @imports,
// path-scoped rules, commands, skills, output styles and hooks.
//
// It is ON by default and the chart's `pi.piCode` switches it off, through the
// `PI_CODE_ENABLED` env this file reads. Three facts bound what it can do here:
//
//   - NO TOOL IS ADDED TO A SESSION. `openSession` passes an explicit `tools:`
//     list, and pi's own `createAgentSession` makes that the initial active set
//     outright, so the tools pi-code registers (todo, web_fetch, subagent, ...)
//     are known to pi and never offered to the model.
//   - THE PERMISSION HOOK STILL ANSWERS EVERY `tool_call`, deny by default, so a
//     tool pi-code registered is refused even if something activated it.
//   - IT LOADS NOTHING FROM A PROJECT UNTIL THE PROJECT IS APPROVED, and a
//     headless run cannot ask. The workspace is the repository the AgentProfile
//     names, chosen by whoever wired the route, so this runtime records that
//     approval itself (`approveWorkspace`) instead of leaving the feature inert.
//     Hooks it then runs are shell commands from that repository.

const path = require('path');

// A value counts as "off" only when it says so. Unset, empty or anything else
// is on, so a runtime started without the chart's env still behaves as the
// default the bundle documents.
function piCodeEnabled(env = process.env) {
  const v = String(env.PI_CODE_ENABLED ?? 'true').trim().toLowerCase();
  return !['false', '0', 'no', 'off'].includes(v);
}

// The installed package's directory, which `DefaultResourceLoader` takes as an
// additional extension path and expands through the package's own `pi` manifest.
function piCodeDir() {
  return path.dirname(require.resolve('pi-code/package.json'));
}

// pi-code reads the agent directory through the SDK's own `getAgentDir()`,
// which honours PI_CODING_AGENT_DIR and otherwise defaults to ~/.pi/agent. The
// runtime states its agent directory explicitly (PI_AGENT_DIR), so the two must
// be one string or the approval below would be recorded where pi-code never
// looks.
function pointPiCodeAtAgentDir(agentDir, env = process.env) {
  env.PI_CODING_AGENT_DIR = agentDir;
}

// Records the workspace as approved in pi's own trust store, the store pi-code
// consults for a `.claude/`-shaped project.
function approveWorkspace(sdk, agentDir, workspace) {
  new sdk.ProjectTrustStore(agentDir).set(workspace, true);
}

// Everything buildResourceLoader needs from this feature, in one place:
// `{ additionalExtensionPaths }` to spread into the loader's options, or an
// empty object when the feature is off.
function loaderOptions(sdk, { agentDir, workspace, env = process.env }) {
  if (!piCodeEnabled(env)) return {};
  pointPiCodeAtAgentDir(agentDir, env);
  approveWorkspace(sdk, agentDir, workspace);
  return { additionalExtensionPaths: [piCodeDir()] };
}

module.exports = { piCodeEnabled, piCodeDir, pointPiCodeAtAgentDir, approveWorkspace, loaderOptions };
