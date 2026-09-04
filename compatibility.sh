#!/bin/bash
set -euo pipefail

# mise
eval "$(mise activate bash)"
mise install

# Exercise the go install path: build into an isolated GOBIN, then run the
# resulting binary's --version and one real replacement to validate
# end-to-end install.
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

GOBIN="$TMP" go install ./...
"$TMP/sub1" --version
printf 'alpha\nbeta\n' >"$TMP/sample.txt"
"$TMP/sub1" "$TMP/sample.txt" <<'BLOCK'
beta
====
gamma
BLOCK
grep -qx gamma "$TMP/sample.txt"
