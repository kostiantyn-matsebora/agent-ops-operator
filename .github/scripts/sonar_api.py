"""Reach SonarCloud with `curl`, and validate every value that gets there.

`sonar-issues.py` (one pull request's issues, for the fixing loop) and
`sonar-findings-baseline.py` (a component's branch-wide backlog) each read
the same API the same way -- paginated `issues/search`, a `components.sh`
listing, a CLI-supplied output path -- and only one of them had been hardened
against a hostile `--api` or `--out`. This is that hardening, once, so a
future third reader inherits it rather than needing its own pass.
"""
from __future__ import annotations

import json
import os
import pathlib
import re
import subprocess
import urllib.parse

DEFAULT_API = "https://sonarcloud.io"
PAGE = 500
TIMEOUT = 60  # seconds per subprocess call, so a stalled service cannot hang the workflow

SAFE_URL = re.compile(r"^https?://[\w.\-~:/]+\?[\w.\-~%=&+]*$")


def validated_api(raw: str) -> str:
    """Refuses anything but an http(s) URL -- the base every request is built
    from, and the one CLI-supplied string that must not reach `curl`
    unexamined (pythonsecurity:S8701/S8705)."""
    parsed = urllib.parse.urlparse(raw)
    if parsed.scheme not in ("http", "https") or not parsed.netloc:
        raise SystemExit(f"--api must be an http(s) URL, not {raw!r}")
    return raw


def validated_path(raw: pathlib.Path, *, must_exist: bool) -> pathlib.Path:
    """Canonicalises the path and refuses one that resolves outside the
    current working directory -- the pythonsecurity:S2083/S8707 remediation
    (their own compliant example: `os.path.realpath` against `os.getcwd()`,
    checked with the trailing separator the rule's own "partial path
    traversal" pitfall warns is required), applied to every CLI-supplied
    path: `--out`, `--components`, `--components-script`."""
    base_dir = os.path.realpath(os.getcwd())
    resolved = os.path.realpath(str(raw))
    if resolved != base_dir and not resolved.startswith(base_dir + os.sep):
        raise SystemExit(f"path resolves outside the working directory: {raw}")
    result = pathlib.Path(resolved)
    if must_exist and not result.is_file():
        raise SystemExit(f"not a file: {result}")
    return result


def fetch(api: str, path: str, token: str, **params) -> dict:
    """One `GET` against the service, as JSON. Raises RuntimeError on a
    failed call.

    THE URL IS VALIDATED IMMEDIATELY BEFORE THE SUBPROCESS CALL IT GUARDS --
    pythonsecurity:S8705's own compliant example is a check adjacent to the
    sink; a validation several statements away was not credited, the same
    lesson `validated_path` states for its own callers. `--` marks the end of
    options too: verified against curl itself that without it a URL
    beginning with `-` is read as an unrecognised flag rather than the
    target -- the concrete case this regex also rules out, since only an
    http(s) scheme passes it."""
    url = f"{api}/api/{path}?{urllib.parse.urlencode(params)}"
    if not SAFE_URL.match(url):
        raise SystemExit(f"refusing a malformed request URL: {url!r}")
    # The credential rides a curl config on stdin, never argv, where `ps` shows it.
    # Backslash, quote and newline are escaped so a token cannot break out of the quoted value.
    escaped = token.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n").replace("\r", "\\r")
    try:
        out = subprocess.run(["curl", "-sf", "-K", "-", "--", url], input=f'user = "{escaped}:"\n',
                             capture_output=True, text=True, timeout=TIMEOUT)
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"{path}: curl timed out after {TIMEOUT}s") from exc
    if out.returncode != 0:
        raise RuntimeError(f"{path}: curl exit {out.returncode} {out.stderr.strip()}")
    return json.loads(out.stdout or "{}")


def components(path: pathlib.Path | None, script: pathlib.Path) -> list[dict]:
    """`[{component, context}]` from components.sh (or a captured copy of its
    output, for the suite)."""
    if path:
        return json.loads(validated_path(path, must_exist=True).read_text())
    resolved = validated_path(script, must_exist=True)
    if not os.access(resolved, os.X_OK):
        raise SystemExit(f"not executable: {resolved}")
    try:
        out = subprocess.run([str(resolved), "images"], capture_output=True, text=True, check=True,
                             timeout=TIMEOUT).stdout
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"{resolved}: timed out after {TIMEOUT}s") from exc
    return json.loads(out)


def issues_for(api: str, token: str, key: str, **filters) -> list[dict]:
    """Every open (`resolved=false`) issue on `key` matching `filters`,
    paged. No caller passes `pullRequest` and `componentKeys` alone here --
    that distinction (one pull request's issues vs. a branch-wide backlog)
    is the caller's, not this function's."""
    items: list[dict] = []
    page = 1
    while True:
        payload = fetch(api, "issues/search", token, componentKeys=key, resolved="false",
                        ps=PAGE, p=page, **filters)
        found = payload.get("issues", [])
        items.extend(found)
        total = int(payload.get("total") or 0)
        if page * PAGE >= total or not found:
            return items
        page += 1
