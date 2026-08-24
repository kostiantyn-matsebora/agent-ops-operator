#!/usr/bin/env python3
"""EVERY ServiceAccount A RENDER PRODUCES IS EXPLAINED, OR THE BUILD FAILS.

An account bound to nothing that no pod runs as is indistinguishable from the
floor every unnamed route already inherits. It grants no boundary, and it adds a
name to every audit of who holds what in the cluster.

Four of them shipped that way — two bundle route identities and two MCP server
accounts — because the rule read "an account for EVERY route a bundle ships".
Nothing failed, nothing warned, and the only way to notice was to render the
chart and read the list.

An account is explained when it is one of:

  bound        a ClusterRoleBinding or RoleBinding names it as a subject
  authenticates  a pod runs as it AND mounts its token, so the identity actually
                 reaches the API server
  the floor    bound to nothing BY DESIGN, and the chart refuses to bind it

Anything else is a name.

THE SECOND TEST USED TO BE "a pod runs as it", AND THAT WAS TOO GENEROUS — it
was written to justify two accounts that turned out not to deserve it. A pod
with `automountServiceAccountToken: false` never presents its identity to the API
server, so which account it names changes nothing it can reach. Running as the
namespace default with no token is the same reach: none.

Usage: serviceaccount-guard.py <rendered.yaml> [name]
"""
import sys
import yaml

FLOOR = {"agentops-runtime"}


def main() -> int:
    path = sys.argv[1]
    label = sys.argv[2] if len(sys.argv) > 2 else path
    docs = [d for d in yaml.safe_load_all(open(path)) if d]

    accounts = {}
    for d in docs:
        if d.get("kind") == "ServiceAccount":
            accounts[d["metadata"]["name"]] = d

    bound = set()
    for d in docs:
        if d.get("kind") in ("ClusterRoleBinding", "RoleBinding"):
            for s in d.get("subjects", []):
                if s.get("kind") == "ServiceAccount":
                    bound.add(s["name"])

    # A pod EXPLAINS an account only when it also mounts the token: without one
    # the pod never authenticates, so the name is decorative.
    authenticates = {}
    named_only = {}
    for d in docs:
        if d.get("kind") not in ("Deployment", "StatefulSet", "DaemonSet", "CronJob", "Job"):
            continue
        spec = d["spec"]
        pod = (spec.get("jobTemplate", {}).get("spec", {}).get("template", {}).get("spec")
               or spec.get("template", {}).get("spec", {}))
        sa = pod.get("serviceAccountName")
        if not sa:
            continue
        mounts = pod.get("automountServiceAccountToken")
        if mounts is False or accounts.get(sa, {}).get("automountServiceAccountToken") is False:
            named_only.setdefault(sa, []).append(d["metadata"]["name"])
        else:
            authenticates.setdefault(sa, []).append(d["metadata"]["name"])

    unexplained = sorted(set(accounts) - bound - set(authenticates) - FLOOR)
    # An account a pod names but never presents. Report which pod, because the
    # fix is usually to stop rendering the account rather than to mount a token.
    loose = sorted(set(named_only) - bound - set(authenticates) - FLOOR)

    print(f"{label}: {len(accounts)} ServiceAccount(s), "
          f"{len(bound & set(accounts))} bound, {len(authenticates)} authenticating")
    ok = True
    if unexplained:
        ok = False
        print("  UNEXPLAINED — nothing is bound to them and nothing authenticates as them:")
        for n in unexplained:
            print(f"    {n}")
    if loose:
        ok = False
        print("  NAMED BUT NEVER PRESENTED — the pod mounts no token, so this "
              "account reaches nothing. Stop rendering it, or bind it and mount it:")
        for n in loose:
            print(f"    {n} (named by {', '.join(named_only[n])})")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
