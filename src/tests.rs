use crate::{clip, config, external, json, scaffold, store};
use std::{fs, os::unix::fs::PermissionsExt, path::Path};

fn open_store(dir: &Path) -> store::Store {
    store::Store::open(dir.join("store.json").to_str().unwrap()).unwrap()
}

fn saved_store(dir: &Path) -> serde_json::Value {
    serde_json::from_slice(&fs::read(dir.join("store.json")).unwrap()).unwrap()
}

#[test]
fn store_round_trips_raw_values_and_cleans_up_temporary_files() {
    let dir = tempfile::tempdir().unwrap();
    let mut st = open_store(dir.path());
    let raw = r#"{"cpu":2,"large":9007199254740993,"duplicate":1,"duplicate":2}"#;
    st.set("limits", raw.into(), false).unwrap();
    let path = dir.path().join("store.json");
    assert_eq!(
        fs::metadata(&path).unwrap().permissions().mode() & 0o777,
        0o600
    );
    let mut st = open_store(dir.path());
    assert_eq!(json::compact(st.get("limits").unwrap()), raw);
    st.set("limits", "2".into(), false).unwrap();
    assert_eq!(open_store(dir.path()).get("limits"), Some("2"));
    st.delete("limits").unwrap();
    assert!(st.delete("limits").is_err());
    assert!(open_store(dir.path()).keys().is_empty());
    assert_eq!(fs::read_dir(dir.path()).unwrap().count(), 1);
}

#[test]
fn secret_marks_persist_without_becoming_user_values() {
    let dir = tempfile::tempdir().unwrap();
    let mut st = open_store(dir.path());
    st.set("plain", "1".into(), false).unwrap();
    assert!(saved_store(dir.path()).get(store::META).is_none());
    st.set("token", r#""shh""#.into(), true).unwrap();
    let mut st = open_store(dir.path());
    assert!(st.secret("token"));
    assert!(!st.secret("plain"));
    assert_eq!(st.keys(), ["plain", "token"]);
    assert!(st.get(store::META).is_none());
    assert_eq!(st.get("token"), Some(r#""shh""#));
    assert!(st.mark("plain", true).unwrap());
    assert!(!st.mark("plain", true).unwrap());
    assert!(st.mark("ghost", true).is_err());
    assert_eq!(
        saved_store(dir.path())[store::META]["secrets"],
        serde_json::json!(["plain", "token"])
    );
    st.set("plain", "2".into(), false).unwrap();
    st.delete("token").unwrap();
    assert!(!st.secret("plain"));
    assert!(!st.secret("token"));
    assert!(saved_store(dir.path()).get(store::META).is_none());
}

#[test]
fn unknown_metadata_survives_and_stale_secret_marks_do_not() {
    let dir = tempfile::tempdir().unwrap();
    fs::write(
        dir.path().join("store.json"),
        r#"{"$leo":{"future":true,"secrets":["a","gone"]},"a":1}"#,
    )
    .unwrap();
    let mut st = open_store(dir.path());
    assert!(st.secret("a"));
    assert!(!st.secret("gone"));
    st.set("gone", "2".into(), false).unwrap();
    assert!(!st.secret("gone"));
    st.delete("a").unwrap();
    assert_eq!(
        saved_store(dir.path())[store::META],
        serde_json::json!({"future": true})
    );
    for bad in [
        r#"{"$leo":"x","a":1}"#,
        r#"{"$leo":null,"a":1}"#,
        r#"{"$leo":{"secrets":"a"},"a":1}"#,
    ] {
        let path = dir.path().join("store.json");
        fs::write(&path, bad).unwrap();
        assert!(store::Store::open(path.to_str().unwrap()).is_err());
        assert_eq!(fs::read_to_string(path).unwrap(), bad);
    }
}

#[test]
fn reserved_metadata_cannot_be_modified_through_value_operations() {
    let dir = tempfile::tempdir().unwrap();
    let mut st = open_store(dir.path());
    st.set("a", "1".into(), false).unwrap();
    let before = fs::read(dir.path().join("store.json")).unwrap();
    assert_eq!(
        st.set(store::META, "1".into(), false).unwrap_err(),
        store::RESERVED
    );
    assert_eq!(st.mark(store::META, true).unwrap_err(), store::RESERVED);
    assert_eq!(st.delete(store::META).unwrap_err(), store::RESERVED);
    assert_eq!(fs::read(dir.path().join("store.json")).unwrap(), before);
}

#[test]
fn search_orders_match_tiers_and_hides_secret_values() {
    let dir = tempfile::tempdir().unwrap();
    let mut st = open_store(dir.path());
    for (key, value) in [
        ("api", "1"),
        ("api.token", r#""abc""#),
        ("apiary", "2"),
        ("github.api", "3"),
        ("rapid", "4"),
        ("awsPrimaryKey", "5"),
        ("deploy.host", r#""example.com""#),
        ("notes", r#""call the api team""#),
        ("hidden", r#""api secret""#),
        ("unrelated", "6"),
        ("a_long_p_i_key", "7"),
    ] {
        st.set(key, value.into(), key == "hidden").unwrap();
    }
    assert_eq!(
        st.search("API"),
        [
            "api",
            "api.token",
            "apiary",
            "github.api",
            "rapid",
            "awsPrimaryKey",
            "a_long_p_i_key",
            "notes"
        ]
    );
    assert_eq!(st.search("dh"), ["deploy.host"]);
    assert_eq!(st.search("key")[0], "awsPrimaryKey");
    assert_eq!(st.search("example"), ["deploy.host"]);
    assert_eq!(st.search("hidden"), ["hidden"]);
    assert!(st.search("secret").is_empty());
    assert!(st.search("zzz").is_empty());
}

#[test]
fn json_types_and_unquoting_preserve_raw_data() {
    for (raw, kind) in [
        ("{}", "object"),
        ("[]", "array"),
        (r#""hi""#, "string"),
        ("42", "number"),
        ("-1.5", "number"),
        ("true", "boolean"),
        ("false", "boolean"),
        ("null", "null"),
        ("", "null"),
        ("  {\"a\":1}", "object"),
    ] {
        assert_eq!(json::kind(raw), kind);
    }
    assert_eq!(json::unquote(r#""hello""#), "hello");
    assert_eq!(json::unquote("9007199254740993"), "9007199254740993");
    assert_eq!(
        json::compact(r#"{ "z": 1.00, "z": 1e20, "s": "a b" }"#),
        r#"{"z":1.00,"z":1e20,"s":"a b"}"#
    );
}

#[test]
fn external_command_names_and_comment_markers_match_the_contract() {
    for (file, expected) in [
        ("leo-hello", Some("hello")),
        ("leo-deploy.sh", Some("deploy")),
        ("leo-build.mts", Some("build")),
        ("leo-tool.py", Some("tool")),
        ("leo-app.js", Some("app")),
        ("leo-", None),
        ("leo-notes.txt", None),
        ("notleo", None),
        ("leo", None),
    ] {
        assert_eq!(external::name(file).as_deref(), expected);
    }
    let dir = tempfile::tempdir().unwrap();
    for (name, body, expected) in [
        ("bash", "#!/bin/sh\n# leo: greet someone\n", "greet someone"),
        (
            "node",
            "#!/usr/bin/env node\n// leo: build the thing\n",
            "build the thing",
        ),
        ("none", "#!/bin/sh\necho hi\n", ""),
    ] {
        let path = dir.path().join(name);
        fs::write(&path, body).unwrap();
        assert_eq!(external::marker(path.to_str().unwrap()), expected);
    }
}

#[test]
fn generated_templates_are_executable_and_overwrites_require_force() {
    let dir = tempfile::tempdir().unwrap();
    for (lang, filename, shebang) in [
        ("bash", "leo-x", "#!/usr/bin/env bash"),
        ("zsh", "leo-x", "#!/usr/bin/env zsh"),
        ("python", "leo-x", "#!/usr/bin/env python3"),
        ("node", "leo-x", "#!/usr/bin/env node"),
        ("typescript", "leo-x.mts", "#!/usr/bin/env node"),
    ] {
        let workspace = dir.path().join(lang);
        let path = scaffold::generate(workspace.to_str().unwrap(), "x", lang, false).unwrap();
        assert_eq!(Path::new(&path).file_name().unwrap(), filename);
        let body = fs::read_to_string(&path).unwrap();
        assert!(body.starts_with(shebang));
        assert!(body.contains("leo:"));
        assert!(body.contains("leo x"));
        assert_eq!(
            fs::metadata(&path).unwrap().permissions().mode() & 0o777,
            0o755
        );
        assert!(scaffold::generate(workspace.to_str().unwrap(), "x", lang, false).is_err());
        assert_eq!(fs::read_to_string(&path).unwrap(), body);
        scaffold::generate(workspace.to_str().unwrap(), "x", lang, true).unwrap();
    }
    for name in ["", "bad name", "has/slash", "-leading"] {
        assert!(scaffold::generate(dir.path().to_str().unwrap(), name, "bash", false).is_err());
    }
}

#[test]
fn language_aliases_and_workspace_selection_are_stable() {
    for (alias, lang) in [
        ("sh", "bash"),
        ("bash", "bash"),
        ("zsh", "zsh"),
        ("py", "python"),
        ("python", "python"),
        ("js", "node"),
        ("javascript", "node"),
        ("node", "node"),
        ("ts", "typescript"),
        ("typescript", "typescript"),
    ] {
        assert_eq!(scaffold::normalize(alias).unwrap(), lang);
    }
    assert!(scaffold::normalize("cobol").is_err());
    let mut cfg = config::Config {
        base_dir: String::new(),
        config_path: String::new(),
        store_path: String::new(),
        gen_lang: "bash".into(),
        workspaces: vec![],
    };
    assert!(scaffold::choose(&cfg, "").is_err());
    cfg.workspaces = vec![
        config::Workspace {
            name: "personal".into(),
            path: "/personal".into(),
        },
        config::Workspace {
            name: "work".into(),
            path: "/work".into(),
        },
    ];
    assert_eq!(scaffold::choose(&cfg, "").unwrap().path, "/personal");
    assert_eq!(scaffold::choose(&cfg, "work").unwrap().path, "/work");
    assert!(scaffold::choose(&cfg, "missing").is_err());
}

#[test]
fn config_serialization_preserves_typed_values_and_workspace_order() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("config.toml");
    let cfg = config::FileConfig {
        store_path: "~/store.json".into(),
        gen_lang: "python".into(),
        workspaces: vec![
            config::Workspace {
                name: "work".into(),
                path: "$WORK/commands".into(),
            },
            config::Workspace {
                name: "personal".into(),
                path: "~/commands".into(),
            },
        ],
    };
    config::write_file(path.to_str().unwrap(), &cfg).unwrap();
    let loaded = config::read_file(path.to_str().unwrap()).unwrap();
    assert_eq!(loaded.store_path, cfg.store_path);
    assert_eq!(loaded.gen_lang, cfg.gen_lang);
    assert_eq!(loaded.workspaces.len(), 2);
    for (got, want) in loaded.workspaces.iter().zip(&cfg.workspaces) {
        assert_eq!(got.name, want.name);
        assert_eq!(got.path, want.path);
    }
}

#[test]
fn clipboard_resolution_preserves_values_and_recognizes_existing_files() {
    let dir = tempfile::tempdir().unwrap();
    let image = dir.path().join("logo.png");
    fs::write(&image, b"image").unwrap();
    let image = image.to_str().unwrap();
    let mut st = open_store(dir.path());
    st.set("greeting", r#""hello there""#.into(), false)
        .unwrap();
    st.set("limits", r#"{"cpu":2}"#.into(), false).unwrap();
    st.set("logo", json::quote(image), false).unwrap();
    for (arg, text, file, resolved) in [
        ("greeting", "hello there", false, true),
        ("limits", r#"{"cpu":2}"#, false, true),
        ("logo", image, true, true),
        (image, image, true, true),
        ("nosuchkey", "nosuchkey", false, false),
    ] {
        assert_eq!(
            clip::resolve(&st, arg, false),
            (text.into(), file, resolved)
        );
    }
    for (path, expected) in [
        ("/x/a.png", "public.png"),
        ("/x/a.PNG", "public.png"),
        ("/x/a.jpg", "public.jpeg"),
        ("/x/a.jpeg", "public.jpeg"),
        ("/x/a.gif", "com.compuserve.gif"),
        ("/x/a.txt", ""),
        ("/x/noext", ""),
    ] {
        assert_eq!(clip::image_type(path), expected);
    }
    assert!(clip::copy_files(&[]).is_err());
    assert!(clip::copy_files(&[dir.path().join("missing").to_string_lossy().into()]).is_err());
}
