This is the unchanged Go implementation from before the Rust port. It is a
test oracle, not a second implementation to maintain or ship. `tests/parity.py`
builds it and compares its behavior with the Rust executable in disposable
homes. The Rust build and the release archives do not include it.
