use crate::{
    Result,
    cli::{self, Flags},
    config::{self, Config},
    json,
    store::Store,
};
use serde::Serialize;
use std::os::unix::process::ExitStatusExt;
use std::{
    fs,
    io::Write,
    path::Path,
    process::{Command, Stdio},
};

pub fn absolute(p: &str) -> String {
    let p = config::expand_tilde(p);
    if Path::new(&p).is_absolute() {
        p
    } else {
        config::join(&config::cwd(), &p)
    }
}

pub fn image_type(p: &str) -> &'static str {
    match Path::new(p)
        .extension()
        .and_then(|s| s.to_str())
        .unwrap_or("")
        .to_lowercase()
        .as_str()
    {
        "png" => "public.png",
        "jpg" | "jpeg" => "public.jpeg",
        "gif" => "com.compuserve.gif",
        "tiff" | "tif" => "public.tiff",
        "bmp" => "com.microsoft.bmp",
        "heic" => "public.heic",
        "heif" => "public.heic",
        "webp" => "org.webmproject.webp",
        _ => "",
    }
}

fn exists(p: &str) -> bool {
    fs::metadata(absolute(p)).is_ok()
}
pub fn resolve(st: &Store, arg: &str, pretty: bool) -> (String, bool, bool) {
    if let Some(v) = st.get(arg) {
        if json::kind(v) == "string" {
            let s = json::unquote(v);
            let is_file = exists(&s);
            return (s, is_file, true);
        }
        return (
            if pretty {
                json::pretty(v)
            } else {
                json::compact(v)
            },
            false,
            true,
        );
    }
    if exists(arg) {
        (arg.into(), true, true)
    } else {
        (arg.into(), false, false)
    }
}

pub fn run(cfg: &Config, f: &Flags) -> Result<()> {
    if !cfg!(target_os = "macos") {
        return Err("leo clip is only supported on macOS".into());
    }
    if f.yes("file") && f.yes("text") {
        return Err("--file and --text are mutually exclusive".into());
    }
    let st = Store::open(&cfg.store_path)?;
    let mut texts = vec![];
    let mut missing = vec![];
    let mut all_files = true;
    for arg in &f.args {
        let (text, is_file, ok) = resolve(&st, arg, f.yes("pretty"));
        texts.push(text);
        all_files &= is_file;
        if !ok {
            missing.push(arg.as_str());
        }
    }
    if !missing.is_empty() && !f.yes("text") && !f.yes("file") {
        return Err(if missing.len() == 1 {
            format!(
                "hmm, there's no {} in your store, so there's nothing to copy",
                config::quote(missing[0])
            )
        } else {
            format!(
                "hmm, none of these are in your store: {} (nothing copied)",
                missing
                    .iter()
                    .map(|s| config::quote(s))
                    .collect::<Vec<_>>()
                    .join(", ")
            )
        });
    }
    if f.yes("file") || (!f.yes("text") && all_files) {
        copy_files(&texts)?;
        cli::out(format!("copied {} file(s) to clipboard\n", texts.len()));
    } else {
        copy_text(&texts.join("\n"))?;
        cli::out("copied text to clipboard\n");
    }
    Ok(())
}

fn exit_error(status: std::process::ExitStatus) -> String {
    if let Some(code) = status.code() {
        format!("exit status {code}")
    } else {
        format!(
            "signal: {}",
            match status.signal().unwrap_or(0) {
                libc::SIGINT => "interrupt",
                libc::SIGTERM => "terminated",
                libc::SIGKILL => "killed",
                _ => "unknown",
            }
        )
    }
}
fn spawn_error(name: &str, e: std::io::Error) -> String {
    if e.kind() == std::io::ErrorKind::NotFound {
        format!(
            "exec: {}: executable file not found in $PATH",
            config::quote(name)
        )
    } else {
        config::io_error("fork/exec", name, &e)
    }
}

pub fn copy_text(text: &str) -> Result<()> {
    let program = crate::external::program("pbcopy")
        .map_err(|e| format!("copying text to clipboard: {e}"))?;
    let mut child = Command::new(program)
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .map_err(|e| format!("copying text to clipboard: {}", spawn_error("pbcopy", e)))?;
    if let Some(mut stdin) = child.stdin.take() {
        let _ = stdin.write_all(text.as_bytes());
    }
    let status = child.wait().map_err(|e| e.to_string())?;
    if !status.success() {
        return Err(format!("copying text to clipboard: {}", exit_error(status)));
    }
    Ok(())
}

pub fn copy_files(paths: &[String]) -> Result<()> {
    if paths.is_empty() {
        return Err("no files to copy".into());
    }
    #[derive(Serialize)]
    struct Item {
        path: String,
        #[serde(rename = "type")]
        kind: &'static str,
    }
    let mut items = vec![];
    for p in paths {
        let path = absolute(p);
        if fs::metadata(&path).is_err() {
            return Err(format!("not an existing file: {p}"));
        }
        let kind = image_type(&path);
        items.push(Item { path, kind });
    }
    let program = crate::external::program("osascript")
        .map_err(|e| format!("copying files to clipboard: {e}: "))?;
    let output = Command::new(program)
        .args(["-l", "JavaScript", "-e", include_str!("clipboard.js")])
        .env(
            "LEO_CLIP_ITEMS",
            json::escape_html(&serde_json::to_string(&items).unwrap()),
        )
        .output()
        .map_err(|e| {
            format!(
                "copying files to clipboard: {}: ",
                spawn_error("osascript", e)
            )
        })?;
    let stdout = String::from_utf8_lossy(&output.stdout);
    let stderr = String::from_utf8_lossy(&output.stderr);
    if !output.status.success() {
        return Err(format!(
            "copying files to clipboard: {}: {}",
            exit_error(output.status),
            stderr.trim()
        ));
    }
    if stdout.trim() != "ok" {
        return Err(format!(
            "copying files to clipboard failed: {}",
            format!("{stdout} {stderr}").trim()
        ));
    }
    Ok(())
}
