#!/usr/bin/env bash
#
# Runs all credential-free checks that can be completed before publishing a
# signed and notarized release. The actual release workflow additionally
# verifies its secrets, signs and notarizes both binaries, publishes the
# GitHub release and cask, and installs the published cask on macOS.
set -euo pipefail

LEO_REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

step() { printf '\n==> %s\n' "$1"; }
die()  { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

command -v go >/dev/null || die "go is not on PATH"
command -v goreleaser >/dev/null || die "goreleaser is not on PATH (install it with: brew install goreleaser)"

step "Go checks"
make -C "$LEO_REPO" check

step "GoReleaser configuration"
(
  cd "$LEO_REPO"
  goreleaser check
)

step "Unsigned local release snapshot"
(
  cd "$LEO_REPO"
  goreleaser release --snapshot --clean --skip=publish,notarize
)

shopt -s nullglob
arm64_archives=("$LEO_REPO"/dist/leo_*_darwin_arm64.tar.gz)
amd64_archives=("$LEO_REPO"/dist/leo_*_darwin_amd64.tar.gz)
[ "${#arm64_archives[@]}" -eq 1 ] || die "expected one Darwin arm64 archive"
[ "${#amd64_archives[@]}" -eq 1 ] || die "expected one Darwin amd64 archive"

for archive in "${arm64_archives[0]}" "${amd64_archives[0]}"; do
  tar -tzf "$archive" | grep -qx "leo" || die "$archive does not contain leo"
done

printf '\nPASS: Leo is ready for a tagged release.\n'
