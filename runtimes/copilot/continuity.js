// Gone, or merely not answering? The ladder that decides, kept beside the
// runtime rather than inside it so it can be tested without the SDK.
//
// A shared filesystem can fail to answer for seconds — a restarting share
// manager, a stale handle after the pod moved, a listing that has not yet seen
// a directory another node wrote — and ending a conversation over a lag of that
// kind would turn a storage nicety into a correctness bug. Only an answer of
// "not there" is absence. Anything else is the store not answering, and is
// treated as PRESENT so the run is retried rather than failed.

'use strict';

const fs = require('fs');
const path = require('path');

const DEFAULT_DELAYS_MS = [500, 1500, 3000];

// stateDirPresent answers "is the vendor's state for this id on disk", and
// THROWS on anything other than a clean ENOENT so the caller can tell a store
// that did not answer from one that said no.
async function stateDirPresent(sessionsDir, contextId) {
  const dir = path.join(sessionsDir, contextId);
  try {
    const st = await fs.promises.stat(dir);
    return st.isDirectory();
  } catch (e) {
    if (e && e.code === 'ENOENT') {
      // The ROOT must answer for ENOENT to mean absence: a session-state root
      // that is itself unreadable is a volume that is not there.
      await fs.promises.readdir(sessionsDir).catch((pe) => { if (pe.code !== 'ENOENT') throw pe; });
      return false;
    }
    throw e;
  }
}

// confirmContextMissing re-checks after short delays. Returns true only when
// every check answered "not there"; a reappearance or an unreadable store
// returns false. Bounded and short on purpose: a person is waiting on a reply.
async function confirmContextMissing(sessionsDir, contextId, opts = {}) {
  if (!contextId) return true;
  const delays = opts.delays || DEFAULT_DELAYS_MS;
  const wait = opts.sleep || ((ms) => new Promise((r) => setTimeout(r, ms)));
  const probe = opts.probe || stateDirPresent;
  for (const ms of delays) {
    await wait(ms);
    let found;
    try {
      found = await probe(sessionsDir, contextId);
    } catch {
      return false; // unreadable is NOT absent
    }
    if (found) return false;
  }
  return true;
}

module.exports = { DEFAULT_DELAYS_MS, stateDirPresent, confirmContextMissing };
