#[test]
fn matches_recorded_cli_behavior() {
    let status = std::process::Command::new("python3")
        .args(["tests/parity.py", "--rust", env!("CARGO_BIN_EXE_leo")])
        .status()
        .expect("Python 3 is required for CLI parity tests");
    assert!(
        status.success(),
        "CLI behavior differs from recorded expectations; see the comparison above"
    );
}
