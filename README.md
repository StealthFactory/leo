# Leo 🦁

Meet Leo, a tiny command-line sidekick you can teach new tricks in seconds.

Out of the box Leo gives you a few genuinely useful things: a little JSON store
to stash whatever you want, a "just copy this to my clipboard" command for the
Mac, and tab-completion that actually knows your data. But the fun part is that
anything named `leo-<something>` on your machine instantly becomes
`leo <something>`. Write a shell script, a Python file, a snippet of
TypeScript, whatever you like, drop it in, and Leo picks it up. No plugins to
register, no rebuilds, no ceremony.

Think of it as your own personal `git`: a small, stable core with a growing pile
of subcommands that are entirely yours.

## Getting Leo

```sh
brew install leo
```

## Sixty seconds with Leo

```sh
leo config init                              # set up ~/.config/leo
leo store set limits '{"cpu":2,"mem":"4Gi"}' # stash some JSON
leo store get limits --query .cpu            # -> 2
leo generate hello                           # scaffold your first subcommand
leo hello world                              # ...and run it
leo help                                     # see everything, yours included
```

That's the whole loop: keep some data around, teach Leo a new command, run it.

## The store

Leo keeps a JSON object store at `~/.config/leo/store.json`, locked down to
`0600` since you might keep secrets in there, and always written back tidy with
sorted keys so it diffs nicely.

The nice touch: Leo figures out the type for you. Hand it real JSON and it stays
JSON; hand it anything else and it's a string. No fuss.

```sh
leo store set dancegif https://x.gif   # not valid JSON, so it's a string
leo store set retries 5                 # a number
leo store set limits '{"cpu":2}'        # an object
leo store set zip --string 7001         # force a string when you need to

leo store get limits --query .cpu       # slice it up with jq
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

## Teaching Leo new commands

This is where Leo earns its keep. Any executable called `leo-<name>` living in
one of your command directories becomes `leo <name>` automatically, showing up
in `leo help`, in tab-completion, everywhere.

You can write these by hand, but `leo generate` gets you started with a working
template in the language of your choice (bash, zsh, python, node, or typescript,
with `sh`/`py`/`js`/`ts` as shorthand):

```sh
leo generate deploy --lang python --set work
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

## Tab-completion that gets it

Set it up once:

```sh
leo completion install     # tucks _leo into a directory zsh already searches
exec zsh                   # so it gets picked up
```

After that, just press `<TAB>`. Command names complete, and so do your store
keys, ranked the way you'd actually want them: exact prefixes first, then
substring matches, then loose subsequence matches, with no file names muddled
in. It loads lazily, so it costs your shell nothing at startup.

Prefer to wire things up yourself? `leo completion bash|zsh|fish|powershell`
prints the raw script and gets out of your way.

## Releasing

Leo installs from a tagged release, and the Homebrew formula lives in
`packaging/Formula/leo.rb`. Cutting a new version and checking the formula goes
like this (using `vX.Y.Z` for your next version, e.g. `v0.2.3`):

1. Commit whatever you want in the release. Tags capture committed code, not
   your working tree, so anything still uncommitted won't be in it.

2. Tag the release and push it. The checks fetch the tarball from GitHub, so the
   tag has to be pushed, not just created locally:
   ```sh
   git tag -a vX.Y.Z -m "leo vX.Y.Z"
   git push origin vX.Y.Z
   ```

3. Grab the tarball checksum:
   ```sh
   make formula-sha TAG=vX.Y.Z
   ```

4. Point the formula at the new release: in `packaging/Formula/leo.rb`, set `url`
   to the `vX.Y.Z` tarball and `sha256` to the value from step 3.

5. Run the readiness check, which validates the formula the way homebrew-core
   would (Go tests, `brew style`, the tarball and its checksum, `brew audit
   --new`, a source build, and `brew test`):
   ```sh
   make homebrew-core-readiness-check
   ```
   It must end in `PASS`. It stages the formula into a throwaway tap and cleans
   up after itself, so nothing is left installed.

Once it passes, the single file `packaging/Formula/leo.rb` is what you submit to
homebrew-core. See `packaging/SUBMITTING.md` for the submission steps.

Leo is a personal tool, built to stay small and get out of the way. Add the
commands you wish your shell had, and make it yours.
