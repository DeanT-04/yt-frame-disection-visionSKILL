#!/usr/bin/env bash
# One-command dev loop for this project: auto-format + auto-lint-fix + verify.
# Run from anywhere; safe to call repeatedly (idempotent).
#
#   ./dev.sh          -> fix+check everything (the default)
#   ./dev.sh fmt      -> gofmt + goimports -w
#   ./dev.sh lint     -> golangci-lint run --fix ./...
#   ./dev.sh check    -> fmt + lint + vet + test + build
#   ./dev.sh vuln     -> govulncheck ./... (dependency vulnerability scan)
#
# Exit code is non-zero if anything fails AFTER fixing, so CI/agents get a
# hard signal. Everything auto-fixes first, so the working tree stays clean.
set -euo pipefail
cd "$(dirname "$0")"

GO_FILES() {
  if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git ls-files '*.go'
  else
    find . -name '*.go' -not -path './vendor/*'
  fi
}

do_fmt() {
  echo "[dev] gofmt -w"
  gofmt -w . 
  echo "[dev] goimports -w"
  mapfile -t files < <(GO_FILES)
  if ((${#files[@]} > 0)); then
    goimports -w "${files[@]}"
  fi
}

do_lint() {
  echo "[dev] golangci-lint run --fix ./..."
  golangci-lint run --fix ./...
}

do_vuln() {
  if ! command -v govulncheck >/dev/null 2>&1; then
    echo "[dev] SKIP govulncheck: not installed — run: go install golang.org/x/vuln/cmd/govulncheck@latest" >&2
    return 0
  fi
  echo "[dev] govulncheck ./..."
  govulncheck ./...
}

do_check() {
  echo "[dev] go vet ./..."
  go vet ./...
  echo "[dev] go test ./..."
  go test ./...
  do_vuln
  echo "[dev] go build ./... (-> bin/)"
  mkdir -p bin
  go build -o bin/app.exe .
  # After --fix, formatting must already be clean; enforce it:
  if ! gofmt -l . >/dev/null; then :; fi
  unformatted=$(gofmt -l .)
  if [[ -n "$unformatted" ]]; then
    echo "[dev] FAIL: files need gofmt:" >&2
    echo "$unformatted" >&2
    exit 1
  fi
  echo "[dev] all clean ✔"
}

case "${1:-all}" in
  fmt)   do_fmt ;;
  lint)  do_fmt; do_lint ;;
  vuln)  do_vuln ;;
  check) do_fmt; do_lint; do_check ;;
  all)   do_fmt; do_lint; do_check ;;
  *) echo "usage: $0 [fmt|lint|vuln|check|all]" >&2; exit 2 ;;
esac
