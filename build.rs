use std::process::Command;

fn main() {
    println!("cargo:rerun-if-env-changed=LEO_VERSION");
    println!("cargo:rerun-if-changed=.git/HEAD");
    println!("cargo:rerun-if-changed=.git/refs");
    let version = std::env::var("LEO_VERSION").unwrap_or_else(|_| {
        Command::new("git")
            .args(["describe", "--tags", "--always", "--dirty"])
            .output()
            .ok()
            .filter(|o| o.status.success())
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_owned())
            .unwrap_or_else(|| "dev".into())
    });
    println!("cargo:rustc-env=LEO_VERSION={version}");
}
