This is jaq-core 3.1.1 (MIT). The only runtime change adds
`Error::message`, an owned diagnostic with the same display and catch behavior
as existing native diagnostics. This lets the JSON adapter preserve gojq's
error messages while distinguishing them from a user's `error("message")`.

Only the library target is vendored; upstream example and test targets are
omitted. Leo verifies these changes through its recorded CLI compatibility suite.
