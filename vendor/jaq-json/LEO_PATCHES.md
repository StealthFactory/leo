This is jaq-json 2.0.3, distributed under MIT, with compatibility changes for
Leo's previous gojq engine. Leo still executes queries entirely in Rust.

The changes preserve sorted object traversal, creation of missing containers
and array elements during assignment, numeric indexes from JSON numbers, and
legacy number formatting and native diagnostics (including caught errors).
They are exercised against the Go implementation by
`tests/parity.py`. Keep these changes when updating jaq-json.

Only the library target is vendored; upstream example and test targets are
omitted. Leo verifies these changes through its recorded CLI compatibility suite.
