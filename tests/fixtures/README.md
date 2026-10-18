# CLI compatibility expectations

`cli.json` records the original implementation's exit statuses, stdout, stderr,
and filesystem state after every step of 710 scenarios (1,184 invocations on
macOS). It was captured directly from the frozen implementation carried in
release v0.11.0, before its removal. The source commit and tree ID are recorded
in the file. The expected results were not generated from the Rust executable.

`tests/parity.py` runs the Rust CLI in disposable homes and compares it with
these expectations. Only the temporary root, executable identity, and version
token are normalized. Text fields use UTF-8 with Python's `surrogateescape`
encoding so even invalid UTF-8 bytes round-trip exactly. File modes, symlink
targets, and file bytes are checked along with console output. Clipboard
request tests run on macOS and use stubs; native clipboard checks remain opt-in.
Fixture setup and ordinary CLI steps use umask `022`, while the permission
scenarios also exercise `000`, `007`, and `077` explicitly.

Each scenario includes a SHA-256 digest of its inputs. Changing a scenario
without reviewing its expectations fails the suite. There is intentionally no
command to overwrite the baseline from the executable under test. When adding
or changing behavior, edit the scenario in `tests/parity.py`, derive the
expected results from the behavior being specified, and update only that
scenario's digest and expectations. Run a subset with:

```sh
cargo build --locked
python3 tests/parity.py --rust target/debug/leo --filter 'secret lifecycle'
```

The input digest is `case_digest(case)` in `tests/parity.py`. Record intentional
behavior changes in the relevant review rather than accepting an unexplained
bulk fixture update.

The former implementation's tests for CLI syntax, config loading and setup,
type detection, updates and prompts, secret masking, metadata handling, search,
querying, templates, discovery, child environment, interrupts, and clipboard
requests are covered by the recorded scenarios. Rust unit tests in
`src/tests.rs` retain the module checks for storage round trips and cleanup,
secret metadata, reserved keys, search ranking, raw JSON handling, template
permissions, language and workspace selection, external names and markers,
config serialization, and clipboard argument resolution and validation.
Invalid clipboard inputs are checked in a child process with a successful
`osascript` stub, so validation cannot depend on desktop clipboard access.

`builtins.json` lists the previous query engine's builtins. The suite checks
their availability as well as query results and errors in `cli.json`.
