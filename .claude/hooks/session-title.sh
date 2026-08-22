#!/usr/bin/env bash
# Keep a terminal window titled with what the session is working on.
#
#   session-title.sh set "<title>"   write this session's title and paint it
#   session-title.sh paint           repaint from the file (hook: stdin JSON)
#   session-title.sh clean           forget this session's title (hook)
#
# Claude Code repaints the terminal title itself, so a one-off escape does not
# survive. The paint mode is wired to turn boundaries to re-assert it.
#
# Every failure is a silent no-op: a session with no title file, no terminal,
# or no jq must not turn a hook into an error.
set -u

dir="${XDG_RUNTIME_DIR:-/tmp}/claude-titles"

paint() {
  local t=$1
  [ -n "$t" ] || return 0
  # /dev/tty when the caller has a controlling terminal; otherwise the terminal
  # the Claude process itself is attached to (tool subprocesses have no tty).
  # 2>/dev/null FIRST: bash reports a failed redirection on the shell's own
  # stderr, before a later 2>/dev/null on the same command could suppress it.
  if ! { printf '\033]0;%s\007\033]2;%s\007' "$t" "$t" 2>/dev/null >/dev/tty; } 2>/dev/null; then
    local p
    p=$(ps -o tty= -p "${CLAUDE_PID:-0}" 2>/dev/null | tr -d ' ')
    [ -n "$p" ] && [ "$p" != "?" ] || return 0
    printf '\033]0;%s\007\033]2;%s\007' "$t" "$t" > "/dev/$p" 2>/dev/null || true
  fi
}

# session id: the argument form takes it from the environment, the hook forms
# from the payload on stdin.
case "${1:-paint}" in
  set)
    sid=${CLAUDE_CODE_SESSION_ID:-}
    ;;
  *)
    command -v jq >/dev/null 2>&1 || exit 0
    sid=$(jq -r '.session_id // empty' 2>/dev/null) || exit 0
    ;;
esac
[ -n "${sid:-}" ] || exit 0
file="$dir/$sid"

case "${1:-paint}" in
  set)
    title=${2:-}
    [ -n "$title" ] || exit 0
    mkdir -p "$dir" 2>/dev/null || exit 0
    printf '%s' "$title" > "$file" 2>/dev/null || exit 0
    paint "$title"
    ;;
  clean)
    rm -f "$file" 2>/dev/null || true
    ;;
  *)
    [ -r "$file" ] || exit 0
    paint "$(cat "$file" 2>/dev/null)"
    ;;
esac
exit 0
