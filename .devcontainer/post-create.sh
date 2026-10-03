#!/bin/sh
# Provisions the tools AGENTS.md lists beyond Go itself, then readies the
# repository: hooks installed and the module cache warm.
set -eu

# The devcontainers/go image ships golangci-lint; only fall back to the
# official binary installer if a slimmer image is ever substituted
# (building it with `go install` is unsupported upstream).
GOLANGCI_LINT_VERSION=v2.13.1
LEFTHOOK_VERSION=1.13.6

if ! command -v golangci-lint >/dev/null 2>&1; then
  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
    | sh -s -- -b "$(go env GOPATH)/bin" "${GOLANGCI_LINT_VERSION}"
fi

# Lefthook is fetched as a release binary: `go install` compiles its own
# module graph, which does not build on every Go release.
if ! command -v lefthook >/dev/null 2>&1; then
  arch="$(uname -m)"
  case "$arch" in
    aarch64) arch=arm64 ;;
  esac
  curl -sSfL -o "$(go env GOPATH)/bin/lefthook" \
    "https://github.com/evilmartians/lefthook/releases/download/v${LEFTHOOK_VERSION}/lefthook_${LEFTHOOK_VERSION}_Linux_${arch}"
  chmod +x "$(go env GOPATH)/bin/lefthook"
fi

# A linked worktree's .git file points outside the container mount; hooks
# then cannot be installed, which must not fail container creation.
if git rev-parse --git-dir >/dev/null 2>&1; then
  make hooks
else
  echo "skipping hook install: not inside a usable git repository"
fi
go mod download

echo "boreas devcontainer ready: run 'make db' then 'BOREAS_ADMIN_PASSWORD=change-me make dev'"
