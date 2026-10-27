#!/bin/sh
# Runs the whole suite on Linux in a container, as root, so the Linux-only
# tests (SO_PEERCRED, /proc, process groups) run too. Needs Docker.
set -eu
cd "$(dirname "$0")/.."
exec docker run --rm -v "$PWD":/src -w /src golang:1.27 \
  sh -c 'go version && go test -race -count=1 ./... && go test -count=1 -run TestFromConnReadsPeerCreds -v ./internal/peer/'
