# Leo 🦁

Leo is a tiny command-line sidekick you can teach new tricks in seconds.
The very first public _Stealth Factory_ make.

Out of the box Leo gives you a little JSON store to stash whatever you want and
a powerful "just copy this to my clipboard" command for the Mac, an extensible
ecosystem with the ability to separate the plugins by environment. Your existing
scripts are compatible with Leo as long as they're either in your `PATH` or
in Leo's command directory.

## Why

Over the years I've written a ton of helpful scripts for myself and I usually
throw them all into my home directory and invoke them through shell functions.
This can get out of hand real quick and maintaining them has included manual
efforts from my side. At first, I tried using various task runners and they
usually tend to come with their own baggage and none of them have any storage.
I see myself using this storage to store random configs or values. This gets out
of hand really quickly. I wrote myself a simple snippet storage CLI and after
using it for years on a daily basis, I figured it'd be a good addition to Leo.
So, Leo is extensible and ships with a data store. The cool part about the storage
is that it's a simple JSON, but if you give it a path to a file, it'll not just
copy the path, but it actually copies the file to your clipboard, ready to paste
into Slack, Telegram, Teams, or even Finder.

## Getting Leo

```sh
brew tap stealthfactory/leo
brew trust stealthfactory/leo
brew install --cask leo
```

The `brew trust` line is a one-time step. Since Homebrew 6+, a formula from a
third-party tap won't load until you trust the tap; the official taps are
trusted already.

To upgrade later:

```sh
brew update && brew upgrade --cask leo
```

## Sixty seconds with Leo

```sh
leo store set limits '{"cpu":2,"mem":"4Gi"}' # stash some JSON
leo store get limits --query .cpu            # -> 2
leo generate hello                           # scaffold your first subcommand
leo hello world                              # ...and run it
leo help                                     # see everything, yours included
```

That's the whole loop: keep some data around, teach Leo a new command, run it.

## The store

Leo keeps a JSON object store at `~/.config/leo/store.json` (by default and it's
configurable via `leo config setup`), locked down to `0600` since you might keep
secrets in there, and always written back tidy with sorted keys so it diffs nicely.

Don't use shell to store your secrets.

Leo figures out the type for you. Hand it real JSON and it stays JSON; hand it
anything else and it's a string. No fuss.

```sh
leo store set dancegif https://x.gif   # not valid JSON, so it's a string
leo store set retries 5                 # a number
leo store set limits '{"cpu":2}'        # an object
leo store set zip --string 7001         # force a string when you need to

leo store get limits --query .cpu       # query your data
leo store search dep                    # find keys fast (--values to search values too)
leo store type limits                   # object? number? string?
leo store list
leo store delete dancegif
```

A few conveniences worth knowing: read a value straight from a file with
`--file ./thing.json`, or from a pipe with a trailing `-`
(`echo '{"a":1}' | leo store set blob -`). And if you ever need the literal
string `"42"` instead of the number, quoting is your escape hatch: `'"42"'`.

Big files and images don't belong in a JSON blob, so Leo doesn't try. Just
store the path or URL and let the file live where it is.

## Copying things (macOS)

`leo clip` is the "copy this, you know what I mean" command. Give it a store key
or a path, and it does the sensible thing:

```sh
leo clip logo        # if that's a real file, Cmd+V pastes the actual file
leo clip limits      # otherwise you get text, and JSON comes across as JSON
```

Point it at an existing file and you get a proper file object on the clipboard,
the same thing Finder puts there with Cmd+C. Point it at anything else and you
get text. `--pretty` indents JSON, and `--file` or `--text` let you insist on a
mode when you want to.

NOTE: You need to permit your terminal app in Settings.

## Teaching Leo new commands

This is where Leo earns its keep. Any executable called `leo-<name>` living in
one of your command directories becomes `leo <name>` automatically, showing up
in `leo help` alongside the built-ins.

You can write these by hand, but `leo generate` gets you started with a working
template in the language of your choice (bash, zsh, python, node, or typescript,
with `sh`/`py`/`js`/`ts` as shorthand):

```sh
leo generate deploy --lang python --env work
```

When Leo runs your script, it hands you a little environment so your subcommand
can lean on the rest of Leo:

- `LEO_BIN` is the path to Leo itself, so you can call back into it.
- `LEO_STORE` is where the store lives.
- `LEO_CONFIG` is where the config lives.
- `LEO_COMMAND_PATHS` lists your command directories.

The second line of a generated script is a little `# leo:` (or `// leo:`) note.
Whatever you write there shows up as the one-line description in `leo help`, so
future-you knows what the command does. Your script gets its arguments
untouched, and calling back in is as easy as `"$LEO_BIN" store get something`.

## Where your commands live

Leo looks for subcommands in one or more named directories, in order. By default
that's just `~/.config/leo/commands`, but you can keep, say, work and personal
commands apart:

```toml
# ~/.config/leo/config.toml
store_path = "~/.config/leo/store.json"
gen_lang   = "bash"          # your favorite language for `leo generate`

[[command_path]]
name = "default"
path = "~/.config/leo/commands"

[[command_path]]
name = "work"
path = "~/work/leo-commands"
```

`~` and environment variables get expanded for you. Need an extra directory just
for this shell? Set `LEO_PATH` and Leo folds it in (it shows up under `[env]` in
help). When two commands share a name the earlier one wins, and Leo's own
built-ins always come first, so nothing you drop in can shadow them by accident.

Prefer not to hand-edit? `leo config setup` walks through each value (press Enter
to keep the current one) and can add new command-path sets for you.
`leo config show` prints the resolved paths and sets, and `leo config path`
prints the config file location.

## Releasing

Leo is distributed as signed and notarized macOS binaries through GitHub
Releases and the
[`stealthfactory/homebrew-leo`](https://github.com/stealthfactory/homebrew-leo)
tap. GoReleaser builds Apple Silicon and Intel archives, publishes their
checksums, and updates `Casks/leo.rb` in the tap.

Before the first release, configure these repository secrets under
**StealthFactory/leo → Settings → Secrets and variables → Actions → New
repository secret**:

- `HOMEBREW_TAP_TOKEN`
- `MACOS_SIGN_P12`
- `MACOS_SIGN_PASSWORD`
- `MACOS_NOTARY_KEY`
- `MACOS_NOTARY_KEY_ID`
- `MACOS_NOTARY_ISSUER_ID`

Create `HOMEBREW_TAP_TOKEN` at **GitHub → Settings → Developer settings →
Personal access tokens → Fine-grained tokens → Generate new token**. Set the
resource owner to `StealthFactory`, select only the `homebrew-leo` repository,
and grant **Repository permissions → Contents: Read and write**. No account or
organization permissions are needed. Copy the token immediately and save it as
the `HOMEBREW_TAP_TOKEN` repository secret in `StealthFactory/leo`, not in the
tap repository.

The remaining values require a paid Apple Developer Program membership:

1. In the Apple Developer portal, create a **Developer ID Application**
   certificate. Import its `.cer` file into Keychain Access, export the
   certificate and private key as a password-protected `.p12`, and save its
   password as `MACOS_SIGN_PASSWORD`.
2. In App Store Connect, open **Users and Access → Integrations → App Store
   Connect API**, create a team API key, and download the `.p8` file. Save the
   key ID as `MACOS_NOTARY_KEY_ID` and the issuer ID as
   `MACOS_NOTARY_ISSUER_ID`. Apple allows the `.p8` file to be downloaded only
   once.
3. Base64-encode both files on macOS without line wrapping:
   ```sh
   base64 -i DeveloperIDApplication.p12 | tr -d '\n' | pbcopy
   ```
   Save the clipboard value as `MACOS_SIGN_P12`, then repeat for the `.p8`
   file and save it as `MACOS_NOTARY_KEY`.

Keep the original certificate, private key, `.p12`, password, and `.p8` file in
a secure credential store. Never add them to either Git repository. See
[GoReleaser's notarization documentation](https://goreleaser.com/customization/sign/notarize/)
for additional background.

To check the build locally without signing, notarizing, or publishing:

```sh
brew install goreleaser
make release-check
```

To publish `vX.Y.Z`, commit the release, then create and push its tag:

```sh
git tag -a vX.Y.Z -m "leo vX.Y.Z"
git push origin vX.Y.Z
```

The `Release` workflow tests Leo, signs and notarizes both binaries, creates the
GitHub Release, publishes the cask, installs and smoke-tests it, and then removes
the legacy source formula from the tap. The formula remains available until
that first cask release succeeds, so a failed migration does not break current
installs.

Leo is a personal tool, built to stay small and get out of the way. Add the
commands you wish your shell had, and make it yours.

A Stealth Factory make -

<img height="200" alt="final-logo" src="https://github.com/user-attachments/assets/5ab1926a-606d-46b0-a9c8-f36b40eeb983" />

## License

MIT
