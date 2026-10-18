use crate::{Result, json};
use jaq_all::{
    jaq_core,
    json::{Num, Rc, Val},
};
use jaq_core::{
    compile::{Compiler, Undefined},
    load::{self, Arena, File, Loader},
};
use std::cell::RefCell;

struct Kind;
struct Data<'a> {
    lut: &'a jaq_core::Lut<Kind>,
    halt: RefCell<Option<String>>,
}
impl jaq_core::DataT for Kind {
    type V<'a> = Val;
    type Data<'a> = &'a Data<'a>;
}
impl<'a> jaq_core::data::HasLut<'a, Kind> for &'a Data<'a> {
    fn lut(&self) -> &'a jaq_core::Lut<Kind> {
        self.lut
    }
}

fn funs() -> impl Iterator<Item = jaq_core::native::Fun<Kind>> {
    use jaq_core::native::{run, v};
    let custom: [jaq_core::native::Filter<jaq_core::RunPtr<Kind>>; 6] = [
        ("nan", v(0), |_| {
            jaq_core::native::bome(Ok(Val::Num(Num::Float(f64::NAN))))
        }),
        ("infinite", v(0), |_| {
            jaq_core::native::bome(Ok(Val::Num(Num::Float(f64::INFINITY))))
        }),
        ("modulemeta", v(0), |cv| {
            let s = cv.1.to_string();
            let message = if json::kind(&s) == "string" {
                format!("cannot load module: {}", json::go_quote(&json::unquote(&s)))
            } else {
                format!("modulemeta cannot be applied to: {}", type_preview(&s))
            };
            jaq_core::native::bome(Err(jaq_core::Error::message(message)))
        }),
        ("halt", v(0), |cv| {
            *cv.0.data().halt.borrow_mut() = Some("halt error: null".into());
            jaq_core::box_iter::box_once(Err(jaq_core::Exn::halt(0)))
        }),
        ("halt_error", v(0), |cv| {
            *cv.0.data().halt.borrow_mut() = Some(halt_message(&cv.1));
            jaq_core::box_iter::box_once(Err(jaq_core::Exn::halt(0)))
        }),
        ("halt_error", v(1), |mut cv| {
            let code = cv.0.pop_var();
            if !matches!(code, Val::Num(_)) {
                return jaq_core::native::bome(Err(jaq_core::Error::message(format!(
                    "halt_error cannot be applied to: {} ({code})",
                    json::kind(&code.to_string())
                ))));
            }
            *cv.0.data().halt.borrow_mut() = Some(halt_message(&cv.1));
            jaq_core::box_iter::box_once(Err(jaq_core::Exn::halt(0)))
        }),
    ];
    jaq_core::funs()
        .chain(jaq_all::jaq_std::funs())
        .chain(jaq_all::json::funs())
        .chain(jaq_all::fmts::funs())
        .filter(|(name, _, _)| !["halt", "debug_empty", "stderr_empty"].contains(name))
        .chain(custom.into_iter().map(run::<Kind>))
}
fn halt_message(v: &Val) -> String {
    let s = encode(v).unwrap_or_else(|_| v.to_string());
    format!(
        "halt error: {}",
        if json::kind(&s) == "string" {
            json::unquote(&s)
        } else {
            s
        }
    )
}

fn compile(expr: &str) -> Result<jaq_core::Filter<Kind>> {
    let extensions = load::parse(include_str!("query_compat.jq"), |p| p.defs())
        .expect("valid compatibility definitions");
    let arena = Arena::default();
    let loader = Loader::new(
        jaq_all::defs()
            .filter(|d| {
                ![
                    "debug",
                    "stderr",
                    "halt",
                    "halt_error",
                    "inputs",
                    "nan",
                    "infinite",
                ]
                .contains(&d.name)
            })
            .chain(extensions.into_iter().filter(|d| d.name != "modulemeta")),
    );
    let modules = loader
        .load(
            &arena,
            File {
                path: (),
                code: expr,
            },
        )
        .map_err(|errors| {
            let (_, error) = errors.into_iter().next().unwrap();
            let token = match error {
                load::Error::Lex(errors) => errors
                    .first()
                    .map(|(_, found)| {
                        found
                            .chars()
                            .next()
                            .map(|c| c.to_string())
                            .unwrap_or_default()
                    })
                    .unwrap_or_default(),
                load::Error::Parse(errors) => errors
                    .first()
                    .map(|(_, found)| found.to_string())
                    .unwrap_or_default(),
                load::Error::Io(errors) => {
                    return errors
                        .into_iter()
                        .next()
                        .map(|(_, e)| e)
                        .unwrap_or_default();
                }
            };
            if token.is_empty() {
                "invalid query: unexpected EOF".into()
            } else {
                format!("invalid query: unexpected token {}", json::go_quote(&token))
            }
        })?;
    load::import(&modules, |path| {
        Err(format!("cannot load module: {}", json::go_quote(path.path)))
    })
    .map_err(|errors| {
        let (_, error) = errors.into_iter().next().unwrap();
        if let load::Error::Io(errors) = error {
            errors
                .into_iter()
                .next()
                .map(|(_, e)| e)
                .unwrap_or_default()
        } else {
            "invalid query".into()
        }
    })?;
    Compiler::default()
        .with_funs(funs())
        .compile(modules)
        .map_err(|errors| {
            let (_, errors) = errors.into_iter().next().unwrap();
            let (name, undefined) = errors.into_iter().next().unwrap();
            match undefined {
                Undefined::Filter(_) if name.starts_with('@') => {
                    format!("format not defined: {name}")
                }
                Undefined::Filter(_) if name == "input" || name == "inputs" => {
                    "input(s)/0 is not allowed".into()
                }
                Undefined::Filter(n) => format!("function not defined: {name}/{n}"),
                Undefined::Var => format!("variable not defined: {name}"),
                _ => format!("{} not defined: {name}", undefined.as_str()),
            }
        })
}

pub fn run(input: &str, expr: &str) -> Result<Vec<String>> {
    let filter = compile(expr)?;
    let value: serde_json::Value =
        serde_json::from_str(&json::repair_surrogates(input)).map_err(|e| e.to_string())?;
    let value = from_input(value)?;
    let mut results = vec![];
    let data = Data {
        lut: &filter.lut,
        halt: RefCell::new(None),
    };
    let ctx = jaq_core::Ctx::<Kind>::new(&data, Default::default());
    for v in filter.id.run((ctx, value)) {
        let v = v.map_err(|e| match e.get_err() {
            Ok(e) => runtime_error(&e.to_string()),
            Err(_) => data
                .halt
                .borrow()
                .clone()
                .unwrap_or_else(|| "halt error: null".into()),
        })?;
        // Go's encoder sorts object keys and escapes HTML. Bare store values
        // retain their original order; query results use canonical encoding.
        results.push(encode(&v)?);
    }
    Ok(results)
}

fn from_input(v: serde_json::Value) -> Result<Val> {
    use serde_json::Value as V;
    Ok(match v {
        V::Object(map) => Val::Obj(Rc::new(
            map.into_iter()
                .map(|(k, v)| Ok((Val::from(k), from_input(v)?)))
                .collect::<Result<_>>()?,
        )),
        V::Array(a) => Val::Arr(Rc::new(
            a.into_iter().map(from_input).collect::<Result<_>>()?,
        )),
        V::Number(n) => {
            let s = n.to_string();
            let f = s.parse::<f64>().map_err(|e| e.to_string())?;
            if !f.is_finite() {
                return Err(format!(
                    "json: cannot unmarshal number {s} into Go value of type float64"
                ));
            }
            Val::Num(Num::Float(f))
        }
        V::Null => Val::Null,
        V::Bool(b) => Val::Bool(b),
        V::String(s) => Val::from(s),
    })
}

fn encode(v: &Val) -> Result<String> {
    match v {
        Val::Num(Num::Float(f)) => {
            if !f.is_finite() {
                return Err(format!(
                    "json: unsupported value: {}",
                    if f.is_nan() {
                        "NaN"
                    } else if *f > 0.0 {
                        "+Inf"
                    } else {
                        "-Inf"
                    }
                ));
            }
            Ok(float(*f))
        }
        Val::Obj(map) => {
            let mut entries: Vec<_> = map
                .iter()
                .map(|(k, v)| Ok((json::unquote(&k.to_string()), encode(v)?)))
                .collect::<Result<_>>()?;
            entries.sort_by(|(a, _), (b, _)| a.cmp(b));
            Ok(format!(
                "{{{}}}",
                entries
                    .into_iter()
                    .map(|(k, v)| format!("{}:{v}", json::quote(&k)))
                    .collect::<Vec<_>>()
                    .join(",")
            ))
        }
        Val::Arr(a) => Ok(format!(
            "[{}]",
            a.iter().map(encode).collect::<Result<Vec<_>>>()?.join(",")
        )),
        _ => Ok(json::escape_html(&v.to_string())),
    }
}

fn float(f: f64) -> String {
    if f == 0.0 {
        return if f.is_sign_negative() {
            "-0".into()
        } else {
            "0".into()
        };
    }
    if f.abs() >= 1e-6 && f.abs() < 1e21 {
        f.to_string()
    } else {
        let s = format!("{f:e}");
        let (base, exp) = s.split_once('e').unwrap();
        let e = exp.parse::<i32>().unwrap();
        format!("{base}e{}{e}", if e >= 0 { "+" } else { "" })
    }
}

fn runtime_error(error: &str) -> String {
    if let Some(v) = error
        .strip_prefix("cannot use ")
        .and_then(|v| v.strip_suffix(" as iterable (array or object)"))
    {
        return if v == "null" {
            "cannot iterate over: null".into()
        } else {
            format!("cannot iterate over: {}", type_preview(v))
        };
    }
    if let Ok(message) = serde_json::from_str::<String>(error) {
        return if message.starts_with("format not defined:") {
            message
        } else {
            format!("error: {message}")
        };
    }
    if json::valid(error) {
        return format!("error: {error}");
    }
    if let Some((v, index)) = error
        .strip_prefix("cannot index ")
        .and_then(|s| s.split_once(" with "))
        && json::kind(index) == "string"
    {
        return format!("expected an object but got: {}", type_preview(v));
    }
    error.into()
}

fn type_preview(s: &str) -> String {
    if s == "null" {
        return "null".into();
    }
    let mut preview = s.to_owned();
    if preview.len() > 30 {
        let trailing = match json::kind(s) {
            "string" => " ...\"",
            "array" => " ...]",
            "object" => " ...}",
            _ => " ...",
        };
        let mut end = 30 - trailing.len();
        while !preview.is_char_boundary(end) {
            end -= 1;
        }
        preview.truncate(end);
        preview.push_str(trailing);
    }
    format!("{} ({preview})", json::kind(s))
}
