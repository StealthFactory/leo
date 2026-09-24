# Submitting `leo` to homebrew-core

`brew install leo` with no tap requires the formula to live in
[homebrew-core](https://github.com/Homebrew/homebrew-core). This is the
checklist for getting it there, based on the homebrew-core
[CONTRIBUTING guide](https://github.com/Homebrew/homebrew-core/blob/master/CONTRIBUTING.md)
and [Acceptable Formulae](https://docs.brew.sh/Acceptable-Formulae).

The formula lives in this repo at `packaging/Formula/leo.rb`. There is no
personal tap; the core route needs none.

## Will homebrew-core accept it?

What the docs require, and where leo stands:

- Stable, versioned release with an immutable tag (no HEAD-only, no moving
  branch, no unchecksummed archive). Met: built from a release tarball with a
  pinned `sha256`.
- Open-source license compatible with the Debian Free Software Guidelines, with
  a LICENSE file in the repo. Met: MIT.
- Builds from source and passes `brew test` on the current macOS and Linux CI
  matrix. Met locally (see the readiness check below).
- Not a native `.app`, not a cask/binary-only, not self-updating. Met.

The docs no longer publish hard notability numbers, but acceptance is still a
maintainer judgment and a brand-new tool can be declined for not being notable
enough. Don't open the PR until the project is plausibly notable; a rejected PR
wastes maintainer time.

## AI/LLM rules (read carefully)

The CONTRIBUTING guide has explicit rules when AI/LLM tools are involved. This
formula was drafted with AI help, so these apply:

- Disclose in the PR that you used an AI/LLM, and which tool/model.
- Review all AI-generated content yourself before asking anyone to review it.
- Do NOT attribute commits to AI as author or co-author. The formula commit must
  have no `Co-Authored-By` AI line.
- Answer all maintainer questions yourself, without AI assistance.
- Keep only one open AI-assisted PR at a time.

## Steps when ready

1. Make sure the release tag is pushed and the formula points at it:
   - `url` -> `https://github.com/StealthFactory/leo/archive/refs/tags/vX.Y.Z.tar.gz`
   - `sha256` -> `make formula-sha TAG=vX.Y.Z` (run from the leo repo)

2. Run the local gate from the leo repo:
   ```sh
   make homebrew-core-readiness-check
   ```
   This runs the Go checks, `brew style`, verifies the release tarball and its
   sha256, then `brew audit --new --strict --online`, a source build with
   `HOMEBREW_NO_INSTALL_FROM_SOURCE=1 brew install --build-from-source`, and
   `brew test`. It must end with `PASS`.

3. Fork homebrew-core and drop the formula in place:
   ```sh
   gh repo fork Homebrew/homebrew-core --clone
   cp packaging/Formula/leo.rb homebrew-core/Formula/l/leo.rb
   ```

4. Commit as a single commit, no AI co-author:
   ```sh
   cd homebrew-core
   git checkout -b leo
   git add Formula/l/leo.rb
   git commit -m "leo X.Y.Z (new formula)"
   ```
   Squash to one commit if you push more changes later.

5. Open the PR and disclose the AI usage in its description:
   ```sh
   gh pr create --repo Homebrew/homebrew-core --title "leo X.Y.Z (new formula)"
   ```

## Notes

- The formula builds with the system `go` (`depends_on "go" => :build`) and
  injects the version via `-ldflags "-X main.version=#{version}"`.
- `leo clip` is macOS-only; the `brew test` block exercises `version` and the
  object store, which are cross-platform, so it passes on Linux CI too.
