#!/usr/bin/env python3
"""Check observable behavior against recorded expectations, in disposable homes.

Every step compares exit status, stdout, stderr, file bytes and permissions.
Expected results were captured from the pre-port implementation; no second
implementation or compiler is needed to run this suite.
"""
import argparse
import dataclasses
import hashlib
import json
import os
from pathlib import Path
import pty
import shutil
import signal
import select
import stat
import subprocess
import sys
import tempfile


@dataclasses.dataclass
class Step:
    args: list
    stdin: bytes = b""
    tty: bool = False
    interrupt: bool = False
    stdout_closed: bool = False
    stderr_closed: bool = False


@dataclasses.dataclass
class Case:
    name: str
    steps: list
    files: dict = dataclasses.field(default_factory=dict)
    env: dict = dataclasses.field(default_factory=dict)
    modes: dict = dataclasses.field(default_factory=dict)
    links: dict = dataclasses.field(default_factory=dict)
    umask: int = -1


def step(*args, stdin=b"", tty=False, interrupt=False, stdout_closed=False, stderr_closed=False):
    return Step(list(args), stdin, tty, interrupt, stdout_closed, stderr_closed)


STORE = "xdg/leo/store.json"
CONFIG = "xdg/leo/config.toml"
COMMANDS = "xdg/leo/commands"
CASES = []


def add(name, steps, **kwargs):
    CASES.append(Case(name, steps, **kwargs))


for command in [[], ["store"], ["kv"], ["clip"], ["generate"], ["gen"], ["new"],
                ["config"], ["config", "path"], ["config", "show"], ["config", "setup"],
                ["version"], ["help"]]:
    for ending in [["--help"], ["-h"], ["--help=true"], ["-h=1"]]:
        add("help " + repr(command + ending), [step(*(command + ending))])
    if command:
        add("help topic " + repr(command), [step("help", *command)])
for args in [[], ["help"], ["generate"], ["gen"], ["new"], ["config"], ["config", "unknown"],
             ["version"], ["--version"], ["-v"], ["--version=false"], ["--", "kv"],
             ["--help", "kv"], ["-h", "kv"], ["--help=true", "kv"],
             ["help", "unknown"], ["help", "config", "unknown"], ["help", "kv", "foo"],
             ["wat"], ["confi"], ["versio"], ["stor"], ["GEN"], [""],
             ["completion"], ["--bad"], ["--bad", "wat"], ["version", "wat"],
             ["config", "path", "extra"], ["config", "show", "-v"],
             ["kv", "--bad", "--help"], ["kv", "--version"], ["kv", "-xyz"],
             ["kv", "--query"], ["kv", "--file"], ["kv", "-h=false"],
             ["kv", "--secret=nah"], ["kv", "-f=nah"], ["kv", "-rc=nah"],
             ["kv", "a", "b", "c"], ["generate", "a", "b"], ["clip"],
             ["clip", "--file", "--text", "x"]]:
    add("grammar " + repr(args), [step(*args)])

values = ["https://giphy.com/x.gif", "1.2.3", "07001", "+15551234567", "hello world", "42",
          "3.14", "true", "false", "null", '{"cpu":2,"mem":"4Gi"}', "[1,2,3]", '"42"',
          "", "-", "-5", "1e+20", "-0", "1.00", "9007199254740993", "1e400",
          '{ "z": [1, {"b": [], "a": {}}], "a": 2, "z": 3 }',
          "<hello>&\u2028\u2029", "apostrophe' quote\" backslash\\", "☺🐱日本語", '"\\ud800"']
for value in values:
    add("value " + repr(value), [step("kv", "key", value), step("store", "key"),
        step("kv", "-c", "key"), step("kv"), step("kv", "key", value), step("kv", "-d", "key")])
for flag in ["--string", "--json", "--string=false", "--json=false", "--secret", "--secret=false"]:
    for value in ["42", "false", "not JSON", '{"b":1,"a":2}', '"x"']:
        add("typed " + flag + value, [step("kv", flag, "key", value), step("kv", "key"), step("kv")])
add("input sources", [step("kv", "--string", "input", "-", stdin=b"line one\nline two\n\n"),
    step("kv", "input"), step("kv", "--file", "input.txt", "file"), step("kv", "file"),
    step("kv", "--file=input.txt", "file", "ignored"), step("kv", "--file", "missing", "x")],
    files={"input.txt": "contents\n\n"})
add("flag combinations", [step("kv", "--string", "--json", "x", "1"),
    step("kv", "--", "temperature", "-5"), step("kv", "temperature"),
    step("kv", "--string", "--", "option", "--verbose"), step("kv", "option"),
    step("kv", "--query=type", "option"), step("kv", "--file=", "x", "a")])
add("closed stdout",[step("kv","x",stdout_closed=True)],files={STORE:'{"x":"hello"}'})
add("closed stderr",[step("kv","missing",stderr_closed=True)])
add("external closed stdout",[step("emit",stdout_closed=True)],files={COMMANDS+"/leo-emit":"#!/bin/sh\nprintf hello\n"},modes={COMMANDS+"/leo-emit":0o755})
add("overwrite noninteractive", [step("kv", "foo", "bar"), step("kv", "foo", "baz"),
    step("kv", "foo"), step("kv", "-f", "foo", "baz"), step("kv", "foo", "baz"),
    step("kv", "foo", "-", stdin=b"new\n"), step("kv", "-f", "foo", "-", stdin=b"new\n")])
for answer in [b"y\n", b"yes\n", b"Y\n", b"no\n", b"\n"]:
    add("overwrite terminal " + repr(answer), [step("kv", "foo", "bar"),
        step("kv", "foo", "baz", stdin=answer, tty=True), step("kv", "foo")])
add("secret lifecycle", [step("kv", "plain", "hi"), step("kv", "--secret", "token", "hunter2"),
    step("kv"), step("kv", "token"), step("kv", "--query", ".[]"),
    step("kv", "token", "swordfish", tty=True, stdin=b"y\n"), step("kv"),
    step("kv", "--secret", "token"), step("kv", "--secret=false", "token"),
    step("kv", "--secret=false", "token"), step("kv", "--secret", "token", "swordfish"),
    step("kv", "--secret=false", "-f", "token", "new"), step("kv", "--secret", "ghost"),
    step("kv", "-d", "token"), step("kv", "-d", "plain"), step("kv")])
for key in ["help", "get", "set", "list", "$leo", "foo bar", "a\nkey", "☺"]:
    add("literal key " + repr(key), [step("kv", key, "1"), step("kv", key),
        step("kv", "--secret", key), step("kv", "-d", key)])
for args in [["-d"], ["-s"], ["-d", "a", "b"], ["-s", "a", "b"], ["-s", "-d", "a"]]:
    add("invalid mode " + repr(args), [step("kv", *args)])
for flag in ["--string", "--json", "--file=x", "--force", "--secret", "--query=.", "--raw", "--compact"]:
    for args in [[], ["x"], ["-d", "x"], ["-s", "x"], ["x", "1"]]:
        add("flag applicability " + repr([flag] + args), [step("kv", flag, *args)])
for content in ["", " \n\t", "null", "{}", '{"a":1,"$leo":{"secrets":["a","gone"],"future":true}}',
                '{"a":1,"$leo":{"secrets":null}}', '{"a":1,"$leo":{}}',
                '{"":2,"$leo":{"secrets":[null]}}', '{"a":1,"$leo":"x"}',
                '{"a":1,"$leo":null}', '{"a":1,"$leo":[]}',
                '{"a":1,"$leo":{"secrets":"a"}}', '{"a":1,"$leo":{"secrets":[1]}}',
                "[]", "true", "42", '"x"', "{", '{"a":}', "broken"]:
    add("existing store " + repr(content), [step("kv"), step("kv", "gone", "2"), step("kv", "-d", "a")],
        files={STORE: content})
search_data = {"api": 1, "api.token": "abc", "apiary": 2, "github.api": 3, "rapid": 4,
               "awsPrimaryKey": 5, "deploy.host": "example.com", "notes": "call the api team",
               "hidden": "api secret", "unrelated": 6, "a_long_p_i_key": 7,
               "Åpi": "unicode", "İtem": "case", "long": "é" * 100,
               "$leo": {"secrets": ["hidden"]}}
add("search ranking and previews", [step("kv", "-s", q) for q in ["API", "dh", "key", "example", "hidden", "zzz", "", "a", "i", "long"]]
    + [step("kv", "-s", "api", "--query=type"), step("kv", "--query=type")], files={STORE:json.dumps(search_data)})

query_input = '{"cpu":2,"mem":"4Gi","hosts":["api","worker"],"nums":[1,2,3],"nested":{"b":1,"a":2}}'
queries = [".cpu", ".mem", "type", ".hosts[]", ".nums | map(. * 2)", ".nested", ".nested | keys", ".nested | to_entries",
           ".nested | values", ".missing", "empty", "select(.cpu==2)", "{x:.cpu,y:.mem}", "[.hosts[] | ascii_upcase]",
           ".nums | add", ".nums | length", ".nums | reverse", ".nums | sort", ".nums | group_by(. % 2)",
           ".hosts | join(\",\")", "paths", ".. | scalars", "[range(5)]", "reduce .nums[] as $x (0; .+$x)",
           ".cpu += 1", "del(.cpu)", ".nums[1:3]", ".nums[-1]", ".cpu // 5", "if .cpu == 2 then true else false end",
           '"hello" | @base64', '"hello world" | @uri', '"hello" | test("ell")', '"hello" | capture("(?<x>ell)")',
           '"2020-01-02T00:00:00Z" | fromdateiso8601', '[1,1,2] | unique', '["x","y"] | @csv',
           'def double: . * 2; .cpu | double', 'try .mem[] catch "caught"']
for q in queries:
    add("query " + q, [step("kv", "--query", q, "input"), step("kv", "-rc", "--query="+q, "input")],
        files={STORE:'{"input":'+query_input+'}'})
for value in ["42", "1.0", "1e20", "1e21", "1e-7", "1e-6", "9007199254740993", "-0", '"<>&"', "null", "[1,2]"]:
    add("query numeric encoding " + value, [step("kv", "--query=.", "x"), step("kv", "--query=type", "x")],files={STORE:'{"x":'+value+'}'})
for function in json.loads((Path(__file__).parent / "fixtures/builtins.json").read_text()):
    name, arity = function.rsplit("/", 1)
    call = name + ("(" + ";".join("null" for _ in range(int(arity))) + ")" if int(arity) else "")
    add("builtin available " + function, [step("kv", "--query", "empty | " + call, "x")], files={STORE:'{"x":null}'})
for q in ["builtins", "[1,2]|IN(2,3)", "IN(.cpu;1,2,3)", ".hosts|INDEX(.)", "INDEX(.hosts[];.)",
          '.hosts|JOIN({api:1,worker:2};.)', 'JOIN({api:1,worker:2};.hosts[];.)',
          'JOIN({api:1,worker:2};.hosts[];.;.[1])', '[1,2]|tostream', '[1,2]|fromstream(tostream)',
          '1|truncate_stream([1,["a"]])', '"xhellox"|trimstr("x")', '"hello"|format("base64")',
          '"x"|modulemeta', "modulemeta", 'format("wat")', "halt", "halt_error", "halt_error(7)",
          '"oops"|halt_error(3)', "1,halt", "try halt_error catch .", "input", "inputs", "debug", 'debug("x")', "stderr",
          "[", "foo", "foo(1;2)", "$foo", ".foo[", ".[?]", ". +", "select(", "@wrong",
          ".[]|.x", "1|.x", "[]|.x", 'error("oops")', "error(42)", "nan", "infinite", "null|.[]",
          'try error("oops") catch .']:
    add("query compatibility " + q, [step("kv", "--query", q, "x")], files={STORE:'{"x":'+query_input+'}'})
for expr in [".", ".[]", "type", "empty", "select(type==\"number\")"]:
    add("list search projection " + expr, [step("kv", "--query", expr), step("kv", "-s", "x", "--query", expr)],
        files={STORE:'{"x1":[1,2],"x2":[3],"x3":"secret","$leo":{"secrets":["x3"]}}'})

for lang in ["bash", "sh", "zsh", "python", "py", "node", "js", "javascript", "typescript", "ts", "cobol", ""]:
    add("generate " + lang, [step("generate", "hello", "--lang", lang), step("help"),
        step("gen", "hello", "--lang", lang), step("new", "--force", "--lang", lang, "hello")])
for name in ["", "bad name", "../escape", "bad/slash", "-leading", "a.b", "こんにちは", "A_1-z"]:
    add("generate name " + name, [step("generate", "--", name)])
add("named workspaces", [step("config", "show"), step("generate", "--workspace", "work", "deploy"),
    step("generate", "--workspace", "missing", "x"), step("help")],
    files={CONFIG:'gen_lang="python"\n[[workspace]]\nname="default"\npath="@ROOT@/personal"\n[[workspace]]\nname="work"\npath="@ROOT@/work"\n'})
for config in ["", 'store_path="~/store.json"\ngen_lang="py"', 'store_path="$TEST_PATH/store.json"',
               '[[workspace]]\npath="~/scripts"', '[[workspace]]\npath=""',
               '[[workspace]]\nname="work"\npath="same"\n[[workspace]]\nname="second"\npath="same"']:
    add("config resolution " + config, [step("config", "show"), step("kv", "x", "1"), step("generate", "x")],
        files={CONFIG:config},env={"TEST_PATH":"@ROOT@/custom"})
for config in ['store_path = 3', 'gen_lang = true', 'workspace = "oops"', '[[workspace]]\nname=3',
               '[[workspace]]\npath=false', 'store_path =', 'store_path = "unterminated',
               'store_path="a"\nstore_path="b"', '[workspace', 'not toml', 'store_path="a" garbage',
               'workspace=[{name="x",path="p"}]', 'store_path="\\e"', 'store_path="\\x61"',
               'StOrE_PaTh="@ROOT@/alternate.json"\nGeN_lAnG="py"',
               '[[WoRkSpAcE]]\nNaMe="work"\nPaTh="@ROOT@/commands"',
               'workspace = [{\nname="x",\npath="p",\n}]']:
    add("config syntax and schema " + repr(config), [step("config", "show")], files={CONFIG:config})
for path in ['$', '${}', '${HOME', '${HOME}/x', '$$', '${1}', '${UNSET}/x', '~/a/../b',
             '~/$UNSET/../x', '/a/$#/z', '$%x', '$猫', '${', '${a:b}', '$~']:
    add("path expansion " + repr(path), [step("config", "show")], files={CONFIG:'store_path='+json.dumps(path,ensure_ascii=False)})
for value in ['{"\\ud800":1}', '{"$leo":{"secrets":["\\ud800"]},"\\ud800":1}',
              '{"a":1,}', '{"a" 1}', '{a:1}', '{"a":tru}', '{"a":"\\q"}',
              '{"a":1}{}', '{"a":01}', '{"a":1e}', '{"a":1.}', '{"a":-}', '{"a":[1,]}']:
    add("malformed JSON and Unicode " + repr(value), [step("kv"), step("kv", "b", "2")], files={STORE:value})
for q in ['null|.a.b=1', '[]|.[2]=3', '.nums[.cpu]', '.nums[.cpu:]','null|setpath(["v",0];1)',
          '{b:1,a:2}|to_entries', '{b:1,a:2}|[.[]]', '{"!":1,"\\n":2,"é":3}|.',
          '[1,2]|fromstream(tostream)', '0|[range(.cpu)]',
          '[{Key:"x",Value:false},{name:"y",value:2},{Name:"z",Value:null}]|from_entries',
          '"foo 12 bar 34"|scan("([0-9]+)")', '"foo 12 bar 34"|scan("[0-9]+")',
          'try (1|.[]) catch .', 'try (1|.a) catch .', 'try ({}|.[1]) catch .',
          '1/0', 'try (1/0) catch .', '1%0', 'try (1%0) catch .', '1+"x"', 'try (1+"x") catch .',
          '[{key:1,value:2}]|from_entries', 'try error("cannot load module: hi") catch .']:
    add("query containers " + q, [step("kv", "--query", q, "x")], files={STORE:'{"x":'+query_input+'}'})
for mask in [0,0o007,0o077]:
    add("directory and file permissions " + oct(mask),[step("kv","x","1"),step("config","setup",stdin=b"\n\n\n"),step("generate","hello")],umask=mask)
add("environment workspaces", [step("config", "show"), step("generate", "x"), step("help")],env={"LEO_PATH":"@ROOT@/env1::@ROOT@/env2:@ROOT@/env1:  "})
add("alternate config and xdg", [step("config", "path"), step("config", "show"), step("kv", "x", "1")],
    files={"alternate.toml":'store_path="@ROOT@/custom.json"'},env={"LEO_CONFIG":"$TEST_CONFIG","TEST_CONFIG":"alternate.toml","XDG_CONFIG_HOME":"~/xdg-alt"})
for input_ in [b"", b"\n\n\n", b"/custom/store.json\npython\n/custom/commands\n",
               b"\npy\n\ny\nwork\n/w/cmds\nn\n", b"\ncobol\nnode\n\nn\n", b"\ncobol", b"\n\n\ny\n\n\nn\n"]:
    add("config setup " + repr(input_), [step("config", "setup", stdin=input_), step("config", "show"), step("config", "setup", stdin=b"\n\n\n\nn\n")])
add("setup retains literal paths and workspaces", [step("config", "setup", stdin=b"\n\n\n\nn\n"), step("config", "show")],
    files={CONFIG:'store_path="~/store.json"\ngen_lang="ts"\n[[workspace]]\nname="personal"\npath="~/personal"\n[[workspace]]\nname="work"\npath="$TEST_PATH/work"'},env={"TEST_PATH":"@ROOT@"})

external_script = """#!/bin/sh
# leo: echo arguments and environment
printf '%s\\n' "$LEO_BIN" "$LEO_STORE" "$LEO_CONFIG" "$LEO_WORKSPACE_PATHS" "$PATH"
printf '<%s>\\n' "$@"
cat
"""
add("external arguments environment and stdin", [step("echo", "--help", "--", "space value", "$(false)", "*", "-x", stdin=b"piped input\n"),
    step("help"), step("help", "echo")], files={COMMANDS+"/leo-echo":external_script},modes={COMMANDS+"/leo-echo":0o755},
    env={"LEO_BIN":"old","LEO_STORE":"old","LEO_WORKSPACE_PATHS":"old"})
for code in [0, 1, 42, 127, 130, 255]:
    add("external exit " + str(code), [step("exit", "--anything")], files={COMMANDS+"/leo-exit":"#!/bin/sh\nexit "+str(code)+"\n"},modes={COMMANDS+"/leo-exit":0o755})
for body in ["trap 'exit 0' INT", "trap 'exit 42' INT", ""]:
    add("external interrupt " + body, [step("wait", interrupt=True)], files={COMMANDS+"/leo-wait":"#!/bin/sh\n"+body+"\necho READY\nwhile :; do sleep 1; done\n"},modes={COMMANDS+"/leo-wait":0o755})
add("external precedence and discovery", [step("help"), step("dupe"), step("path"), step("kv", "x", "1"), step("completion")],
    files={CONFIG:'[[workspace]]\nname="first"\npath="@ROOT@/first"\n[[workspace]]\nname="second"\npath="@ROOT@/second"',
           "first/leo-dupe.py":"#!/bin/sh\n# leo: first\necho first\n", "second/leo-dupe":"#!/bin/sh\n# leo: second\necho second\n",
           "bin/leo-path":"#!/bin/sh\n// leo: from path\necho path\n", "first/leo-kv":"#!/bin/sh\nexit 99\n",
           "first/leo-help":"#!/bin/sh\nexit 99\n", "first/leo-ignored.txt":"ignored", "first/leo-completion":"#!/bin/sh\necho custom completion\n"},
    modes={"first/leo-dupe.py":0o755,"second/leo-dupe":0o755,"bin/leo-path":0o755,"first/leo-kv":0o755,"first/leo-help":0o755,"first/leo-completion":0o755})
add("nonexecutable external", [step("help"), step("broken")],files={COMMANDS+"/leo-broken":"#!/bin/sh\necho broken\n"})
add("symlink external", [step("linked","a")], files={"actual":"#!/bin/sh\necho linked \"$@\"\n"},modes={"actual":0o755},links={COMMANDS+"/leo-linked":"@ROOT@/actual"})
add("long external names help width", [step("help")],files={COMMANDS+"/leo-very-long-command-name":"#!/bin/sh\n# leo: long\n"})

add("Unicode external help width", [step("help"),step("猫猫猫猫猫")],files={COMMANDS+"/leo-猫猫猫猫猫":"#!/bin/sh\n# leo: unicode\necho unicode\n"},modes={COMMANDS+"/leo-猫猫猫猫猫":0o755})
add("external raw argument bytes", [step("echo",os.fsdecode(b"\xff\xfe\xe2\x82"))],files={COMMANDS+"/leo-echo":"#!/bin/sh\nprintf '%s' \"$1\"\n"},modes={COMMANDS+"/leo-echo":0o755})
add("forced generation retains existing permissions",[step("generate","--force","existing")],files={COMMANDS+"/leo-existing":"old"},modes={COMMANDS+"/leo-existing":0o600})
for path in [".:/usr/bin:/bin", "/usr/bin:/bin", ""]:
    add("external cwd lookup " + path, [step("local")],files={"leo-local":"#!/bin/sh\necho local\n"},modes={"leo-local":0o755},env={"LEO_PATH":".","PATH":path})
add("relative PATH entry",[step("local")],files={"bin/leo-local":"#!/bin/sh\necho local\n"},modes={"bin/leo-local":0o755},env={"PATH":"bin:/usr/bin:/bin"})

if sys.platform == "darwin":
    stubs = {"bin/pbcopy":"#!/bin/sh\ncat > \"$CAPTURE\"\n", "bin/osascript":"#!/bin/sh\nprintf '%s' \"$LEO_CLIP_ITEMS\" > \"$CAPTURE\"\nprintf '%s' \"$4\" > \"$CAPTURE.script\"\necho ok\n"}
    files = {**stubs, STORE: '{"greeting":"hello there","limits":{"cpu":2},"logo":"@ROOT@/logo.PNG","secret":"shh","$leo":{"secrets":["secret"]}}',
             "logo.PNG":b"image", "report.pdf":b"pdf", "quote'file.png":b"image", "home/file.jpg":b"image"}
    add("clipboard resolution file payload and image types", [step("clip", "greeting"), step("clip", "limits"), step("clip", "--pretty", "limits"),
        step("clip", "logo"), step("clip", "logo.PNG", "report.pdf", "quote'file.png", "~/file.jpg"),
        step("clip", "--text", "logo"), step("clip", "greeting", "logo"), step("clip", "secret"),
        step("clip", "--text", "literal", "another"), step("clip", "absent"), step("clip", "x", "y"),
        step("clip", "--file", "missing"), step("clip", "$leo")],files=files,modes={"bin/pbcopy":0o755,"bin/osascript":0o755},env={"CAPTURE":"@ROOT@/clipboard"})
    add("clipboard logical working directory",[step("clip","file.png")],files={**stubs,"file.png":b"x"},modes={"bin/osascript":0o755},links={"alias":"@ROOT@"},env={"PWD":"@ROOT@/alias","CAPTURE":"@ROOT@/clipboard"})
    for ext in ["png","jpg","jpeg","gif","tiff","tif","bmp","heic","heif","webp","pdf"]:
        add("clipboard type " + ext,[step("clip","file."+ext)],files={**stubs,"file."+ext:b"x"},modes={"bin/osascript":0o755},env={"CAPTURE":"@ROOT@/clipboard"})
    for body in ["echo failed >&2; exit 7", "echo write-failed", "echo nothing"]:
        add("clipboard errors " + body,[step("clip","file.png"),step("clip","--text","x")],
            files={"file.png":b"x","bin/osascript":"#!/bin/sh\n"+body+"\n","bin/pbcopy":"#!/bin/sh\nexit 7\n"},modes={"bin/osascript":0o755,"bin/pbcopy":0o755})


def replace(s, root):
    return s.replace("@ROOT@", str(root))


def prepare(case, root):
    if root.exists():
        shutil.rmtree(root)
    for d in [root, root / "home", root / "bin"]:
        d.mkdir(parents=True, exist_ok=True)
    for name, content in case.files.items():
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(replace(content, root).encode() if isinstance(content, str) else content)
        path.chmod(case.modes.get(name, 0o644))
    for name, target in case.links.items():
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.symlink_to(replace(target, root))


def snapshot(root):
    result = {}
    for path in sorted(root.rglob("*")):
        if path.is_symlink():
            result[str(path.relative_to(root))] = ("link", os.readlink(path))
        elif path.is_file():
            result[str(path.relative_to(root))] = (stat.S_IMODE(path.stat().st_mode), path.read_bytes())
        elif path.is_dir():
            result[str(path.relative_to(root))] = ("directory", stat.S_IMODE(path.stat().st_mode))
    return result


def run(case, binary, root):
    # Fixture inputs must not inherit the developer's filesystem policy. Cases
    # that exercise another umask apply it only to the CLI process below.
    original_umask = os.umask(0o022)
    try:
        prepare(case, root)
    finally:
        os.umask(original_umask)
    env = {k: v for k, v in os.environ.items() if not k.startswith("LEO_") and k not in ["XDG_CONFIG_HOME", "HOME"]}
    env.update(HOME=str(root / "home"), XDG_CONFIG_HOME=str(root / "xdg"), PATH=str(root / "bin")+":/usr/bin:/bin")
    env.update({k: replace(v, root) for k, v in case.env.items()})
    preexec = lambda: os.umask(case.umask if case.umask >= 0 else 0o022)
    trace = []
    for command in case.steps:
        args = [replace(a, root) for a in command.args]
        if command.stdout_closed or command.stderr_closed:
            read_fd, write_fd = os.pipe()
            os.close(read_fd)
            try:
                p = subprocess.run([str(binary),*args],input=command.stdin,
                    stdout=write_fd if command.stdout_closed else subprocess.PIPE,
                    stderr=write_fd if command.stderr_closed else subprocess.PIPE,
                    env=env,cwd=root,timeout=10,preexec_fn=preexec)
                stdout,stderr=p.stdout or b"",p.stderr or b""
            finally:
                os.close(write_fd)
        elif command.tty:
            master, slave = pty.openpty()
            p = subprocess.Popen([str(binary), *args], stdin=slave, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env, cwd=root, preexec_fn=preexec)
            os.close(slave)
            os.write(master, command.stdin)
            try:
                stdout, stderr = p.communicate(timeout=10)
            finally:
                os.close(master)
        elif command.interrupt:
            p = subprocess.Popen([str(binary), *args], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env, cwd=root, start_new_session=True, preexec_fn=preexec)
            if not select.select([p.stdout], [], [], 10)[0]:
                os.killpg(p.pid, signal.SIGKILL)
                raise TimeoutError("external command did not become ready: " + case.name)
            ready = p.stdout.readline()
            assert ready == b"READY\n", (case.name, ready)
            os.killpg(p.pid, signal.SIGINT)
            stdout, stderr = p.communicate(timeout=10)
            stdout = ready + stdout
        else:
            p = subprocess.run([str(binary), *args], input=command.stdin, capture_output=True, env=env, cwd=root, timeout=10, preexec_fn=preexec)
            stdout, stderr = p.stdout, p.stderr
        # Executable paths intentionally differ; normalize only that identity.
        stdout = stdout.replace(str(binary).encode(), b"<LEO_BIN>")
        stdout = stdout.replace((str(binary.parent)+":").encode(), b"<LEO_BIN_DIR>:")
        if args in [["version"], ["--version"], ["-v"]]:
            version = subprocess.run([str(binary), "version"], capture_output=True, env=env, cwd=root, check=True).stdout.removeprefix(b"leo ").rstrip(b"\n")
            stdout = stdout.replace(b"leo " + version + b"\n", b"leo <VERSION>\n").replace(b"leo version " + version + b"\n", b"leo version <VERSION>\n")
        trace.append((p.returncode, stdout, stderr, snapshot(root)))
    return trace


def case_digest(case):
    """Bind expectations to their inputs so edited cases cannot silently pass."""
    payload = json.dumps(dataclasses.asdict(case), sort_keys=True,
                         default=lambda b: {"bytes": b.hex()})
    return hashlib.sha256(payload.encode()).hexdigest()


def recorded_trace(trace, root):
    # surrogateescape round-trips arbitrary bytes, including non-UTF-8 argv and
    # previews truncated in the middle of a UTF-8 character. JSON escapes them.
    def text(value):
        if isinstance(value, bytes):
            value = value.decode("utf-8", errors="surrogateescape")
        return value.replace(str(root), "@ROOT@")

    result = []
    for status, stdout, stderr, files in trace:
        entries = {}
        for name, (kind, contents) in files.items():
            if kind == "link":
                entry = {"kind": "link", "target": text(contents)}
            elif kind == "directory":
                entry = {"kind": "directory", "mode": contents}
            else:
                entry = {"kind": "file", "mode": kind, "contents": text(contents)}
            entries[name] = entry
        result.append({"exit": status, "stdout": text(stdout),
                       "stderr": text(stderr), "files": entries})
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--rust", type=Path, required=True)
    parser.add_argument("--filter", default="")
    args = parser.parse_args()
    rust = args.rust.resolve()
    fixture = json.loads((Path(__file__).parent / "fixtures/cli.json").read_text())
    if fixture["schema"] != 1:
        parser.error("unsupported CLI fixture schema")
    with tempfile.TemporaryDirectory(prefix="leo-parity-") as temp:
        base = Path(temp).resolve()
        failed = 0
        checked = 0
        for case in CASES:
            if args.filter not in case.name:
                continue
            saved = fixture["cases"].get(case.name)
            if saved is None or saved["input_sha256"] != case_digest(case):
                print("FAIL:", case.name, "has missing or stale expectations")
                failed += 1
                continue
            expected = saved["steps"]
            actual = recorded_trace(run(case, rust, base / "fixture"), base / "fixture")
            checked += len(case.steps)
            if len(expected) != len(actual):
                print("FAIL:", case.name, "has the wrong number of expected steps")
                failed += 1
                continue
            for i, (want, got) in enumerate(zip(expected, actual)):
                if want == got:
                    continue
                failed += 1
                print("FAIL:", case.name, "step", i, case.steps[i].args)
                for label in ["exit", "stdout", "stderr", "files"]:
                    a, b = want[label], got[label]
                    if a != b:
                        print(" ", label, "Expected:", repr(a)[:1800], "Actual:", repr(b)[:1800])
                break
        scenarios = sum(args.filter in c.name for c in CASES)
        if not scenarios:
            parser.error("no scenarios match --filter")
        print(f"Checked {checked} invocations across {scenarios} scenarios; {failed} failures.")
        return bool(failed)


if __name__ == "__main__":
    sys.exit(main())
