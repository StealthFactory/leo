use crate::{Result, json};
use serde::Deserialize;
use std::io::Write;
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt};
use std::{
    collections::HashSet,
    env, fs, io,
    path::{Component, Path, PathBuf},
};

#[derive(Clone, Default)]
pub struct Workspace {
    pub name: String,
    pub path: String,
}

#[derive(Default)]
pub struct FileConfig {
    pub store_path: String,
    pub gen_lang: String,
    pub workspaces: Vec<Workspace>,
}

impl<'de> Deserialize<'de> for Workspace {
    fn deserialize<D: serde::Deserializer<'de>>(d: D) -> std::result::Result<Self, D::Error> {
        struct Visitor;
        impl<'de> serde::de::Visitor<'de> for Visitor {
            type Value = Workspace;
            fn expecting(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
                f.write_str("a table")
            }
            fn visit_map<M: serde::de::MapAccess<'de>>(
                self,
                mut map: M,
            ) -> std::result::Result<Workspace, M::Error> {
                let mut w = Workspace::default();
                while let Some(key) = map.next_key::<String>()? {
                    match key.to_lowercase().as_str() {
                        "name" => w.name = map.next_value()?,
                        "path" => w.path = map.next_value()?,
                        _ => {
                            map.next_value::<serde::de::IgnoredAny>()?;
                        }
                    }
                }
                Ok(w)
            }
        }
        d.deserialize_map(Visitor)
    }
}
impl<'de> Deserialize<'de> for FileConfig {
    fn deserialize<D: serde::Deserializer<'de>>(d: D) -> std::result::Result<Self, D::Error> {
        struct Visitor;
        impl<'de> serde::de::Visitor<'de> for Visitor {
            type Value = FileConfig;
            fn expecting(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
                f.write_str("a table")
            }
            fn visit_map<M: serde::de::MapAccess<'de>>(
                self,
                mut map: M,
            ) -> std::result::Result<FileConfig, M::Error> {
                let mut fc = FileConfig::default();
                while let Some(key) = map.next_key::<String>()? {
                    match key.to_lowercase().as_str() {
                        "store_path" => fc.store_path = map.next_value()?,
                        "gen_lang" => fc.gen_lang = map.next_value()?,
                        "workspace" => fc.workspaces = map.next_value()?,
                        _ => {
                            map.next_value::<serde::de::IgnoredAny>()?;
                        }
                    }
                }
                Ok(fc)
            }
        }
        d.deserialize_map(Visitor)
    }
}

pub struct Config {
    pub base_dir: String,
    pub config_path: String,
    pub store_path: String,
    pub gen_lang: String,
    pub workspaces: Vec<Workspace>,
}

pub fn getenv(key: &str) -> String {
    env::var(key).unwrap_or_default()
}

pub fn mkdir_all(path: impl AsRef<Path>) -> io::Result<()> {
    fs::DirBuilder::new()
        .recursive(true)
        .mode(0o755)
        .create(path)
}

pub fn cwd() -> String {
    use std::os::unix::fs::MetadataExt;
    let pwd = getenv("PWD");
    if Path::new(&pwd).is_absolute()
        && let (Ok(a), Ok(b)) = (fs::metadata("."), fs::metadata(&pwd))
        && a.dev() == b.dev()
        && a.ino() == b.ino()
    {
        return pwd;
    }
    env::current_dir()
        .unwrap_or_default()
        .to_string_lossy()
        .into_owned()
}

pub fn clean(p: impl AsRef<Path>) -> String {
    let p = p.as_ref();
    let mut out = PathBuf::new();
    for c in p.components() {
        match c {
            Component::CurDir => {}
            Component::ParentDir => {
                if out.file_name().is_some_and(|s| s != "..") {
                    out.pop();
                } else if !out.has_root() {
                    out.push("..");
                }
            }
            c => out.push(c.as_os_str()),
        }
    }
    if out.as_os_str().is_empty() {
        ".".into()
    } else {
        out.to_string_lossy().into_owned()
    }
}

pub fn join(a: &str, b: &str) -> String {
    if a.is_empty() {
        return if b.is_empty() {
            String::new()
        } else {
            clean(b)
        };
    }
    if b.is_empty() {
        return clean(a);
    }
    clean(format!("{a}/{b}"))
}

pub fn expand(p: &str) -> String {
    let b = p.as_bytes();
    let mut out = String::new();
    let mut start = 0;
    let mut i = 0;
    while i + 1 < b.len() {
        if b[i] != b'$' {
            i += 1;
            continue;
        }
        out.push_str(&p[start..i]);
        let j = i + 1;
        if b[j] == b'{' {
            if let Some(end) = b[j + 1..]
                .iter()
                .position(|b| *b == b'}')
                .map(|n| j + 1 + n)
            {
                if end > j + 1 {
                    out.push_str(&getenv(&p[j + 1..end]));
                }
                i = end + 1;
            } else {
                i = j + 1;
            }
        } else if b[j].is_ascii_digit() || b"*#$@!?-".contains(&b[j]) {
            out.push_str(&getenv(&p[j..j + 1]));
            i = j + 1;
        } else {
            let mut end = j;
            while end < b.len() && (b[end].is_ascii_alphanumeric() || b[end] == b'_') {
                end += 1;
            }
            if end == j {
                out.push('$');
            } else {
                out.push_str(&getenv(&p[j..end]));
            }
            i = end;
        }
        start = i;
    }
    out.push_str(&p[start..]);
    expand_tilde(&out)
}

pub fn expand_tilde(p: &str) -> String {
    if p == "~" {
        getenv("HOME")
    } else if let Some(rest) = p.strip_prefix("~/") {
        join(&getenv("HOME"), rest)
    } else {
        p.to_owned()
    }
}

pub fn io_error(op: &str, path: &str, e: &io::Error) -> String {
    let detail = match e.raw_os_error() {
        Some(libc::ENOENT) => "no such file or directory".into(),
        Some(libc::EISDIR) => "is a directory".into(),
        Some(libc::ENOTDIR) => "not a directory".into(),
        Some(libc::EACCES) => "permission denied".into(),
        Some(libc::EEXIST) => "file exists".into(),
        _ => e
            .to_string()
            .split(" (os error ")
            .next()
            .unwrap_or("")
            .to_lowercase(),
    };
    format!("{op} {path}: {detail}")
}

pub fn read_file(path: &str) -> Result<FileConfig> {
    let b = match fs::read_to_string(path) {
        Ok(b) => b,
        Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(FileConfig::default()),
        Err(e) => return Err(format!("reading {path}: {}", io_error("open", path, &e))),
    };
    let mut fc: FileConfig =
        toml::from_str(&b).map_err(|e| format!("parsing {path}: {}", toml_error(&b, &e)))?;
    for w in &mut fc.workspaces {
        if w.name.is_empty() {
            w.name = "default".into();
        }
    }
    Ok(fc)
}

fn toml_error(source: &str, error: &toml::de::Error) -> String {
    let start = error
        .span()
        .map_or(source.len(), |s| s.start.min(source.len()));
    let line = source[..start].bytes().filter(|b| *b == b'\n').count() + 1;
    let current = source.lines().nth(line - 1).unwrap_or("");
    let message = error.message();
    let mut table = String::new();
    for text in source.lines().take(line) {
        let text = text.trim();
        if text.starts_with('[') && text.ends_with(']') {
            table = text.trim_matches(['[', ']']).to_owned();
        }
    }
    let key = current
        .split_once('=')
        .map(|(k, _)| k.trim().trim_matches(['\'', '"']).to_owned());
    let key = key.map(|k| {
        if table.is_empty() {
            k
        } else {
            format!("{table}.{k}")
        }
    });
    let last = || {
        key.as_ref().map_or(String::new(), |k| {
            format!(" (last key {})", json::go_quote(k))
        })
    };
    let detail = if message.starts_with("invalid type:") {
        let typ = if message.contains("integer") {
            "int64"
        } else if message.contains("float") {
            "float64"
        } else if message.contains("boolean") {
            "bool"
        } else if message.contains("string") {
            "string"
        } else if message.contains("sequence") {
            "[]any"
        } else {
            "map[string]any"
        };
        let dest = if message.ends_with("a string") {
            "string"
        } else if message.ends_with("a sequence") {
            "slice"
        } else {
            "struct"
        };
        format!(
            "{}: incompatible types: TOML value has type {typ}; destination has type {dest}",
            last()
        )
    } else if message.starts_with("duplicate key") {
        format!(
            "{}: Key '{}' has already been defined.",
            last(),
            key.as_deref().unwrap_or("")
        )
    } else if source.trim_end().ends_with('=') {
        format!("{}: unexpected EOF; expected value", last())
    } else if message.contains("string") && start >= source.len() {
        format!("{}: unexpected EOF; expected '\"'", last())
    } else {
        let c = source.as_bytes().get(start).copied().unwrap_or(0);
        let c = if c == 0 {
            "\\x00".into()
        } else {
            (c as char).to_string()
        };
        if message.contains("newline") || message.contains("end of line") {
            format!(
                ": expected a top-level item to end with a newline, comment, or EOF, but got '{c}' instead"
            )
        } else if message.contains("table header") || current.starts_with('[') {
            format!(": expected '.' or ']' to end table name, but got '{c}' instead")
        } else if message.contains("expected `.`")
            || message.contains("expected '.', '='")
            || message.contains("key with no value")
        {
            format!(": expected '.' or '=', but got '{c}' instead")
        } else {
            return format!("toml: line {line}{}: {message}", last());
        }
    };
    format!("toml: line {line}{detail}")
}

impl Config {
    pub fn load() -> Result<Self> {
        let xdg = getenv("XDG_CONFIG_HOME");
        let base = if xdg.is_empty() {
            join(&getenv("HOME"), ".config/leo")
        } else {
            join(&expand(&xdg), "leo")
        };
        let path = getenv("LEO_CONFIG");
        let path = if path.is_empty() {
            join(&base, "config.toml")
        } else {
            expand(&path)
        };
        let fc = read_file(&path)?;
        let mut c = Self {
            store_path: if fc.store_path.is_empty() {
                join(&base, "store.json")
            } else {
                expand(&fc.store_path)
            },
            gen_lang: if fc.gen_lang.is_empty() {
                "bash".into()
            } else {
                fc.gen_lang
            },
            base_dir: base.clone(),
            config_path: path,
            workspaces: vec![],
        };
        let mut seen = HashSet::new();
        let mut add = |name: String, p: String| {
            let p = expand(&p);
            if !p.is_empty() && seen.insert(p.clone()) {
                c.workspaces.push(Workspace { name, path: p });
            }
        };
        for p in getenv("LEO_PATH")
            .split(':')
            .filter(|s| !s.trim().is_empty())
        {
            add("env".into(), p.into());
        }
        if fc.workspaces.is_empty() {
            add("default".into(), join(&base, "commands"));
        } else {
            for w in fc.workspaces {
                add(w.name, w.path);
            }
        }
        Ok(c)
    }
}

pub fn write_file(path: &str, fc: &FileConfig) -> Result<()> {
    fn quote(s: &str) -> String {
        format!("\"{}\"", s.replace('\\', "\\\\").replace('"', "\\\""))
    }
    let mut s = format!(
        "# leo configuration (TOML). All keys are optional.\n\n# Where the object store lives.\nstore_path = {}\n\n# Default language for `leo generate`: bash|zsh|python|node|typescript\ngen_lang = {}\n\n# One or more named workspaces, searched in the order listed.\n# Earlier workspaces win on name collisions and appear first in `leo help`.\n",
        quote(&fc.store_path),
        quote(&fc.gen_lang)
    );
    for w in &fc.workspaces {
        s.push_str(&format!(
            "\n[[workspace]]\nname = {}\npath = {}\n",
            quote(&w.name),
            quote(&w.path)
        ));
    }
    let parent = Path::new(path)
        .parent()
        .filter(|p| !p.as_os_str().is_empty())
        .unwrap_or(Path::new("."));
    mkdir_all(parent).map_err(|e| io_error("mkdir", &parent.to_string_lossy(), &e))?;
    let mut f = fs::OpenOptions::new()
        .write(true)
        .create(true)
        .truncate(true)
        .mode(0o644)
        .open(path)
        .map_err(|e| io_error("open", path, &e))?;
    f.write_all(s.as_bytes())
        .map_err(|e| io_error("write", path, &e))
}

pub fn quote(s: &str) -> String {
    json::go_quote(s)
}
