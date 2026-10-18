//! JSON values remain raw: number spelling, object order and duplicate keys
//! survive reads and writes just as they do with Go's json.RawMessage.
use serde_json::value::RawValue;

pub fn valid(s: &str) -> bool {
    serde_json::from_str::<Box<RawValue>>(s).is_ok()
}

pub fn compact(s: &str) -> String {
    let mut out = String::new();
    let (mut quoted, mut escaped) = (false, false);
    for c in s.chars() {
        if quoted {
            out.push(c);
            if escaped {
                escaped = false;
            } else if c == '\\' {
                escaped = true;
            } else if c == '"' {
                quoted = false;
            }
        } else if c == '"' {
            quoted = true;
            out.push(c);
        } else if !matches!(c, ' ' | '\t' | '\r' | '\n') {
            out.push(c);
        }
    }
    out
}

pub fn pretty(s: &str) -> String {
    let s = compact(s);
    let chars: Vec<char> = s.chars().collect();
    let mut out = String::new();
    let (mut quoted, mut escaped, mut depth) = (false, false, 0usize);
    for (i, &c) in chars.iter().enumerate() {
        if quoted {
            out.push(c);
            if escaped {
                escaped = false;
            } else if c == '\\' {
                escaped = true;
            } else if c == '"' {
                quoted = false;
            }
            continue;
        }
        match c {
            '"' => {
                quoted = true;
                out.push(c);
            }
            '{' | '[' => {
                out.push(c);
                depth += 1;
                if !matches!(chars.get(i + 1), Some('}' | ']')) {
                    out.push('\n');
                    out.push_str(&"  ".repeat(depth));
                }
            }
            '}' | ']' => {
                depth = depth.saturating_sub(1);
                if i > 0 && !matches!(chars[i - 1], '{' | '[') {
                    out.push('\n');
                    out.push_str(&"  ".repeat(depth));
                }
                out.push(c);
            }
            ',' => {
                out.push_str(",\n");
                out.push_str(&"  ".repeat(depth));
            }
            ':' => out.push_str(": "),
            _ => out.push(c),
        }
    }
    out
}

pub fn escape_html(s: &str) -> String {
    s.replace('&', "\\u0026")
        .replace('<', "\\u003c")
        .replace('>', "\\u003e")
        .replace('\u{2028}', "\\u2028")
        .replace('\u{2029}', "\\u2029")
}

pub fn quote(s: &str) -> String {
    escape_html(&serde_json::to_string(s).expect("strings serialize"))
}

pub fn object(s: &str) -> crate::Result<std::collections::BTreeMap<String, Box<RawValue>>> {
    validate(s)?;
    let s = s.trim();
    if !s.starts_with('{') {
        return Err("expected object".into());
    }
    let b = s.as_bytes();
    let mut map = std::collections::BTreeMap::new();
    let mut i = 1;
    fn space(b: &[u8], i: &mut usize) {
        while b.get(*i).is_some_and(|c| b" \t\r\n".contains(c)) {
            *i += 1;
        }
    }
    fn string_end(b: &[u8], mut i: usize) -> usize {
        i += 1;
        while i < b.len() {
            match b[i] {
                b'\\' => i += 2,
                b'"' => return i + 1,
                _ => i += 1,
            }
        }
        i
    }
    loop {
        space(b, &mut i);
        if b[i] == b'}' {
            break;
        }
        let start = i;
        i = string_end(b, i);
        let key = unquote(&s[start..i]);
        space(b, &mut i);
        i += 1;
        space(b, &mut i);
        let start = i;
        let mut depth = 0;
        while i < b.len() {
            match b[i] {
                b'"' => i = string_end(b, i),
                b'{' | b'[' => {
                    depth += 1;
                    i += 1;
                }
                b'}' | b']' if depth > 0 => {
                    depth -= 1;
                    i += 1;
                }
                b',' | b'}' if depth == 0 => break,
                _ => i += 1,
            }
        }
        let raw =
            RawValue::from_string(s[start..i].trim_end().to_owned()).map_err(|e| e.to_string())?;
        map.insert(key, raw);
        if b[i] == b'}' {
            break;
        }
        i += 1;
    }
    Ok(map)
}

pub fn kind(s: &str) -> &'static str {
    match s.trim_start().as_bytes().first() {
        Some(b'{') => "object",
        Some(b'[') => "array",
        Some(b'"') => "string",
        Some(b't' | b'f') => "boolean",
        Some(b'n') | None => "null",
        _ => "number",
    }
}

pub fn unquote(s: &str) -> String {
    serde_json::from_str::<String>(s)
        .or_else(|_| serde_json::from_str(&repair_surrogates(s)))
        .unwrap_or_else(|_| s.to_owned())
}

pub fn repair_surrogates(s: &str) -> String {
    let b = s.as_bytes();
    let mut out = String::new();
    let mut start = 0;
    let mut i = 0;
    while i < b.len() {
        if b[i] != b'\\' {
            i += 1;
            continue;
        }
        if b.get(i + 1) == Some(&b'u')
            && i + 6 <= b.len()
            && let Ok(n) = u16::from_str_radix(&s[i + 2..i + 6], 16)
        {
            if (0xd800..=0xdbff).contains(&n)
                && b.get(i + 6..i + 8) == Some(b"\\u")
                && i + 12 <= b.len()
                && u16::from_str_radix(&s[i + 8..i + 12], 16)
                    .is_ok_and(|v| (0xdc00..=0xdfff).contains(&v))
            {
                i += 12;
                continue;
            }
            if (0xd800..=0xdfff).contains(&n) {
                out.push_str(&s[start..i]);
                out.push_str("\\ufffd");
                i += 6;
                start = i;
                continue;
            }
        }
        i = (i + 2).min(b.len());
    }
    out.push_str(&s[start..]);
    out
}

pub fn preview(s: &str) -> Vec<u8> {
    let s = compact(s);
    if s.len() > 80 {
        [s.as_bytes()[..77].to_vec(), b"...".to_vec()].concat()
    } else {
        s.into_bytes()
    }
}

pub fn go_quote(s: &str) -> String {
    let mut out = String::from("\"");
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            '\u{7}' => out.push_str("\\a"),
            '\u{8}' => out.push_str("\\b"),
            '\u{b}' => out.push_str("\\v"),
            '\u{c}' => out.push_str("\\f"),
            c if c.is_control() && (c as u32) < 256 => {
                out.push_str(&format!("\\x{:02x}", c as u32))
            }
            c if c.is_control() || matches!(c, '\u{2028}' | '\u{2029}') => {
                out.push_str(&format!("\\u{:04x}", c as u32))
            }
            _ => out.push(c),
        }
    }
    out.push('"');
    out
}

/// Keep legacy diagnostics for corrupt stores independently of serde's wording.
pub fn validate(s: &str) -> crate::Result<()> {
    struct Scanner<'a> {
        b: &'a [u8],
        i: usize,
        depth: usize,
    }
    impl Scanner<'_> {
        fn space(&mut self) {
            while self.b.get(self.i).is_some_and(|b| b" \t\r\n".contains(b)) {
                self.i += 1;
            }
        }
        fn bad(&self, context: &str) -> String {
            if let Some(&b) = self.b.get(self.i) {
                let c = match b {
                    b'\n' => "\\n".into(),
                    b'\r' => "\\r".into(),
                    b'\t' => "\\t".into(),
                    b'\\' => "\\\\".into(),
                    b'\'' => "\\'".into(),
                    b if !(32..127).contains(&b) => format!("\\x{b:02x}"),
                    b => (b as char).to_string(),
                };
                format!("invalid character '{c}' {context}")
            } else {
                "unexpected end of JSON input".into()
            }
        }
        fn string(&mut self) -> crate::Result<()> {
            self.i += 1;
            loop {
                match self.b.get(self.i).copied() {
                    Some(b'"') => {
                        self.i += 1;
                        return Ok(());
                    }
                    Some(b'\\') => {
                        self.i += 1;
                        match self.b.get(self.i).copied() {
                            Some(b'u') => {
                                for _ in 0..4 {
                                    self.i += 1;
                                    if !self.b.get(self.i).is_some_and(u8::is_ascii_hexdigit) {
                                        return Err(self.bad("in \\u hexadecimal character escape"));
                                    }
                                }
                                self.i += 1;
                            }
                            Some(b'"' | b'\\' | b'/' | b'b' | b'f' | b'n' | b'r' | b't') => {
                                self.i += 1
                            }
                            Some(c) => {
                                return Err(format!(
                                    "invalid escape sequence `\\{}` in string",
                                    c as char
                                ));
                            }
                            None => return Err("unexpected end of JSON input".into()),
                        }
                    }
                    Some(0..=31) => return Err(self.bad("in string literal")),
                    Some(_) => self.i += 1,
                    None => return Err(self.bad("in string literal")),
                }
            }
        }
        fn number(&mut self) -> crate::Result<()> {
            if self.b.get(self.i) == Some(&b'-') {
                self.i += 1;
            }
            match self.b.get(self.i) {
                Some(b'0') => self.i += 1,
                Some(b'1'..=b'9') => {
                    while self.b.get(self.i).is_some_and(u8::is_ascii_digit) {
                        self.i += 1;
                    }
                }
                _ => return Err(self.bad("in numeric literal")),
            }
            if self.b.get(self.i) == Some(&b'.') {
                self.i += 1;
                if !self.b.get(self.i).is_some_and(u8::is_ascii_digit) {
                    return Err(self.bad("in numeric literal"));
                }
                while self.b.get(self.i).is_some_and(u8::is_ascii_digit) {
                    self.i += 1;
                }
            }
            if matches!(self.b.get(self.i), Some(b'e' | b'E')) {
                self.i += 1;
                if matches!(self.b.get(self.i), Some(b'+' | b'-')) {
                    self.i += 1;
                }
                if !self.b.get(self.i).is_some_and(u8::is_ascii_digit) {
                    return Err(self.bad("in numeric literal"));
                }
                while self.b.get(self.i).is_some_and(u8::is_ascii_digit) {
                    self.i += 1;
                }
            }
            Ok(())
        }
        fn value(&mut self) -> crate::Result<()> {
            self.space();
            match self.b.get(self.i).copied() {
                Some(b'"') => self.string(),
                Some(b'-' | b'0'..=b'9') => self.number(),
                Some(b't' | b'f' | b'n') => {
                    let token = match self.b[self.i] {
                        b't' => "true",
                        b'f' => "false",
                        _ => "null",
                    };
                    for b in token.bytes() {
                        if self.b.get(self.i) != Some(&b) {
                            return Err(self
                                .bad(&format!("in literal {token} (expecting '{}')", b as char)));
                        }
                        self.i += 1;
                    }
                    Ok(())
                }
                Some(open @ (b'{' | b'[')) => {
                    self.depth += 1;
                    if self.depth > 10000 {
                        return Err(self.bad("exceeded max depth"));
                    }
                    self.i += 1;
                    self.space();
                    let close = if open == b'{' { b'}' } else { b']' };
                    if self.b.get(self.i) == Some(&close) {
                        self.i += 1;
                        self.depth -= 1;
                        return Ok(());
                    }
                    loop {
                        if open == b'{' {
                            if self.b.get(self.i) != Some(&b'"') {
                                return Err(self.bad("looking for beginning of object key string"));
                            }
                            self.string()?;
                            self.space();
                            if self.b.get(self.i) != Some(&b':') {
                                return Err(self.bad("after object key"));
                            }
                            self.i += 1;
                        }
                        self.value()?;
                        self.space();
                        if self.b.get(self.i) == Some(&close) {
                            self.i += 1;
                            self.depth -= 1;
                            return Ok(());
                        }
                        if self.b.get(self.i) != Some(&b',') {
                            return Err(self.bad(if open == b'{' {
                                "after object key:value pair"
                            } else {
                                "after array element"
                            }));
                        }
                        self.i += 1;
                        self.space();
                    }
                }
                _ => Err(self.bad("looking for beginning of value")),
            }
        }
    }
    let mut scanner = Scanner {
        b: s.as_bytes(),
        i: 0,
        depth: 0,
    };
    scanner.value()?;
    scanner.space();
    if scanner.i < s.len() {
        Err(scanner.bad("after top-level value"))
    } else {
        Ok(())
    }
}
