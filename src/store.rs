use crate::{Result, config, json};
use serde_json::value::RawValue;
use std::os::unix::fs::PermissionsExt;
use std::{
    collections::{BTreeMap, BTreeSet},
    fs,
    io::{self, Write},
    path::Path,
};

pub const META: &str = "$leo";
pub const RESERVED: &str = "\"$leo\" is reserved for leo's own data; pick another key";
type Object = BTreeMap<String, Box<RawValue>>;

pub struct Store {
    path: String,
    pub data: Object,
    pub secrets: BTreeSet<String>,
    meta: Object,
}

impl Store {
    pub fn open(path: &str) -> Result<Self> {
        let b = match fs::read(path) {
            Ok(b) => b,
            Err(e) if e.kind() == io::ErrorKind::NotFound => vec![],
            Err(e) => {
                return Err(format!(
                    "reading store {path}: {}",
                    config::io_error("open", path, &e)
                ));
            }
        };
        let s = String::from_utf8_lossy(&b);
        let mut st = Self {
            path: path.into(),
            data: BTreeMap::new(),
            secrets: BTreeSet::new(),
            meta: BTreeMap::new(),
        };
        if s.trim().is_empty() {
            return Ok(st);
        }
        if s.trim() != "null" {
            st.data = json::object(&s)
                .map_err(|e| format!("parsing store {path}: {}", json_error(&s, &e)))?;
        }
        if let Some(raw) = st.data.remove(META) {
            st.meta = json::object(raw.get()).map_err(|_| {
                format!("parsing store {path}: \"$leo\" must be an object of leo metadata")
            })?;
            if let Some(raw) = st.meta.get("secrets") {
                let keys: Option<Vec<Option<String>>> =
                    serde_json::from_str(&json::repair_surrogates(raw.get())).map_err(|_| {
                        format!("parsing store {path}: \"$leo\" secrets must be a list of keys")
                    })?;
                for k in keys
                    .unwrap_or_default()
                    .into_iter()
                    .map(Option::unwrap_or_default)
                {
                    if st.data.contains_key(&k) {
                        st.secrets.insert(k);
                    }
                }
            }
        }
        Ok(st)
    }

    pub fn get(&self, key: &str) -> Option<&str> {
        self.data.get(key).map(|v| v.get())
    }
    pub fn secret(&self, key: &str) -> bool {
        self.secrets.contains(key)
    }
    pub fn keys(&self) -> Vec<String> {
        self.data.keys().cloned().collect()
    }

    pub fn save(&mut self) -> Result<()> {
        if self.secrets.is_empty() {
            self.meta.remove("secrets");
        } else {
            self.meta.insert(
                "secrets".into(),
                RawValue::from_string(serde_json::to_string(&self.secrets).unwrap()).unwrap(),
            );
        }
        let mut all: BTreeMap<&str, &RawValue> = self
            .data
            .iter()
            .map(|(k, v)| (k.as_str(), v.as_ref()))
            .collect();
        let metadata = RawValue::from_string(serde_json::to_string(&self.meta).unwrap()).unwrap();
        if !self.meta.is_empty() {
            all.insert(META, &metadata);
        }
        let b = format!(
            "{}\n",
            json::pretty(&json::escape_html(&serde_json::to_string(&all).unwrap()))
        );
        let parent = Path::new(&self.path)
            .parent()
            .filter(|p| !p.as_os_str().is_empty())
            .unwrap_or(Path::new("."));
        config::mkdir_all(parent)
            .map_err(|e| config::io_error("mkdir", &parent.to_string_lossy(), &e))?;
        let mut tmp = tempfile::Builder::new()
            .prefix(".store-")
            .suffix(".tmp")
            .tempfile_in(parent)
            .map_err(|e| config::io_error("open", &self.path, &e))?;
        tmp.as_file()
            .set_permissions(fs::Permissions::from_mode(0o600))
            .map_err(|e| e.to_string())?;
        tmp.write_all(b.as_bytes())
            .map_err(|e| config::io_error("write", &self.path, &e))?;
        fs::rename(tmp.path(), &self.path).map_err(|e| config::io_error("rename", &self.path, &e))
    }

    pub fn set(&mut self, key: &str, value: String, secret: bool) -> Result<()> {
        if key == META {
            return Err(RESERVED.into());
        }
        self.data.insert(
            key.into(),
            RawValue::from_string(value).map_err(|e| e.to_string())?,
        );
        if secret {
            self.secrets.insert(key.into());
        } else {
            self.secrets.remove(key);
        }
        self.save()
    }

    pub fn mark(&mut self, key: &str, secret: bool) -> Result<bool> {
        if key == META {
            return Err(RESERVED.into());
        }
        if !self.data.contains_key(key) {
            return Err(format!("key not found: {key}"));
        }
        if self.secret(key) == secret {
            return Ok(false);
        }
        if secret {
            self.secrets.insert(key.into());
        } else {
            self.secrets.remove(key);
        }
        self.save()?;
        Ok(true)
    }

    pub fn delete(&mut self, key: &str) -> Result<()> {
        if key == META {
            return Err(RESERVED.into());
        }
        if self.data.remove(key).is_none() {
            return Err(format!("key not found: {key}"));
        }
        self.secrets.remove(key);
        self.save()
    }

    pub fn search(&self, term: &str) -> Vec<String> {
        let needle = lower(term);
        let mut hits = vec![];
        for (key, v) in &self.data {
            let score = match_key(key, &needle).or_else(|| {
                let text = if json::kind(v.get()) == "string" {
                    json::unquote(v.get())
                } else {
                    json::compact(v.get())
                };
                if !self.secret(key) && contains(&lower(&text), &needle) {
                    Some((5, 0))
                } else {
                    None
                }
            });
            if let Some((tier, score)) = score {
                hits.push((tier, score, key.clone()));
            }
        }
        hits.sort();
        hits.into_iter().map(|(_, _, k)| k).collect()
    }
}

fn lower(s: &str) -> Vec<char> {
    s.chars()
        .map(|c| c.to_lowercase().next().unwrap())
        .collect()
}
fn prefix(s: &[char], i: usize, n: &[char]) -> bool {
    s.get(i..i + n.len()) == Some(n)
}
fn contains(s: &[char], n: &[char]) -> bool {
    (0..=s.len()).any(|i| prefix(s, i, n))
}
fn match_key(key: &str, n: &[char]) -> Option<(usize, usize)> {
    let original: Vec<char> = key.chars().collect();
    let s = lower(key);
    if s == n {
        return Some((0, 0));
    }
    if prefix(&s, 0, n) {
        return Some((1, 0));
    }
    let mut substring = None;
    for i in 1..s.len() {
        if !prefix(&s, i, n) {
            continue;
        }
        if ".-_/: @#".contains(original[i - 1])
            || (original[i - 1].is_lowercase() && original[i].is_uppercase())
        {
            return Some((2, i));
        }
        substring.get_or_insert(i);
    }
    if let Some(i) = substring {
        return Some((3, i));
    }
    if n.is_empty() {
        return None;
    }
    let mut best = None;
    for start in 0..s.len() {
        if s[start] != n[0] {
            continue;
        }
        let (mut j, mut end) = (1, start);
        for (i, c) in s.iter().enumerate().skip(start + 1) {
            if j == n.len() {
                break;
            }
            if *c == n[j] {
                j += 1;
                end = i;
            }
        }
        if j < n.len() {
            break;
        }
        let span = end - start + 1;
        best = Some(best.map_or(span, |b: usize| b.min(span)));
    }
    best.map(|b| (4, b))
}

pub fn detect(input: &str, as_string: bool, as_json: bool) -> Result<String> {
    if as_string && as_json {
        return Err("--string and --json are mutually exclusive".into());
    }
    if as_string {
        return Ok(json::quote(input));
    }
    if json::valid(input) {
        return Ok(json::compact(input));
    }
    if as_json {
        return Err("value is not valid JSON".into());
    }
    Ok(json::quote(input))
}

fn json_error(s: &str, e: &str) -> String {
    match json::kind(s) {
        "array" | "string" | "number" | "boolean" if json::valid(s) => format!(
            "json: cannot unmarshal {} into Go value of type map[string]jsontext.Value",
            if json::kind(s) == "boolean" {
                "bool"
            } else {
                json::kind(s)
            }
        ),
        _ => json::validate(s).err().unwrap_or_else(|| e.to_string()),
    }
}
