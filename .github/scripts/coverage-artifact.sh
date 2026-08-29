#!/usr/bin/env sh
# THE ONE TRANSFORM from a component path to its coverage artifact name.
#
# The test jobs upload under this name and the sonar job downloads by it, so
# the rule is written here and called from both — spelled in two jobs, a
# mismatch fails GREEN: the analysis submits and coverage reads 0 %.
#
#   platform/manager      -> coverage-platform-manager
#   ./platform/console    -> coverage-platform-console
#   platform/console/ui   -> coverage-platform-console-ui
#
# `components.sh images` reports contexts as `./path`, the modules matrix as
# `path`; the prefix is dropped so both sides agree.
set -eu
printf 'coverage-%s\n' "$(printf '%s' "${1#./}" | tr '/' '-')"
