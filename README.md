# leo — a helpful assistant

A small, personal, extensible task-runner CLI. `leo` ships a handful of
built-in commands and dispatches everything else to **git-style external
subcommands** (`leo-<name>` scripts in any language) that you can add without
recompiling. It includes a JSON **object store**, a macOS **clipboard** helper,
a **scaffolding** command, and lazy-autoloaded shell **completion**.

## Install

### Homebrew

Once accepted into [homebrew-core](https://github.com/Homebrew/homebrew-core):

```sh
brew install leo
```

### From source

```sh
brew install go            # if you don't have Go
git clone https://github.com/StealthFactory/leo && cd leo
go build -ldflags "-X main.version=$(git describe --tags --always)" -o leo .
cp leo /usr/local/bin/     # or ~/.local/bin, anywhere on $PATH
```

## Quick start

```sh
leo config init                              # write ~/.config/leo/config.toml
leo store set limits '{"cpu":2,"mem":"4Gi"}' # auto-typed JSON object
leo store get limits --query .cpu            # -> 2
leo generate hello                           # scaffold ~/.config/leo/commands/leo-hello
leo hello world                              # runs your new subcommand
leo help                                     # built-ins + your subcommands, grouped
leo completion install                       # one-time: enable <TAB> completion
```

## Built-in commands

| Command | What it does |
| --- | --- |
| `leo store …` | Object store for arbitrary JSON values (see below). |
| `leo clip <key-or-path>…` | Copy a store value or file to the macOS clipboard. |
| `leo generate <name>` | Scaffold a new `leo-<name>` subcommand. |
| `leo config init\|show\|path` | Initialize / inspect configuration. |
| `leo completion install` | Install zsh completion into an on-`fpath` dir. |
| `leo version` | Print the version. |

### Object store

Values are **arbitrary JSON**, stored at `~/.config/leo/store.json`
(mode `0600` — values may be secrets), pretty-printed with sorted keys.

```sh
leo store set dancegif https://x.gif    # invalid JSON -> string
leo store set retries 5                  # -> number
leo store set zip --string 7001          # force string "7001"
leo store set cfg --file ./cfg.json      # read value from a file
echo '{"a":1}' | leo store set blob -    # read value from stdin
leo store get cfg --query '.items[]'     # project with jq (gojq)
leo store search dep --values            # match keys (and values with --values)
leo store type cfg                       # object|array|string|number|boolean|null
leo store list
leo store delete cfg
```

Values are **auto-typed**: valid JSON (`{…}`, `[…]`, `42`, `true`, `null`)
keeps its type; anything else (URLs, `1.2.3`, `07001`, phone numbers) stays a
string. Use `--string`/`--json` to force, and shell quoting as the escape hatch
(`'"42"'` stores the string `"42"`). Binary/images are stored **by reference**
(a path or URL string); `leo` never copies bytes.

### Clipboard (macOS)

`leo clip` is a universal "copy this". Each argument resolves as a **store key
first**, otherwise as a literal:

- A string that names an **existing file** → copied as a **file object**
  (`public.file-url`), so Cmd+V pastes the actual file.
- Anything else → text; JSON objects/arrays are copied as JSON text
  (`--pretty` to indent). `--file`/`--text` force a mode.

## Writing a subcommand

Any executable named `leo-<name>` inside a configured command-path set becomes
`leo <name>`. Scaffold one with `leo generate` (languages: `bash`, `zsh`,
`python`, `node`, `typescript`; aliases `sh`/`py`/`js`/`ts`):

```sh
leo generate deploy --lang python --set work
```

`leo` injects this environment when it runs your script:

| Variable | Meaning |
| --- | --- |
| `LEO_BIN` | Path to the `leo` binary (its dir is prepended to `$PATH`). |
| `LEO_STORE` | Resolved `store.json` path. |
| `LEO_CONFIG` | Resolved `config.toml` path. |
| `LEO_COMMAND_PATHS` | Colon-separated command-path set dirs. |

Line 2 of the script is a **marker comment** (`# leo:` or `// leo:`) whose text
becomes the one-line description shown in `leo help`. Scripts pass their own
flags straight through (leo does not parse them), and call back into leo via
`"$LEO_BIN" store get …`.

## Configuration (`~/.config/leo/config.toml`)

```toml
store_path = "~/.config/leo/store.json"
gen_lang   = "bash"                       # default `leo generate` language

[[command_path]]                          # searched in order; earlier wins
name = "default"
path = "~/.config/leo/commands"

[[command_path]]
name = "work"
path = "~/work/leo-commands"
```

- `~` and `$VARS` are expanded on load.
- `LEO_PATH` (colon-separated dirs) **prepends** extra sets for the current
  shell (grouped as `[env]` in help).
- `LEO_CONFIG` points to an alternate config file; `XDG_CONFIG_HOME` moves the
  base dir.
- **Built-ins always win** over external subcommands of the same name; earlier
  sets win over later ones.

## Completion

```sh
leo completion install     # writes _leo into a dir already on your $fpath
exec zsh                   # once, so compinit picks it up
```

Then just press `<TAB>`: subcommand names and **store keys** complete, the
latter ranked **prefix → substring → subsequence** (no shell re-sort, no file
names mixed in). The `_leo` function is lazily autoloaded — no shell-startup
cost. `leo completion bash|zsh|fish|powershell` prints the raw script if you
prefer to wire it up yourself.
