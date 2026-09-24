#!/usr/bin/env bash
#
# formula-readiness-check.sh
#
# Runs the checks the leo Homebrew formula should pass before it is published to
# the tap, in order:
#
#   1. Go project is sound      -> make check (gofmt, vet, tests)
#   2. Formula style            -> brew style
#   3. Pinned release is real   -> tarball reachable and sha256 matches
#   4. New-formula audit        -> brew audit --new --strict --online
#   5. Builds from source       -> HOMEBREW_NO_INSTALL_FROM_SOURCE=1
#                                  brew install --build-from-source
#   6. Formula test passes      -> brew test
#
# Step 3 gates the rest: Homebrew builds from the tagged release tarball, so the
# version in Formula/leo.rb must be tagged, pushed, and its sha256 filled in
# before the audit/install/test steps can pass. If it isn't, this script stops
# with the exact command to fix it.
#
# The formula file (the tap's Formula/leo.rb) is staged into a throwaway tap
# only for the duration of this run and untapped on exit, so no tap is left
# behind.
set -euo pipefail

LEO_REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FORMULA="${FORMULA:-$LEO_REPO/../homebrew-leo/Formula/leo.rb}"

# Ephemeral local tap holding a copy of the working-tree formula, so the checks
# validate the file exactly as it is on disk (not a git-committed version of it).
TAP="leo-readiness/check"
FQ="$TAP/leo"

step() { printf '\n==> %s\n' "$1"; }
die()  { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

command -v go   >/dev/null || die "go is not on PATH"
command -v brew >/dev/null || die "brew is not on PATH"
[ -f "$FORMULA" ] || die "formula not found at $FORMULA (override with FORMULA=<path>)"

step "Go checks (make check)"
make -C "$LEO_REPO" check

step "brew style"
brew style "$FORMULA"

step "release tarball is real (url reachable + sha256 matches)"
url=$(awk -F'"' '/^[[:space:]]*url /{print $2; exit}' "$FORMULA")
sha=$(awk -F'"' '/^[[:space:]]*sha256 /{print $2; exit}' "$FORMULA")
[ -n "$url" ] || die "no url found in formula"
if ! curl -fsIL "$url" >/dev/null 2>&1; then
  die "release tarball not reachable: $url
     Tag and push that version first, e.g.:
       git tag -a v0.2.0 -m 'leo v0.2.0' && git push origin v0.2.0"
fi
actual=$(curl -fsSL "$url" | shasum -a 256 | awk '{print $1}')
if [ "$actual" != "$sha" ]; then
  die "sha256 mismatch for $url
     formula has: $sha
     tarball is:  $actual
     Update Formula/leo.rb sha256 to the tarball value above
     (make formula-sha TAG=<tag> prints it)."
fi
echo "ok: tarball reachable and sha256 matches"

# From here on we touch the local Homebrew state; always clean it up.
cleanup() {
  brew uninstall --force "$FQ"  >/dev/null 2>&1 || true
  brew untap "$TAP"             >/dev/null 2>&1 || true
}
trap cleanup EXIT

step "stage the working-tree formula in an ephemeral tap"
brew untap "$TAP" >/dev/null 2>&1 || true
brew tap-new "$TAP" >/dev/null
tapdir="$(brew --repository "$TAP")"
mkdir -p "$tapdir/Formula"
cp "$FORMULA" "$tapdir/Formula/leo.rb"

step "brew audit --new --strict --online"
brew audit --new --strict --online "$FQ"

step "brew install --build-from-source (HOMEBREW_NO_INSTALL_FROM_SOURCE=1)"
HOMEBREW_NO_INSTALL_FROM_SOURCE=1 brew install --build-from-source "$FQ"

step "brew test"
brew test "$FQ"

printf '\nPASS: leo formula is ready to publish to the tap.\n'
