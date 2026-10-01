# Leo 🦁

Leo is a CLI tool that lets you scaffold and quickly create scripts and run them
through a unified interface. It's highly configurable and ships with a JSON data
store accessible from the unified CLI.

Out of the box Leo gives you a little JSON store to stash whatever you want and
a powerful "just copy this to my clipboard" command for the Mac, an extensible
ecosystem with the ability to organize the plugins into workspaces. Your existing
scripts are compatible with Leo as long as they're either in your `PATH` or
in Leo's command directory and prefixed with `leo-`.

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

## Basic usage

```sh
leo kv foo "bar"                           # save a value
leo kv foo                                 # -> bar
leo kv foo "baz"                           # asks before replacing bar
leo kv -d foo                              # delete it
leo kv limits '{"cpu":2,"mem":"4Gi"}'
leo kv --query '.cpu' limits               # -> 2
leo kv --secret api.token "s3cr3t"         # listed as SECRET
leo kv -s api                              # search, best matches first
leo generate hello                         # scaffold your first subcommand
leo hello world                            # ...and run it
leo help                                   # see everything, yours included
```

`kv` is an alias for `store`. Pick whichever reads better to you; they share the
same data and flags. You can set a value with `leo kv` and read it back with
`leo store`.

## Command reference

- [Help and version](#help-and-version)
- [Store (`store`, `kv`)](#the-store-store-kv)
- [Clipboard (`clip`)](#copying-things-clip)
- [Generate (`generate`, `gen`, `new`)](#teaching-leo-new-commands-generate-gen-new)
- [Configuration (`config`)](#configuration-config)
- [Workspaces and external commands](#workspaces-and-external-commands)
- [Environment variables](#environment-variables)

Here is the full built-in command tree. Aliases work anywhere the corresponding
command name does, including in help requests.

| Command | Aliases | What it does |
| --- | --- | --- |
| `leo store` | `leo kv` | Print all keys and values. |
| `leo store <key>` | `leo kv <key>` | Read a value, optionally with a query. |
| `leo store <key> <value>` | `leo kv <key> <value>` | Save a value; asks before replacing one. |
| `leo store --secret <key> <value>` | `leo kv --secret <key> <value>` | Save a value that lists as `SECRET`. |
| `leo store -s <text>` | `leo kv -s <text>` | Search keys and values, best matches first. |
| `leo store -d <key>` | `leo kv -d <key>` | Remove one key. |
| `leo clip <key-or-path>...` | — | Copy values or files to the macOS clipboard. |
| `leo generate <name>` | `leo gen`, `leo new` | Create a script in a workspace. |
| `leo config` | — | Show help for configuration commands. |
| `leo config show` | — | Print the effective configuration. |
| `leo config path` | — | Print the config file location. |
| `leo config setup` | — | Walk through configuration interactively. |
| `leo version` | — | Print the installed version. |
| `leo help [command...]` | — | Show general help or help for a command. |

In the usage examples, `<key>` means a required argument, `[value]` means an
optional one, and `...` means you can supply more than one. Leave the angle and
square brackets out when you type the command.

### Help and version

Run `leo` on its own, `leo help`, or `leo --help` to see the available commands.
External commands appear under their workspace names. For a particular command:

```sh
leo help kv
leo kv --help
leo generate -h
```

Every built-in accepts `-h` / `--help`. The root command also accepts `-v` /
`--version`; `leo version` prints the version too. There are no other global
flags. Leo does not install shell completion or expose a `completion` command.
To find a store key without TAB completion, [search](#searching-leo-kv--s-text)
for it with `leo kv -s`.

Flags belong to the command shown below and go before its arguments, as in
`leo new --lang zsh foo-bar`. Leo also accepts flags after the arguments, so
`leo new foo-bar --lang zsh` works too. Long flags with values accept either
`--query '.cpu'` or `--query='.cpu'`. Boolean flags default to off; adding one,
such as `--compact`, turns it on. Use `--` to end flag parsing when an argument
starts with a dash:

```sh
leo kv -- temperature -5
leo kv --string -- option --verbose
```

Successful built-in commands exit with status `0`. Errors exit with status `1`
and a message on stderr. Normal output goes to stdout, so you can pipe it or
capture it in a script.

### The store (`store`, `kv`)

Leo keeps its store in `~/.config/leo/store.json` by default. It's a plain JSON
object, with your keys at the top level. Writes replace the file atomically,
format the JSON with sorted keys, and set file permissions to `0600` (read and
write for your user). See [configuration](#configuration-config) to move it.
Once you have [secrets](#secrets---secret), Leo also keeps a reserved `$leo`
key in the same file to remember which keys they are.

```text
leo kv [flags] [<key> [<value>]]
```

What `leo kv` does depends on how many arguments you give it:

| You type | What happens |
| --- | --- |
| `leo kv` | Print every key and value. |
| `leo kv <key>` | Print one value. |
| `leo kv <key> <value>` | Save a value, asking before replacing an existing one. |
| `leo kv --secret <key> <value>` | Save a value that lists as `SECRET`. |
| `leo kv -s <text>` | Search keys and values, best matches first. |
| `leo kv -d <key>` | Delete a key. |

Keys are literal, case-sensitive strings. A key such as `deploy.host` is one
key, not a path into a nested object. Quote keys containing spaces. Any name
works as a key, including `list` or `help`, except `$leo`, which Leo reserves
for its own data. Single-quote it in the shell: `'$leo'`.

Each flag applies only to some of these actions, for example `--string` only
when saving a value. Passing a flag that doesn't apply is an error, so a typo
never gets silently ignored.

#### Saving a value: `leo kv <key> <value>`

Save a new value or replace an existing one. Leo recognizes valid JSON, so
numbers, booleans, arrays, objects, and `null` keep their types. Anything that
isn't valid JSON becomes a string.

```sh
leo kv greeting "hello there"              # string
leo kv retries 5                           # number
leo kv enabled true                        # boolean
leo kv fallback null                       # null
leo kv hosts '["api","worker"]'            # array
leo kv limits '{"cpu":2,"mem":"4Gi"}'      # object
leo kv homepage https://example.com        # string
```

| Flag | Short form | Default | Meaning |
| --- | --- | --- | --- |
| `--string` | — | Off | Store the input literally as a string, even if it looks like JSON. |
| `--json` | — | Off | Require valid JSON; reject invalid input instead of storing a string. |
| `--file <path>` | — | Unset | Read the value from a file. |
| `--force` | `-f` | Off | Replace an existing value without asking. |
| `--secret` | — | Off | Mark the value [secret](#secrets---secret). |

`--string` and `--json` are mutually exclusive. Shell quotes group arguments;
they don't force Leo to store a string. For example, `"42"` reaches Leo as `42`
and becomes a number. Use `--string` or pass JSON string quotes with `'"42"'`
when you want a string.

Instead of a value, you can use `--file` or a `-` to read stdin:

```sh
leo kv --string zip 7001
leo kv --json --file ./limits.json limits
printf '%s\n' '{"cpu":2}' | leo kv --json limits -
printf '%s' 'a multiline note' | leo kv --string note -
```

File input takes precedence if you also supply a value. File contents are read
as-is; stdin input has trailing newline characters removed. Piping input alone
isn't enough: include the `-`. To store a literal single dash, read it from a
file or pass it as a JSON string: `leo kv dash '"-"'`.

The confirmation includes the stored type, for example `set foo (string)`.
Replacing a key also prints its previous value. Files and URLs supplied as
ordinary values are just strings; use `--file` when you want the file's contents.
For images and other large files, storing their path keeps the JSON store small.

##### Replacing an existing value

Saving to a key that already exists shows the current value and asks first:

```text
$ leo kv foo baz
foo already exists: "bar"
update it to "baz"? [y/N]: y
set foo (string); previous value: "bar"
```

Anything other than `y` or `yes` keeps the old value and prints `foo unchanged`.
Pass `-f` to replace without asking. Saving the value a key already holds changes
nothing and doesn't ask. For a secret, the prompt shows `SECRET` in place of
both values.

Leo only asks when stdin is a terminal. In scripts and pipes, or when the value
itself comes from stdin with `-`, replacing an existing key without `-f` is an
error. That way a script never hangs waiting for an answer.

#### Reading a value: `leo kv <key>`

Read one value. A top-level string prints without quotes, ready to use in a
shell script. Other values print as indented JSON.

```sh
leo kv greeting                            # hello there
leo kv limits                              # indented object
leo kv -c limits                           # {"cpu":2,"mem":"4Gi"}
leo kv --query '.cpu' limits               # 2
leo kv --query '.mem' limits               # "4Gi"
leo kv -r --query '.mem' limits            # 4Gi
leo kv -r --query type limits              # object
```

| Flag | Short form | Default | Meaning |
| --- | --- | --- | --- |
| `--query <expression>` | — | Unset | Run a jq expression against this value. |
| `--raw` | `-r` | Off | Print string query results without JSON quotes. |
| `--compact` | `-c` | Off | Print each JSON result on one line. |

Queries run inside Leo using gojq; you don't need a separate `jq` executable.
Quote expressions so the shell doesn't interpret their punctuation. A query
can return several results, and Leo prints each separately:

```sh
leo kv -r --query '.[]' hosts
# api
# worker
```

You can combine `-r` and `-c` as `-rc`. Raw output takes precedence for string
results; compact output applies to the remaining JSON results. Without a query,
strings are already unquoted, so `-r` doesn't change them. To get a JSON-quoted
string, use `--query '.'` without `-r`. To see a value's JSON type (`object`,
`array`, `string`, `number`, `boolean`, or `null`), query `type` as shown above.

A missing key is an error. A stored `null` is a real value and prints `null`.
A secret prints its real value, so scripts can use it.

#### Listing everything: `leo kv`

Print every key and value, sorted by key. Each line contains the key, a tab,
and the full value as compact JSON. Strings stay quoted. An empty or not-yet-created
store produces no output.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--query <expression>` | Unset | Run a jq expression against each value separately. |

```sh
leo kv
leo kv --query 'type'                           # show each key and its JSON type
leo kv --query 'select(type == "object")'       # only object values
```

The query receives one stored value at a time, not the entire store. A query
that produces multiple results prints the key on each line. Queries that produce
no results omit that key. Invalid queries or runtime query errors fail the
command. Secrets always print as one `key<TAB>SECRET` line; queries don't run
on them.

#### Searching: `leo kv -s <text>`

Find keys by typing part of them. Search ignores case and prints the best
matches first, one `key<TAB>preview` line each. The preview is the value as
compact JSON, shortened to 80 bytes, including the trailing `...`. Leo ranks
matches in this order:

1. The key is the text: `api`.
2. The key starts with the text: `api.token`, `apiary`.
3. A word inside the key starts with the text. Words start after `.`, `-`,
   `_`, `/`, `:`, `@`, `#`, a space, or at a camelCase hump: `github.api`,
   `awsApiKey`.
4. The key contains the text: `rapid`.
5. The key contains the text's letters in order: `dh` finds `deploy.host`.
6. The value contains the text: `hosts`, holding `["api","worker"]`.

Within a group, earlier matches come first (for letters in order, the tightest
match), then keys sort alphabetically.

```sh
leo kv -s api
leo kv -s dh                                    # deploy.host
leo kv -s --query '.cpu' limits                 # project each match
```

| Flag | Short form | Default | Meaning |
| --- | --- | --- | --- |
| `--search` | `-s` | Off | Search for the argument instead of reading it as a key. |
| `--query <expression>` | — | Unset | Project each matching value through a jq expression. |

`-s` takes exactly one search term; quote it if it contains spaces. `--query`
runs after the match; it doesn't decide which keys match. Each query result gets
its own `key<TAB>result` line, with full compact JSON instead of a shortened
preview. Secret keys match by name only: Leo never searches their values, and
they print as `SECRET`. No matches means no output and a successful exit.

Leo can't offer TAB completion for keys without shell setup: zsh handles TAB
itself, before Leo runs. Searching with the first few letters is the zero-setup
way to find a key.

#### Deleting a key: `leo kv -d <key>`

Remove one key and print `deleted <key>`. The deletion happens immediately,
without a confirmation prompt. A missing key is an error. The long form is
`--delete`.

```sh
leo kv -d greeting
```

#### Secrets: `--secret`

Mark a value secret to keep it off your screen. Listings, search results,
replacement prompts, and confirmations show `SECRET` instead of the value.
Reading the key with `leo kv <key>` or copying it with `leo clip <key>` still
gives you the real value.

```sh
leo kv --secret api.token "s3cr3t"
pbpaste | leo kv --secret api.token -          # keeps the secret out of shell history
leo kv                                         # api.token	SECRET
leo kv api.token                               # s3cr3t
curl -H "Authorization: Bearer $(leo kv api.token)" https://example.com
```

The confirmation shows the type as `(string, secret)`. A secret stays secret
when you update it, even without `--secret`. To mark or unmark an existing key
without retyping its value, leave the value out:

```sh
leo kv --secret api.token                      # api.token is now secret
leo kv --secret=false api.token                # api.token is no longer secret
```

With a value, `--secret=false` saves it as a regular value. Listing and search
never run `--query` on a secret, since a query's results or errors could reveal
it; each secret prints one `key<TAB>SECRET` line instead.

Secrets are masked, not encrypted. `store.json` holds them in plain text with
`0600` permissions, and scripts that read `$LEO_STORE` directly see them. Leo
records which keys are secret under a reserved `$leo` key in the same file:

```json
{
  "$leo": {
    "secrets": ["api.token"]
  },
  "api.token": "s3cr3t",
  "greeting": "hello there"
}
```

Values keep their normal shape, and a value and its secret mark are always saved
in the same write. Copying or backing up `store.json` keeps your secrets marked.
`$leo` never appears in listings, search results, or `clip`, and you can't set,
mark, or delete it with `leo kv`. Leo removes it when no secrets remain. Scripts
that read `$LEO_STORE` with `jq` can drop it with `jq 'del(.["$leo"])'`. If
`$leo` isn't in the expected shape, Leo stops with an error rather than show
your secrets.

### Copying things (`clip`)

```text
leo clip [--file | --text] [--pretty] <key-or-path>...
```

`clip` uses the macOS clipboard. Give it a store key or an existing path. Leo
looks for a store key first, then a path on disk. A stored string is unquoted;
other stored values become compact JSON.

If every resolved argument is an existing path, Leo copies file objects, ready
to paste into Finder or an app that accepts files. Otherwise, it copies text,
joining multiple values with newlines. Image files also include image data on
the clipboard for apps that support pasting images.

```sh
leo kv logo "$PWD/logo.png"                   # assuming logo.png exists
leo clip logo                                 # copy the file named by the value
leo clip ./report.pdf ./chart.png             # copy two existing files
leo clip limits                               # copy JSON text
leo clip --pretty limits                      # copy indented JSON
leo clip --text logo                          # copy the stored path as text
leo clip --text "hello there"                 # copy literal text
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--file` | Off | Force file-object mode; every resolved path must exist. |
| `--text` | Off | Force text mode; allow unmatched arguments as literal text. |
| `--pretty` | Off | Indent stored JSON when copying it as text. |

`--file` and `--text` are mutually exclusive. Both still resolve store keys
first; `--text` changes the copy mode, not the lookup order. Text mode copies a
file's path, not its contents. File mode expands a leading `~` and resolves
relative paths from your current directory.

Without `--text` or `--file`, an argument that matches neither a key nor an
existing path is an error, and nothing is copied. This helps catch mistyped key
names. The command reports either `copied text to clipboard` or the number of
files copied. On other operating systems, `clip` returns an unsupported-platform
error.

### Teaching Leo new commands (`generate`, `gen`, `new`)

```text
leo generate [--lang <language>] [--workspace <name>] [--force] <name>
```

Generate a small working script, edit it, and run it through Leo. `gen` and `new`
are aliases for `generate`:

```sh
leo generate hello
leo gen --lang python --workspace work deploy
leo new --lang ts build
leo hello world
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--lang <language>` | Configured `gen_lang`, or `bash` | Choose the script template. |
| `--workspace <name>` | First workspace in search order | Choose the destination by workspace name. |
| `--force` | Off | Overwrite the target script if it already exists. |

The name must start with an ASCII letter or digit. The remaining characters can
be letters, digits, hyphens, or underscores. Use `hello`, not `leo-hello`: Leo
adds the prefix. Running `leo generate` without a name shows help.

| Language | Accepted shorthand | Generated filename | Interpreter |
| --- | --- | --- | --- |
| `bash` | `sh` | `leo-<name>` | `bash` |
| `zsh` | — | `leo-<name>` | `zsh` |
| `python` | `py` | `leo-<name>` | `python3` |
| `node` | `js`, `javascript` | `leo-<name>` | `node` |
| `typescript` | `ts` | `leo-<name>.mts` | `node` with native TypeScript support |

Language names are lowercase. The templates use `/usr/bin/env`, so the relevant
interpreter must be on your `PATH`. The TypeScript template uses erasable type
annotations and runs directly with Node; it doesn't invoke `tsx` or `ts-node`.

Leo creates the destination directory if needed and writes the script with
executable permissions. It refuses to replace an existing target unless you pass
`--force`. The output tells you where the file was written and how to run it.

`--workspace` only chooses where to write the file. All workspaces remain active
when running commands. If `LEO_PATH` is set, its first directory is also the
default generation destination; choose a named workspace explicitly when you
want a different one.

### Configuration (`config`)

Leo works without a config file. Run `leo config` for help with its three
subcommands. They take no arguments or flags other than `-h` / `--help`.

#### `leo config show`

Print the config file location, store location, default generation language, and
workspaces in search order. Paths have `~` and environment variables expanded.
This is the place to check when Leo is looking in a different directory than you
expected. Directories searched through the system `PATH` aren't listed here.

#### `leo config path`

Print only the config file location, even if the file hasn't been created yet:

```sh
leo config path
# /Users/you/.config/leo/config.toml
```

#### `leo config setup`

Walk through the store path, default generation language, and existing workspace
paths, then offer to add more named workspaces. Press Enter to keep the value
shown. New workspaces default to `<Leo config directory>/<name>-commands`.

Setup validates the language, creates the config's parent directory if needed,
and writes a new TOML file. It rewrites the configuration, so custom comments
won't be preserved. Workspaces added through `LEO_PATH` are temporary and aren't
written into the file by setup.

### Workspaces and external commands

A workspace is a named directory of commands. The default is
`~/.config/leo/commands`. Keep work and personal scripts together or split them
into directories:

```toml
# ~/.config/leo/config.toml
store_path = "~/.config/leo/store.json"
gen_lang = "bash"

[[workspace]]
name = "default"
path = "~/.config/leo/commands"

[[workspace]]
name = "work"
path = "~/work/leo-commands"
```

All settings are optional. Without `store_path`, Leo uses `store.json` in its
base config directory. Without `gen_lang`, it uses `bash`. Without workspace
entries, it includes the default `commands` directory. An unnamed workspace is
called `default`. Paths expand a leading `~` and environment variables; use
absolute paths or `~/...` to keep them independent of your current directory.

Leo scans directories in this order:

1. Directories in `LEO_PATH`, from left to right, grouped under `[env]` in help.
2. Workspaces from the config file, in the order written (or the default workspace).
3. Directories on the system `PATH`, grouped under `[$PATH]` in help.

The first command with a given name wins. Built-ins and their aliases always
win, so scripts named `leo-store`, `leo-kv`, `leo-gen`, or `leo-help` can't replace
those commands. Duplicate workspace paths are included only once. Directories
that can't be read are skipped, and scanning doesn't recurse into subdirectories.

To bring an existing script, name it `leo-<name>`, give it a suitable shebang,
make it executable, and put it in one of those directories. You can also keep a
recognized file extension; Leo strips it from the command name:

| Script filenames | Leo command |
| --- | --- |
| `leo-deploy`, `leo-deploy.sh`, `leo-deploy.bash`, `leo-deploy.zsh` | `leo deploy` |
| `leo-report.py` | `leo report` |
| `leo-build.js`, `leo-build.mjs`, `leo-build.cjs` | `leo build` |
| `leo-check.ts`, `leo-check.mts`, `leo-check.cts` | `leo check` |

Other extensions are ignored. The extension controls discovery; the script's
shebang controls how it runs. Leo discovers matching filenames even before
checking whether they can execute, so remember `chmod +x` for scripts you add by
hand.

Add a `# leo:` or `// leo:` comment within the first 20 lines to give the command
a one-line description in `leo help`:

```sh
#!/usr/bin/env bash
# leo: print the saved deployment host
"$LEO_BIN" kv deploy.host
```

External commands run from your current directory and receive their arguments,
stdin, stdout, and stderr directly. Leo forwards their exit status. On terminal
Ctrl-C, Leo waits for the script to stop: a script that handles the interrupt
can exit successfully, while an unhandled SIGINT returns status `130`. Flags after
an external command, including `--help`, go to the script itself; each script
owns its own flags. Generated templates handle `-h` and `--help` as their first
argument. `leo help <name>` shows Leo's entry for the command without running it.

### Environment variables

These variables affect Leo itself:

| Variable | Effect |
| --- | --- |
| `XDG_CONFIG_HOME` | Sets the parent of Leo's base config directory. When unset or empty, Leo uses `~/.config`; otherwise it uses `$XDG_CONFIG_HOME/leo` as its base. |
| `LEO_CONFIG` | Selects the TOML file to read and write, instead of `<base>/config.toml`. It does not move the default store or workspace. |
| `LEO_PATH` | Adds workspace directories ahead of the configured ones. Use the platform path separator (`:` on macOS/Linux). |
| `PATH` | Supplies the final directories searched for `leo-*` commands and the interpreters used by scripts. |

For example, to add a directory for one invocation:

```sh
LEO_PATH="$PWD/scripts" leo help
LEO_CONFIG="$PWD/leo.toml" leo config show
```

When Leo runs an external command, it also sets these variables for the child:

| Variable | Value |
| --- | --- |
| `LEO_BIN` | Path to the running Leo executable. Use it to call Leo from a script. |
| `LEO_STORE` | Effective store file path. This is information for your script, not a store-path override read by Leo. |
| `LEO_CONFIG` | Effective config file path, so calls back into Leo use the same config. |
| `LEO_WORKSPACE_PATHS` | Workspace paths in search order, separated by the platform path separator. It excludes directories discovered only through `PATH`. |

The directory containing `LEO_BIN` is prepended to the child's `PATH`, so a plain
`leo kv foo` inside your script uses the same installation too.

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
