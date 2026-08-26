#!/usr/bin/env python3
"""Derive a dispatch's work list from the review threads, and from nothing else.

THE DISPATCH COMMENT IS A TRIGGER, NOT AN INSTRUCTION. This program never reads
it. It walks every review thread on the pull request and keeps the ones a
PERSON accepted in the stated vocabulary -- so the sentence that authorises
work (the dispatch, gated on who sent it) and the sentence that describes the
work (a finding, accepted in its own thread) can never be the same sentence.

THE VOCABULARY IS A FILE, NOT A JUDGEMENT. `.github/review-triage.json` holds
the phrases, and this program applies its matching rule mechanically: whole
reply, trimmed, trailing punctuation dropped, case-insensitive. A reply that
sounds agreeable and is not on the list is not an acceptance. The failure mode
is "nothing happened", which is visible in an open thread, rather than
"something happened I did not mean", which is not.

FOUR REFUSALS, AND THEY ARE THE PRODUCT:

  1. A thread whose FIRST comment is not the review's. Not a finding; a person's
     remark, which no dispatch may act on.
  2. A thread already resolved. Settled, by a person or by the review.
  3. An acceptance written by the review's own login. A bot that can accept its
     own findings is a bot that writes to the branch unattended.
  4. An acceptance from an account without write access -- read from the
     comment's own `authorAssociation`. A public pull request is a place
     strangers can type.

EVERY SKIP IS REPORTED, never silent. A dispatch that acted on three findings
when a person accepted four is a bug only if someone can see the fourth.
"""
from __future__ import annotations

import argparse
import json
import os
import pathlib
import subprocess
import sys

THREADS_QUERY = """
query($owner:String!, $repo:String!, $number:Int!, $cursor:String) {
  repository(owner:$owner, name:$repo) {
    pullRequest(number:$number) {
      reviewThreads(first:50, after:$cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          isResolved
          isOutdated
          path
          line
          comments(first:100) {
            nodes {
              databaseId
              body
              authorAssociation
              author { login __typename }
            }
          }
        }
      }
    }
  }
}
"""

# The associations GitHub reports for an account that can push here. `OWNER`
# is the repository's owner, `MEMBER` an organisation member with access,
# `COLLABORATOR` an invited one. Everything else -- CONTRIBUTOR (merged a PR
# once), FIRST_TIMER, NONE -- is a stranger for this purpose.
WRITE_ASSOCIATIONS = {"OWNER", "MEMBER", "COLLABORATOR"}

DEFAULT_VOCABULARY = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"


def gh_graphql(query: str, **variables) -> dict:
    cmd = ["gh", "api", "graphql", "-f", f"query={query}"]
    for key, value in variables.items():
        flag = "-F" if isinstance(value, int) else "-f"
        cmd += [flag, f"{key}={value}"]
    out = subprocess.run(cmd, capture_output=True, text=True, check=True).stdout
    payload = json.loads(out)
    if "errors" in payload:
        raise RuntimeError(payload["errors"])
    return payload["data"]


def normalise_login(login: str) -> str:
    """REST says `claude[bot]`, GraphQL says `claude`. Same rule as the resolver."""
    return (login or "").strip().lower().removesuffix("[bot]")


def load_vocabulary(path: pathlib.Path) -> dict:
    doc = json.loads(path.read_text())
    return {
        "accept": {p.strip().lower() for p in doc["accept"]},
        "dispatch": {p.strip().lower() for p in doc["dispatch"]},
        "punctuation": doc.get("trailing_punctuation", ".!,"),
    }


def normalise_reply(body: str, punctuation: str) -> str:
    """The matching rule, applied. Whole text, trimmed, trailing punctuation
    dropped, lower-cased. Nothing looks INSIDE a sentence: `please fix it and
    also X` is not `fix it`, and treating it as one would let the reply carry
    an instruction past the list."""
    return (body or "").strip().rstrip(punctuation).strip().lower()


def is_acceptance(body: str, vocabulary: dict) -> bool:
    return normalise_reply(body, vocabulary["punctuation"]) in vocabulary["accept"]


def is_dispatch(body: str, vocabulary: dict) -> bool:
    return normalise_reply(body, vocabulary["punctuation"]) in vocabulary["dispatch"]


def fetch_threads(owner: str, repo: str, number: int) -> list[dict]:
    threads: list[dict] = []
    cursor = None
    while True:
        kwargs = dict(owner=owner, repo=repo, number=number)
        if cursor:
            kwargs["cursor"] = cursor
        page = gh_graphql(THREADS_QUERY, **kwargs)["repository"]["pullRequest"]["reviewThreads"]
        threads.extend(page["nodes"])
        if not page["pageInfo"]["hasNextPage"]:
            return threads
        cursor = page["pageInfo"]["endCursor"]


def classify(thread: dict, allowed: set[str], vocabulary: dict) -> tuple[str, dict | None]:
    """One thread -> ('accepted', item) or ('<reason>', None). Pure, so the
    suite can exercise every refusal without a network."""
    comments = thread["comments"]["nodes"]
    if not comments:
        return "empty thread", None
    first = comments[0]
    first_author = first.get("author") or {}
    if normalise_login(first_author.get("login")) not in allowed or first_author.get("__typename") != "Bot":
        return f"first comment by {first_author.get('login')!r}, not the review", None
    if thread["isResolved"]:
        return "already resolved", None

    accepted_by = None
    for reply in comments[1:]:
        if not is_acceptance(reply.get("body", ""), vocabulary):
            continue
        author = reply.get("author") or {}
        # THE REVIEW MAY NOT ACCEPT ITS OWN FINDING. Matched on type as well as
        # login, exactly as the resolver does, so a person named `claude` is a
        # person here too.
        if author.get("__typename") == "Bot" or normalise_login(author.get("login")) in allowed:
            return f"acceptance written by the review itself ({author.get('login')!r})", None
        if reply.get("authorAssociation") not in WRITE_ASSOCIATIONS:
            return (f"acceptance by {author.get('login')!r} "
                    f"({reply.get('authorAssociation')}), who cannot push here"), None
        accepted_by = {"login": author.get("login"), "reply": reply.get("body", "")}
        # The FIRST acceptance wins; a later one adds nothing and a later
        # objection does not retract -- the thread is where that argument
        # happens, and the person can resolve it if they change their mind.
        break
    if accepted_by is None:
        return "not accepted", None

    return "accepted", {
        "threadId": thread["id"],
        "commentId": first["databaseId"],
        "path": thread["path"],
        "line": thread.get("line"),
        "outdated": bool(thread.get("isOutdated")),
        "finding": first.get("body", ""),
        "acceptedBy": accepted_by["login"],
        "reply": accepted_by["reply"],
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--pr", type=int, required=True)
    ap.add_argument("--out", type=pathlib.Path, required=True,
                    help="where to write the work list (JSON)")
    ap.add_argument("--vocabulary", type=pathlib.Path, default=DEFAULT_VOCABULARY)
    ap.add_argument("--authors", default=os.environ.get("REVIEW_AUTHORS", "claude[bot],github-actions[bot]"),
                    help="comma-separated logins the review posts as")
    args = ap.parse_args()

    owner, _, repo = args.repo.partition("/")
    allowed = {normalise_login(a) for a in args.authors.split(",") if a.strip()}
    vocabulary = load_vocabulary(args.vocabulary)

    threads = fetch_threads(owner, repo, args.pr)
    print(f"{len(threads)} review thread(s) on {args.repo}#{args.pr}")
    print(f"recognised as the review: {', '.join(sorted(allowed))}")
    print(f"accept vocabulary: {', '.join(sorted(vocabulary['accept']))}")

    accepted: list[dict] = []
    for thread in threads:
        verdict, item = classify(thread, allowed, vocabulary)
        where = f"{thread.get('path')}:{thread.get('line') or '?'}"
        if item is None:
            print(f"  skipped  {thread['id']} ({where}): {verdict}")
            continue
        print(f"  ACCEPTED {thread['id']} ({where}) by {item['acceptedBy']}")
        accepted.append(item)

    # WRITTEN EVEN WHEN EMPTY. An absent file and an empty list are different
    # facts downstream -- the same lesson `.resolve-threads` taught.
    args.out.write_text(json.dumps(accepted, indent=2) + "\n")
    print(f"\n{len(accepted)} accepted finding(s) written to {args.out}")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except subprocess.CalledProcessError as exc:
        print(f"gh failed: {exc.stderr or exc}", file=sys.stderr)
        sys.exit(1)
    except RuntimeError as exc:
        print(f"GraphQL error: {exc}", file=sys.stderr)
        sys.exit(1)
