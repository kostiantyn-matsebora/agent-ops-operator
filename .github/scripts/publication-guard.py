#!/usr/bin/env python3
"""Publication hygiene guard.

This repository is published. Every file AND every commit message is readable
by strangers, so none of them may say who runs the author's cluster, what it is
called, or how to reach it.

WHAT IT IS, and the two properties that decide the design:

1. IT IS AN ALLOWLIST. Never a list of forbidden strings. A denylist has to
   spell out the thing it protects, which publishes it in the guard itself, and
   it catches only what somebody already thought of. What is permitted is a set
   of SHAPES — reserved example domains, cluster-internal names, loopback, this
   repository's own clone url, the documented placeholder identifiers — and
   everything else is reported.

2. IT DOES NOT REPUBLISH WHAT IT CAUGHT. A public repository has public build
   logs, so a report quoting its findings leaks them to exactly the audience it
   exists to protect the tree from. The report is FILE, LINE and RULE. Pass
   --show locally, where the fixing happens, to see the matched text.

Usage:
    publication-guard.py                       scan the tracked tree
    publication-guard.py --show                ... and print what matched (LOCAL ONLY)
    publication-guard.py --counts              per-rule counts, nothing else
    publication-guard.py --messages <range>    also scan commit messages in a range
    publication-guard.py --path <p> [...]      scan named paths instead of the tree

Exit status is 1 when anything is reported.
"""

import argparse
import fnmatch
import ipaddress
import json
import os
import re
import subprocess
import sys
from collections import Counter

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
ALLOWLIST = os.path.join(ROOT, ".github", "publication-allowlist.json")


def load_allowlist(path):
    with open(path, encoding="utf-8") as fh:
        raw = json.load(fh)
    # Comment entries live inline so the reasoning sits beside what it explains.
    # They are stripped here rather than at every use.
    def clean(values):
        return [v for v in values if not v.startswith("_comment")]
    return raw, clean


RAW, _clean = load_allowlist(ALLOWLIST)

SKIP_PATHS = _clean(RAW["skipPaths"])
HOST = RAW["hostname"]
HOST_TLDS = set(HOST["tlds"])
HOST_CLUSTER = HOST["clusterSuffixes"]
HOST_IGNORED = set(_clean(HOST["ignoredLastLabels"]))
HOST_ALLOWED = _clean(HOST["allowed"])
NET_ALLOWED = [ipaddress.ip_network(n) for n in RAW["addressLiteral"]["allowed"]]
REPO_ALLOWED = {r.lower() for r in _clean(RAW["repositoryUrl"]["allowed"])}
REPO_FORGES = {h.lower() for h in RAW["repositoryUrl"]["forgeHosts"]}
CHAT = RAW["chatIdentifier"]
CHAT_ALLOWED = set(_clean(CHAT["allowed"]))
CHAT_KEYS = CHAT["contextKeys"]
CHAT_PATHS = CHAT["contextPaths"]
CHAT_SIGNED_DIGITS = CHAT["signedIdentifierDigits"]
MAIL_ALLOWED = {d.lower() for d in _clean(RAW["email"]["allowedDomains"])}

LABEL = r"[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?"

RE_DOTTED = re.compile(r"\b(?:" + LABEL + r"\.)+" + LABEL + r"\b")
RE_SCHEMED = re.compile(r"[A-Za-z][A-Za-z0-9+.\-]*://([^/\s\"'`,)\]}>]+)")
RE_IPV4 = re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}\b")
# A CLONE or REMOTE url: scheme or scp form, a forge host, owner and repo.
RE_REPO = re.compile(
    r"(?:[A-Za-z][A-Za-z0-9+.\-]*://(?:[^/@\s]+@)?|\b[A-Za-z0-9_.\-]+@)"
    r"([A-Za-z0-9.\-]+)[:/]([A-Za-z0-9_.\-]+)/([A-Za-z0-9_.\-]+?)(?:\.git)?"
    r"(?=[\s\"'`,)\]}>]|$)"
)
# A chat, group or thread identifier, found where one LIVES rather than by
# counting digits: a field whose NAME says so, a transport path that carries
# one, or the signed form a supergroup id takes.
# PROSE NAMES THE FIELD IN TWO WORDS, and both halves of that cost a finding.
#
# A requirement writes "thread id `\"<value>\"`" where a struct writes
# `ThreadID: "<value>"` — so the key and `id` are separated by a SPACE, and the
# value sits inside a code span behind a BACKTICK. The rule matched neither, and
# read straight past an identifier in the one place a spec states it. Found by a
# history rewrite, not by the guard.
RE_CHAT_KEYED = re.compile(
    r"(?i)\b[a-z_.\-]*(?:" + "|".join(CHAT_KEYS) + r")[a-z_.\-]* ?id\b"
    r"[\"'`*_\s:=>,\]}]*(-?\d{6,20})"
)
RE_CHAT_PATH = re.compile(
    r"(?:" + "|".join(re.escape(p) for p in CHAT_PATHS) + r")(-?\d{6,20})"
)
RE_CHAT_SIGNED = re.compile(r"(?<![\d.])(-\d{" + str(CHAT_SIGNED_DIGITS) + r",20})(?![\d.])")
RE_MAIL = re.compile(r"\b[A-Za-z0-9._%+\-]+@([A-Za-z0-9.\-]+\.[A-Za-z]{2,})\b")


def skipped(relpath):
    for pattern in SKIP_PATHS:
        if fnmatch.fnmatch(relpath, pattern) or fnmatch.fnmatch("/" + relpath, "/" + pattern):
            return True
        if pattern.endswith("/*") and relpath.startswith(pattern[:-2] + "/"):
            return True
    return False


def matches_host_pattern(host, pattern):
    if pattern.startswith("*."):
        suffix = pattern[1:]          # ".example.com"
        return host == pattern[2:] or host.endswith(suffix)
    return host == pattern


def host_allowed(host):
    host = host.rstrip(".").lower()
    return any(matches_host_pattern(host, p) for p in HOST_ALLOWED)


def is_host_candidate(name):
    """Is this dotted string plausibly a HOST, rather than a filename or a field access?"""
    name = name.rstrip(".").lower()
    if "." not in name:
        return False
    last = name.rsplit(".", 1)[1]
    if last in HOST_IGNORED:
        return False
    for suffix in HOST_CLUSTER:
        if name.endswith("." + suffix):
            return True
    return last in HOST_TLDS


def scan_text(text, origin, findings):
    """Append (origin, line, rule, matched) for everything not allowlisted."""
    for lineno, line in enumerate(text.split("\n"), start=1):
        def report(rule, matched):
            findings.append((origin, lineno, rule, matched))

        # hostname — bare dotted names and anything reached through a scheme
        seen = set()
        for name in RE_DOTTED.findall(line):
            if is_host_candidate(name):
                seen.add(name.rstrip(".").lower())
        for authority in RE_SCHEMED.findall(line):
            host = authority.split("@")[-1].split(":")[0].rstrip(".").lower()
            host = re.split(r"[<{}]", host)[0].rstrip(".")
            # A name with no public TLD is cluster-internal or a template
            # placeholder — it resolves nowhere and identifies nobody.
            if is_host_candidate(host) or RE_IPV4.fullmatch(host):
                seen.add(host)
        for host in sorted(seen):
            if RE_IPV4.fullmatch(host):
                continue                      # the address rule owns these
            if not host_allowed(host):
                report("hostname", host)

        # address literal
        for literal in RE_IPV4.findall(line):
            try:
                addr = ipaddress.ip_address(literal)
            except ValueError:
                continue                      # a version string, not an address
            if not any(addr in net for net in NET_ALLOWED):
                report("address-literal", literal)

        # repository url — clone and remote forms only, never a bare module path
        # and never a registry path
        for host, owner, repo in RE_REPO.findall(line):
            if host.lower() not in REPO_FORGES:
                continue
            slug = f"{owner}/{repo}".rstrip(".").lower()
            if slug not in REPO_ALLOWED:
                report("repository-url", slug)

        # chat, group and thread identifiers
        idents = set()
        for rx in (RE_CHAT_KEYED, RE_CHAT_PATH, RE_CHAT_SIGNED):
            idents.update(rx.findall(line))
        for ident in sorted(idents):
            if ident not in CHAT_ALLOWED:
                report("chat-identifier", ident)

        # email — the domain has to be a host before it can be an address
        for domain in RE_MAIL.findall(line):
            if not is_host_candidate(domain):
                continue
            if domain.lower() not in MAIL_ALLOWED:
                report("email", domain)


def tracked_files():
    out = subprocess.run(
        ["git", "-C", ROOT, "ls-files"], capture_output=True, text=True, check=True
    ).stdout
    return [f for f in out.split("\n") if f]


def scan_paths(paths, findings):
    for rel in paths:
        if skipped(rel):
            continue
        full = os.path.join(ROOT, rel)
        if not os.path.isfile(full):
            continue
        try:
            with open(full, encoding="utf-8") as fh:
                text = fh.read()
        except (UnicodeDecodeError, OSError):
            continue                          # not text, so not publishable prose
        scan_text(text, rel, findings)


def scan_messages(rev_range, findings):
    """Commit messages live OUTSIDE the tree, so a tree-only guard leaves the one
    hole that cannot be fixed by editing a file later."""
    out = subprocess.run(
        ["git", "-C", ROOT, "log", "--format=%H%x00%B%x00", rev_range],
        capture_output=True, text=True,
    )
    if out.returncode != 0:
        print(f"publication-guard: cannot read commit messages for {rev_range}", file=sys.stderr)
        return
    parts = out.stdout.split("\0")
    for i in range(0, len(parts) - 1, 2):
        sha, body = parts[i].strip(), parts[i + 1]
        if not sha:
            continue
        scan_text(body, f"commit {sha[:12]}", findings)


def main():
    ap = argparse.ArgumentParser(add_help=True, description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--show", action="store_true",
                    help="print the matched text. LOCAL USE ONLY — build logs are public")
    ap.add_argument("--counts", action="store_true",
                    help="print per-rule counts and nothing else")
    ap.add_argument("--messages", metavar="RANGE",
                    help="also scan the commit messages in a git revision range")
    ap.add_argument("--path", action="append", default=[], metavar="PATH",
                    help="scan these paths instead of the tracked tree")
    args = ap.parse_args()

    findings = []
    scan_paths(args.path or tracked_files(), findings)
    if args.messages:
        scan_messages(args.messages, findings)

    counts = Counter(rule for _, _, rule, _ in findings)

    if args.counts:
        for rule in sorted(counts):
            print(f"{rule}: {counts[rule]}")
        print(f"total: {len(findings)}")
        return 1 if findings else 0

    for origin, lineno, rule, matched in findings:
        suffix = f"  [{matched}]" if args.show else ""
        print(f"{origin}:{lineno}: {rule}{suffix}")

    if findings:
        print("", file=sys.stderr)
        for rule in sorted(counts):
            print(f"publication-guard: {rule}: {counts[rule]}", file=sys.stderr)
        print(
            "publication-guard: the matched text is deliberately NOT printed — "
            "run locally with --show to see it.",
            file=sys.stderr,
        )
        return 1

    print("publication-guard: clean")
    return 0


if __name__ == "__main__":
    sys.exit(main())
