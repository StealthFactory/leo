use crate::{
    Result,
    config::{self, Config, Workspace},
};
use std::{fs, io::Write, os::unix::fs::OpenOptionsExt};

pub const LANGUAGES: &str = "bash|zsh|python|node|typescript";
pub fn normalize(lang: &str) -> Result<&str> {
    match lang {
        "bash" | "sh" => Ok("bash"),
        "zsh" => Ok("zsh"),
        "python" | "py" => Ok("python"),
        "node" | "js" | "javascript" => Ok("node"),
        "typescript" | "ts" => Ok("typescript"),
        _ => Err(format!(
            "unknown language {} (want {LANGUAGES})",
            config::quote(lang)
        )),
    }
}

pub fn choose<'a>(cfg: &'a Config, name: &str) -> Result<&'a Workspace> {
    if cfg.workspaces.is_empty() {
        return Err("no workspaces configured; run `leo config setup`".into());
    }
    if name.is_empty() {
        return Ok(&cfg.workspaces[0]);
    }
    cfg.workspaces
        .iter()
        .find(|w| w.name == name)
        .ok_or_else(|| format!("no workspace named {}", config::quote(name)))
}

pub fn generate(dir: &str, name: &str, lang: &str, force: bool) -> Result<String> {
    if !name
        .as_bytes()
        .first()
        .is_some_and(u8::is_ascii_alphanumeric)
        || !name
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
    {
        return Err(format!(
            "invalid subcommand name {} (use letters, digits, - and _)",
            config::quote(name)
        ));
    }
    let lang = normalize(lang)?;
    let (raw, suffix) = match lang {
        "bash" => (include_str!("../templates/subcommand.sh.tmpl"), ""),
        "zsh" => (include_str!("../templates/subcommand.zsh.tmpl"), ""),
        "python" => (include_str!("../templates/subcommand.py.tmpl"), ""),
        "node" => (include_str!("../templates/subcommand.js.tmpl"), ""),
        _ => (include_str!("../templates/subcommand.ts.tmpl"), ".mts"),
    };
    let path = config::join(dir, &format!("leo-{name}{suffix}"));
    if !force && fs::metadata(&path).is_ok() {
        return Err(format!("{path} already exists (use --force to overwrite)"));
    }
    config::mkdir_all(dir).map_err(|e| config::io_error("mkdir", dir, &e))?;
    let mut f = fs::OpenOptions::new()
        .write(true)
        .create(true)
        .truncate(true)
        .mode(0o755)
        .open(&path)
        .map_err(|e| config::io_error("open", &path, &e))?;
    f.write_all(raw.replace("{{.Name}}", name).as_bytes())
        .map_err(|e| config::io_error("write", &path, &e))?;
    Ok(path)
}
