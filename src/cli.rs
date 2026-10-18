//! The public command grammar deliberately mirrors the existing Cobra CLI,
//! including interspersed flags, boolean assignments, aliases and diagnostics.
use crate::{
    Result, clip,
    config::{self, Config, FileConfig, Workspace},
    external::{self, External},
    json, query, scaffold,
    store::{self, Store},
};
use std::{
    collections::{BTreeMap, HashSet},
    fs,
    io::{self, BufRead, Write},
};

pub fn out(s: impl AsRef<[u8]>) {
    console_write(&mut io::stdout().lock(), s.as_ref());
}
pub fn err(s: impl AsRef<[u8]>) {
    console_write(&mut io::stderr().lock(), s.as_ref());
}
fn console_write(w: &mut impl Write, s: &[u8]) {
    if w.write_all(s)
        .is_err_and(|e| e.kind() == io::ErrorKind::BrokenPipe)
    {
        // Go terminates on SIGPIPE for stdout/stderr, while Rust ignores it.
        // Keep private pipes (e.g. pbcopy's stdin) on Rust's normal behavior.
        unsafe {
            libc::signal(libc::SIGPIPE, libc::SIG_DFL);
            libc::raise(libc::SIGPIPE);
        }
    }
}
fn line(s: impl AsRef<str>) {
    out(s.as_ref());
    out("\n");
}

#[derive(Default)]
pub struct Flags {
    values: BTreeMap<String, String>,
    pub args: Vec<String>,
}
impl Flags {
    pub fn has(&self, k: &str) -> bool {
        self.values.contains_key(k)
    }
    pub fn text(&self, k: &str) -> &str {
        self.values.get(k).map(String::as_str).unwrap_or("")
    }
    pub fn yes(&self, k: &str) -> bool {
        self.text(k) == "true"
    }
}

fn specs(command: &str) -> Vec<(&'static str, Option<char>, bool)> {
    let mut s = vec![("help", Some('h'), false)];
    match command {
        "" => s.push(("version", Some('v'), false)),
        "store" => s.extend([
            ("delete", Some('d'), false),
            ("search", Some('s'), false),
            ("force", Some('f'), false),
            ("secret", None, false),
            ("string", None, false),
            ("json", None, false),
            ("file", None, true),
            ("query", None, true),
            ("raw", Some('r'), false),
            ("compact", Some('c'), false),
        ]),
        "generate" => s.extend([
            ("workspace", None, true),
            ("lang", None, true),
            ("force", None, false),
        ]),
        "clip" => s.extend([
            ("file", None, false),
            ("text", None, false),
            ("pretty", None, false),
        ]),
        _ => {}
    }
    s
}

fn boolean(s: &str) -> Option<bool> {
    match s {
        "1" | "t" | "T" | "TRUE" | "true" | "True" => Some(true),
        "0" | "f" | "F" | "FALSE" | "false" | "False" => Some(false),
        _ => None,
    }
}

fn parse(command: &str, args: &[String]) -> Result<Flags> {
    let specs = specs(command);
    let mut f = Flags::default();
    let mut i = 0;
    while i < args.len() {
        let a = &args[i];
        i += 1;
        if a == "--" {
            f.args.extend_from_slice(&args[i..]);
            break;
        }
        if a == "-" || !a.starts_with('-') {
            f.args.push(a.clone());
            continue;
        }
        let (name, raw, display) = if let Some(long) = a.strip_prefix("--") {
            let (name, v) = long
                .split_once('=')
                .map_or((long, None), |(n, v)| (n, Some(v.to_owned())));
            let Some(spec) = specs.iter().find(|(n, _, _)| *n == name) else {
                return Err(format!("unknown flag: --{name}"));
            };
            (spec, v, format!("--{name}"))
        } else {
            // A shorthand cluster may assign its final flag with '=value'.
            let cluster = a[1..].char_indices().peekable();
            for (offset, c) in cluster {
                let Some(spec) = specs.iter().find(|(_, s, _)| *s == Some(c)) else {
                    return Err(format!("unknown shorthand flag: '{}' in {a}", c));
                };
                let rest = &a[1 + offset + c.len_utf8()..];
                let raw = rest.strip_prefix('=').map(|v| v.to_owned());
                let value = if let Some(raw) = raw.as_deref() {
                    boolean(raw).ok_or_else(||format!("invalid argument {} for \"-{}, --{}\" flag: strconv.ParseBool: parsing {}: invalid syntax",config::quote(raw),c,spec.0,config::quote(raw)))?
                } else {
                    true
                };
                f.values.insert(spec.0.into(), value.to_string());
                if raw.is_some() {
                    break;
                }
            }
            continue;
        };
        let value = if name.2 {
            if let Some(raw) = raw {
                raw
            } else {
                let Some(value) = args.get(i) else {
                    return Err(format!("flag needs an argument: {display}"));
                };
                i += 1;
                value.clone()
            }
        } else if let Some(raw) = raw {
            boolean(&raw).ok_or_else(||format!("invalid argument {} for {} flag: strconv.ParseBool: parsing {}: invalid syntax",config::quote(&raw),config::quote(&display),config::quote(&raw)))?.to_string()
        } else {
            "true".into()
        };
        f.values.insert(name.0.into(), value);
    }
    Ok(f)
}

fn canonical(name: &str) -> &str {
    match name {
        "kv" => "store",
        "gen" | "new" => "generate",
        n => n,
    }
}

// During command discovery Cobra has not installed its lazy help/version
// flags yet. An unassigned long flag therefore consumes the following token.
fn command_index(args: &[String]) -> Option<usize> {
    let mut i = 0;
    while i < args.len() {
        let s = &args[i];
        if s == "--" {
            break;
        }
        if s.starts_with('-') {
            if !s.contains('=') && (s.starts_with("--") || s.len() == 2) {
                i += 1;
            }
        } else if !s.is_empty() {
            return Some(i);
        }
        i += 1;
    }
    None
}

pub fn execute() -> Result<()> {
    let cfg = Config::load()?;
    let externals = external::discover(&cfg);
    let mut original: Vec<_> = std::env::args_os().skip(1).collect();
    let mut args: Vec<String> = original
        .iter()
        .map(|s| s.to_string_lossy().into_owned())
        .collect();
    let mut command = String::new();
    if let Some(i) = command_index(&args) {
        let name = canonical(&args[i]);
        if ["store", "clip", "generate", "config", "help", "version"].contains(&name) {
            command = name.into();
            args.remove(i);
        } else if let Some(ext) = externals.iter().find(|e| e.name == args[i]) {
            original.remove(i);
            return external::run(ext, &original, &cfg);
        } else {
            return Err(unknown(&args[i], &externals));
        }
    }
    if command == "help" {
        return help_command(&args, &externals);
    }
    if command == "config"
        && let Some(i) = command_index(&args)
        && ["path", "show", "setup"].contains(&args[i].as_str())
    {
        command = format!("config-{}", args.remove(i));
    }
    let f = parse(&command, &args)?;
    if f.yes("help") {
        out(help(&command, &externals));
        return Ok(());
    }
    match command.as_str() {
        "" => {
            if f.yes("version") {
                line(format!("leo version {}", env!("LEO_VERSION")));
                Ok(())
            } else {
                out(help("", &externals));
                Ok(())
            }
        }
        "config" => {
            out(help("config", &externals));
            Ok(())
        }
        "store" => {
            maximum(&f.args, 2)?;
            run_store(&cfg, &f)
        }
        "generate" => {
            maximum(&f.args, 1)?;
            run_generate(&cfg, &f, &externals)
        }
        "clip" => {
            if f.args.is_empty() {
                return Err("requires at least 1 arg(s), only received 0".into());
            }
            clip::run(&cfg, &f)
        }
        "version" | "config-path" | "config-show" | "config-setup" => {
            if let Some(arg) = f.args.first() {
                return Err(format!(
                    "unknown command {} for {}",
                    config::quote(arg),
                    config::quote(&format!("leo {}", command.replace('-', " ")))
                ));
            }
            match command.as_str() {
                "version" => line(format!("leo {}", env!("LEO_VERSION"))),
                "config-path" => line(&cfg.config_path),
                "config-show" => {
                    out(format!(
                        "config:   {}\nstore:    {}\ngen_lang: {}\nworkspaces (in search order):\n",
                        cfg.config_path, cfg.store_path, cfg.gen_lang
                    ));
                    for (i, w) in cfg.workspaces.iter().enumerate() {
                        line(format!("  {}. [{}] {}", i + 1, w.name, w.path));
                    }
                }
                _ => return setup(&cfg),
            }
            Ok(())
        }
        _ => unreachable!(),
    }
}

fn maximum(args: &[String], n: usize) -> Result<()> {
    if args.len() > n {
        Err(format!(
            "accepts at most {n} arg(s), received {}",
            args.len()
        ))
    } else {
        Ok(())
    }
}

fn help(command: &str, externals: &[External]) -> String {
    let text = match command {
        "store" => include_str!("help/store.txt"),
        "clip" => include_str!("help/clip.txt"),
        "generate" => include_str!("help/generate.txt"),
        "config" => include_str!("help/config.txt"),
        "config-path" => include_str!("help/config-path.txt"),
        "config-show" => include_str!("help/config-show.txt"),
        "config-setup" => include_str!("help/config-setup.txt"),
        "version" => include_str!("help/version.txt"),
        "help" => include_str!("help/help.txt"),
        _ => include_str!("help/root.txt"),
    };
    if !command.is_empty() || externals.is_empty() {
        return text.into();
    }
    let width = externals
        .iter()
        .map(|e| e.name.len())
        .max()
        .unwrap_or(0)
        .max(11);
    let start = text.find("Built-in commands:").unwrap();
    let end = text.find("\nFlags:").unwrap();
    let mut result = text[..start].to_owned();
    let builtins = [
        (
            "clip",
            "Copy a store value or path to the clipboard (macOS)",
        ),
        ("config", "Inspect and set up leo configuration"),
        (
            "generate",
            "Scaffold a new external subcommand (leo-<name>) into a workspace",
        ),
        (
            "store",
            "Get, set, search, and delete values in the JSON store (alias: kv)",
        ),
        ("version", "Print the leo version"),
    ];
    result.push_str("Built-in commands:\n");
    for (name, short) in builtins {
        result.push_str(&format!("  {name:<width$} {short}\n"));
    }
    let mut seen = HashSet::new();
    for e in externals {
        if !seen.insert(&e.group) {
            continue;
        }
        result.push_str(&format!("\n[{}]\n", e.group));
        let mut commands: Vec<_> = externals
            .iter()
            .filter(|other| other.group == e.group)
            .collect();
        commands.sort_by(|a, b| a.name.cmp(&b.name));
        for e in commands {
            result.push_str(&format!("  {:<width$} {}\n", e.name, e.short));
        }
    }
    result.push_str(&format!(
        "\nAdditional Commands:\n  {:<width$} Help about any command\n",
        "help"
    ));
    result.push_str(&text[end..]);
    result
}

fn help_command(args: &[String], externals: &[External]) -> Result<()> {
    let f = parse("help", args)?;
    if f.yes("help") {
        out(help("help", externals));
        return Ok(());
    }
    let Some(first) = f.args.first() else {
        out(help("", externals));
        return Ok(());
    };
    let first = canonical(first);
    if ["store", "clip", "generate", "config", "help", "version"].contains(&first) {
        let mut command = first.to_owned();
        if first == "config"
            && let Some(s) = f.args.get(1)
            && ["path", "show", "setup"].contains(&s.as_str())
        {
            command = format!("config-{s}");
        }
        out(help(&command, externals));
    } else if let Some(ext) = externals.iter().find(|e| e.name == first) {
        let mut s = String::new();
        if !ext.short.is_empty() {
            s.push_str(&ext.short);
            s.push_str("\n\n");
        }
        s.push_str(&format!(
            "Usage:\n  leo {} [flags]\n\nFlags:\n  -h, --help   help for {}\n",
            ext.name, ext.name
        ));
        out(s);
    } else {
        let topics = f
            .args
            .iter()
            .map(|s| format!("`{s}`"))
            .collect::<Vec<_>>()
            .join(" ");
        err(format!(
            "Unknown help topic [{topics}]\n{}",
            usage(&help("", externals))
        ));
    }
    Ok(())
}

fn usage(help: &str) -> String {
    let s = &help[help.find("Usage:").unwrap()..];
    let start = s.find("\nFlags:").unwrap();
    let end = s[start..]
        .find("\nUse ")
        .map(|i| i + start)
        .unwrap_or(s.len());
    format!("{}{}", &s[..start], &s[end..])
}

fn unknown(arg: &str, externals: &[External]) -> String {
    let mut names: Vec<&str> = vec!["clip", "config", "generate", "store", "version"];
    names.extend(externals.iter().map(|e| e.name.as_str()));
    names.push("help");
    let candidates: Vec<_> = names
        .into_iter()
        .filter(|n| n.starts_with(&arg.to_lowercase()) || distance(n, &arg.to_lowercase()) <= 2)
        .collect();
    let mut s = format!("unknown command {} for \"leo\"", config::quote(arg));
    if !candidates.is_empty() {
        s.push_str("\n\nDid you mean this?\n");
        for c in candidates {
            s.push_str(&format!("\t{c}\n"));
        }
    }
    s
}

fn distance(a: &str, b: &str) -> usize {
    let mut row: Vec<usize> = (0..=b.chars().count()).collect();
    for (i, x) in a.chars().enumerate() {
        let mut diagonal = row[0];
        row[0] = i + 1;
        for (j, y) in b.chars().enumerate() {
            let old = row[j + 1];
            row[j + 1] = (row[j] + 1)
                .min(old + 1)
                .min(diagonal + usize::from(x != y));
            diagonal = old;
        }
    }
    row[b.chars().count()]
}

fn run_generate(cfg: &Config, f: &Flags, externals: &[External]) -> Result<()> {
    if f.args.is_empty() {
        out(help("generate", externals));
        return Ok(());
    }
    let target = scaffold::choose(cfg, f.text("workspace"))?;
    let lang = if f.text("lang").is_empty() {
        &cfg.gen_lang
    } else {
        f.text("lang")
    };
    let path = scaffold::generate(&target.path, &f.args[0], lang, f.yes("force"))?;
    out(format!(
        "created {path}  (workspace: {})\nrun it with: leo {}\n",
        target.name, f.args[0]
    ));
    Ok(())
}

#[derive(Clone, Copy, PartialEq)]
enum Mode {
    List,
    Get,
    Set,
    Delete,
    Search,
    Mark,
}
fn run_store(cfg: &Config, f: &Flags) -> Result<()> {
    use Mode::*;
    let mode = if f.yes("delete") && f.yes("search") {
        return Err("-d and -s can't be used together".into());
    } else if f.yes("delete") {
        if f.args.len() != 1 {
            return Err("-d takes exactly one key: leo kv -d <key>".into());
        }
        Delete
    } else if f.yes("search") {
        if f.args.len() != 1 {
            return Err(
                "-s takes one search term (quote it if it has spaces): leo kv -s <text>".into(),
            );
        }
        Search
    } else if f.args.is_empty() {
        List
    } else if f.args.len() == 2 || !f.text("file").is_empty() {
        Set
    } else if f.has("secret") {
        Mark
    } else {
        Get
    };
    let rules: [(&str, &[Mode], &str); 8] = [
        ("string", &[Set], "when setting a value"),
        ("json", &[Set], "when setting a value"),
        ("file", &[Set], "when setting a value"),
        ("force", &[Set], "when setting a value"),
        (
            "secret",
            &[Set, Mark],
            "when setting a value or marking a key",
        ),
        (
            "query",
            &[Get, List, Search],
            "when getting, listing, or searching",
        ),
        ("raw", &[Get], "when getting a value"),
        ("compact", &[Get], "when getting a value"),
    ];
    for (name, modes, where_) in rules {
        if f.has(name) && !modes.contains(&mode) {
            return Err(match mode {
                Delete => format!("--{name} can't be combined with -d"),
                Search => format!("--{name} can't be combined with -s"),
                _ => format!("--{name} only applies {where_}"),
            });
        }
    }
    let mut st = Store::open(&cfg.store_path)?;
    match mode {
        List | Search => {
            let keys = if mode == Search {
                st.search(&f.args[0])
            } else {
                st.keys()
            };
            for k in keys {
                if st.secret(&k) {
                    line(format!("{k}\tSECRET"));
                    continue;
                }
                let v = st.get(&k).unwrap();
                if !f.text("query").is_empty() {
                    for r in query::run(v, f.text("query"))? {
                        line(format!("{k}\t{}", json::compact(&r)));
                    }
                } else {
                    out(format!("{k}\t"));
                    if mode == Search {
                        out(json::preview(v));
                    } else {
                        out(json::compact(v));
                    }
                    out("\n");
                }
            }
            Ok(())
        }
        Get => {
            let k = &f.args[0];
            let v = st.get(k).ok_or_else(|| format!("key not found: {k}"))?;
            if !f.text("query").is_empty() {
                for r in query::run(v, f.text("query"))? {
                    if f.yes("raw") && json::kind(&r) == "string" {
                        line(json::unquote(&r));
                    } else if f.yes("compact") {
                        line(json::compact(&r));
                    } else {
                        line(json::pretty(&r));
                    }
                }
            } else if json::kind(v) == "string" {
                line(json::unquote(v));
            } else if f.yes("compact") {
                line(json::compact(v));
            } else {
                line(json::pretty(v));
            }
            Ok(())
        }
        Set => set(&mut st, f),
        Mark => mark(&mut st, &f.args[0], f.yes("secret")),
        Delete => {
            st.delete(&f.args[0])?;
            line(format!("deleted {}", f.args[0]));
            Ok(())
        }
    }
}

fn display(v: &str, secret: bool) -> Vec<u8> {
    if secret {
        b"SECRET".to_vec()
    } else {
        json::preview(v)
    }
}
fn set(st: &mut Store, f: &Flags) -> Result<()> {
    let key = &f.args[0];
    let from_stdin = f.text("file").is_empty() && f.args.get(1).is_some_and(|s| s == "-");
    let input = if !f.text("file").is_empty() {
        String::from_utf8_lossy(
            &fs::read(f.text("file")).map_err(|e| config::io_error("open", f.text("file"), &e))?,
        )
        .into_owned()
    } else if from_stdin {
        use std::io::Read;
        let mut b = vec![];
        io::stdin().read_to_end(&mut b).map_err(|e| e.to_string())?;
        String::from_utf8_lossy(&b).trim_end_matches('\n').into()
    } else {
        f.args[1].clone()
    };
    let value = store::detect(&input, f.yes("string"), f.yes("json"))?;
    let was_secret = st.secret(key);
    let secret = if f.has("secret") {
        f.yes("secret")
    } else {
        was_secret
    };
    let old = st.get(key).map(str::to_owned);
    if let Some(cur) = &old {
        if json::compact(cur) == json::compact(&value) {
            if secret == was_secret {
                line(format!("{key} unchanged (same value)"));
                return Ok(());
            }
            return mark(st, key, secret);
        }
        if !f.yes("force") {
            if from_stdin || !external::terminal() {
                return Err(format!("{key} already exists; use -f to replace it"));
            }
            err(format!("{key} already exists: "));
            err(display(cur, was_secret));
            err("\nupdate it to ");
            err(display(&value, secret));
            err("? [y/N]: ");
            let mut answer = String::new();
            let _ = io::stdin().read_line(&mut answer);
            if !is_yes(&answer) {
                line(format!("{key} unchanged"));
                return Ok(());
            }
        }
    }
    let mut kind = json::kind(&value).to_owned();
    if secret {
        kind.push_str(", secret");
    }
    st.set(key, value, secret)?;
    out(format!("set {key} ({kind})"));
    if let Some(old) = old {
        out("; previous value: ");
        out(display(&old, was_secret));
    }
    out("\n");
    Ok(())
}

fn mark(st: &mut Store, key: &str, secret: bool) -> Result<()> {
    let changed = st.mark(key, secret)?;
    line(match (secret, changed) {
        (true, true) => format!("{key} is now secret"),
        (true, false) => format!("{key} is already secret"),
        (false, true) => format!("{key} is no longer secret"),
        (false, false) => format!("{key} isn't secret"),
    });
    Ok(())
}

fn is_yes(s: &str) -> bool {
    matches!(s.trim().to_lowercase().as_str(), "y" | "yes")
}
fn prompt(r: &mut impl BufRead, label: &str, default: &str) -> (String, bool) {
    out(if default.is_empty() {
        format!("{label}: ")
    } else {
        format!("{label} [{default}]: ")
    });
    let mut answer = String::new();
    let n = r.read_line(&mut answer).unwrap_or(0);
    let eof = n == 0 || !answer.ends_with('\n');
    (
        if answer.trim().is_empty() {
            default.into()
        } else {
            answer.trim().into()
        },
        eof,
    )
}

fn setup(cfg: &Config) -> Result<()> {
    let fc = config::read_file(&cfg.config_path)?;
    let default = |s: String, fallback: &str| if s.is_empty() { fallback.to_owned() } else { s };
    let mut store = default(fc.store_path, &cfg.store_path);
    let mut lang = default(fc.gen_lang, &cfg.gen_lang);
    let mut workspaces = fc.workspaces;
    if workspaces.is_empty() {
        workspaces.push(Workspace {
            name: "default".into(),
            path: config::join(&cfg.base_dir, "commands"),
        });
    }
    let mut r = io::stdin().lock();
    (store, _) = prompt(&mut r, "store_path", &store);
    loop {
        let (v, eof) = prompt(
            &mut r,
            &format!(
                "gen_lang [default language for leo generate] ({})",
                scaffold::LANGUAGES
            ),
            &lang,
        );
        match scaffold::normalize(&v) {
            Ok(canon) => {
                lang = canon.into();
                break;
            }
            Err(e) => {
                line(format!("  {e}"));
                if eof {
                    return Err(e);
                }
            }
        }
    }
    for w in &mut workspaces {
        (w.path, _) = prompt(&mut r, &format!("workspace [{}]", w.name), &w.path);
    }
    loop {
        let (answer, eof) = prompt(&mut r, "add another workspace? [y/N]", "");
        if !is_yes(&answer) {
            break;
        }
        let (name, eof_n) = prompt(&mut r, "  workspace name", "");
        let def = if name.is_empty() {
            String::new()
        } else {
            config::join(&cfg.base_dir, &format!("{name}-commands"))
        };
        let (path, eof_p) = prompt(&mut r, "  workspace path", &def);
        if name.is_empty() || path.is_empty() {
            line("  skipped: a workspace needs both a name and a path");
        } else {
            workspaces.push(Workspace { name, path });
        }
        if eof || eof_n || eof_p {
            break;
        }
    }
    config::write_file(
        &cfg.config_path,
        &FileConfig {
            store_path: store,
            gen_lang: lang,
            workspaces,
        },
    )?;
    line(format!("wrote {}", cfg.config_path));
    Ok(())
}
