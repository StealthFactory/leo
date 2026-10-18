use crate::{
    Result,
    config::{self, Config},
};
use std::os::unix::{fs::FileTypeExt, process::ExitStatusExt};
use std::{
    collections::HashSet,
    fs,
    io::{BufRead, BufReader},
    path::Path,
    process::{Command, Stdio},
};

pub struct External {
    pub name: String,
    pub group: String,
    pub short: String,
    pub file: String,
}

pub fn name(filename: &str) -> Option<String> {
    let base = filename.strip_prefix("leo-")?;
    if base.is_empty() {
        return None;
    }
    if let Some(ext) = Path::new(base).extension() {
        let ext = ext.to_str()?;
        if ![
            "sh", "bash", "zsh", "py", "js", "mjs", "cjs", "ts", "mts", "cts",
        ]
        .contains(&ext)
        {
            return None;
        }
        Some(base[..base.len() - ext.len() - 1].into())
    } else {
        Some(base.into())
    }
}

pub fn marker(file: &str) -> String {
    let Ok(f) = fs::File::open(file) else {
        return String::new();
    };
    for line in BufReader::new(f).lines().take(20) {
        let Ok(line) = line else {
            break;
        };
        if line.len() > 65535 {
            break;
        }
        let line = line.trim();
        if let Some(text) = line
            .strip_prefix("# leo:")
            .or_else(|| line.strip_prefix("// leo:"))
        {
            return text.trim().into();
        }
    }
    String::new()
}

pub fn discover(cfg: &Config) -> Vec<External> {
    let mut seen: HashSet<String> = [
        "clip", "config", "generate", "gen", "new", "store", "kv", "version", "help",
    ]
    .into_iter()
    .map(String::from)
    .collect();
    let mut result = vec![];
    let mut dirs: Vec<(String, String)> = cfg
        .workspaces
        .iter()
        .map(|w| (w.name.clone(), w.path.clone()))
        .collect();
    dirs.extend(
        config::getenv("PATH")
            .split(':')
            .filter(|p| !p.is_empty())
            .map(|p| ("$PATH".into(), p.into())),
    );
    for (group, dir) in dirs {
        let Ok(entries) = fs::read_dir(&dir) else {
            continue;
        };
        let mut entries: Vec<_> = entries.filter_map(std::result::Result::ok).collect();
        entries.sort_by_key(|e| e.file_name());
        for e in entries {
            if e.file_type().is_ok_and(|t| t.is_dir()) {
                continue;
            }
            let Some(name) = name(&e.file_name().to_string_lossy()) else {
                continue;
            };
            if !seen.insert(name.clone()) {
                continue;
            }
            let file = config::join(&dir, &e.file_name().to_string_lossy());
            result.push(External {
                name,
                group: group.clone(),
                short: marker(&file),
                file,
            });
        }
    }
    result
}

pub fn run(ext: &External, args: &[std::ffi::OsString], cfg: &Config) -> Result<()> {
    let exe = std::env::current_exe().unwrap_or_default();
    let dir = exe.parent().unwrap_or(Path::new("."));
    let mut cmd = Command::new(program(&ext.file)?);
    cmd.args(args)
        .stdin(Stdio::inherit())
        .stdout(Stdio::inherit())
        .stderr(Stdio::inherit())
        .env("LEO_BIN", &exe)
        .env("LEO_STORE", &cfg.store_path)
        .env("LEO_CONFIG", &cfg.config_path)
        .env(
            "LEO_WORKSPACE_PATHS",
            cfg.workspaces
                .iter()
                .map(|w| w.path.as_str())
                .collect::<Vec<_>>()
                .join(":"),
        )
        .env(
            "PATH",
            format!("{}:{}", dir.display(), config::getenv("PATH")),
        );
    // Ignore SIGINT only in the parent. POSIX spawn resets ignored handlers in
    // children only when requested, so restore the default in pre_exec.
    use std::os::unix::process::CommandExt;
    unsafe {
        cmd.pre_exec(|| {
            libc::signal(libc::SIGINT, libc::SIG_DFL);
            Ok(())
        });
    }
    let previous = unsafe { libc::signal(libc::SIGINT, libc::SIG_IGN) };
    let result = cmd.status();
    unsafe {
        libc::signal(libc::SIGINT, previous);
    }
    match result {
        Ok(status) if status.success() => Ok(()),
        Ok(status) => std::process::exit(
            status
                .code()
                .unwrap_or_else(|| 128 + status.signal().unwrap_or(0)),
        ),
        Err(e) => Err(config::io_error("fork/exec", &ext.file, &e)),
    }
}

/// Resolve before injecting the child's PATH, as exec.Command did in Go.
/// Relative PATH entries never implicitly authorize execution from the cwd.
pub fn program(name: &str) -> Result<String> {
    use std::os::unix::fs::PermissionsExt;
    if name.contains('/') {
        return Ok(name.into());
    }
    let paths = config::getenv("PATH");
    for dir in paths.split(':').filter(|_| !paths.is_empty()) {
        let dir = if dir.is_empty() { "." } else { dir };
        let candidate = config::join(dir, name);
        if fs::metadata(&candidate)
            .is_ok_and(|m| !m.is_dir() && m.permissions().mode() & 0o111 != 0)
        {
            if !Path::new(&candidate).is_absolute() {
                return Err(format!(
                    "exec: {}: cannot run executable found relative to current directory",
                    config::quote(name)
                ));
            }
            return Ok(candidate);
        }
    }
    Err(format!(
        "exec: {}: executable file not found in $PATH",
        config::quote(name)
    ))
}

pub fn terminal() -> bool {
    fs::File::open("/dev/stdin")
        .and_then(|f| f.metadata())
        .is_ok_and(|m| m.file_type().is_char_device())
}
