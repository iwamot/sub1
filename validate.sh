#!/bin/bash
set -e

# mise
eval "$(mise activate bash)"
mise fmt
mise install

# Go
go mod tidy
go-licenses check ./...
govulncheck ./...
gofmt -w .
go vet ./...
if [[ -n "$CI" ]]; then
  go test ./... -coverprofile=coverage.out
else
  go test ./... -cover
fi
trap 'rm -rf dist' EXIT
goreleaser release --snapshot --clean

# Shared lint tasks
mise run gha-lint
mise run shell-lint

# Check for uncommitted changes
git diff --exit-code
