#!/usr/bin/env python3
"""Opt-in macOS clipboard parity test. Restores every original item/type."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

SNAPSHOT = r"""
ObjC.import('AppKit');
const pb = $.NSPasteboard.generalPasteboard;
const items = [];
const native = $.NSArray.arrayWithArray(pb.pasteboardItems);
const count = Number(native.count);
for (let i = 0; i < count; i++) {
  const item = native.objectAtIndex(i);
  const data = {};
  const types = ObjC.deepUnwrap(item.types);
  for (let j = 0; j < types.length; j++) {
    const bytes = item.dataForType(types[j]);
    if (!bytes.isNil()) data[types[j]] = bytes.base64EncodedStringWithOptions(0).js;
  }
  items.push(data);
}
JSON.stringify({items: items});
"""
RESTORE = r"""
ObjC.import('AppKit');
const path = $.NSProcessInfo.processInfo.environment.objectForKey('LEO_CLIP_SNAPSHOT').js;
const text = $.NSString.stringWithContentsOfFileEncodingError(path, $.NSUTF8StringEncoding, null).js;
const original = JSON.parse(text);
const items = $.NSMutableArray.alloc.init;
for (let i = 0; i < original.items.length; i++) {
  const item = $.NSPasteboardItem.alloc.init;
  const types = Object.keys(original.items[i]);
  for (let j = 0; j < types.length; j++) {
    const data = $.NSData.alloc.initWithBase64EncodedStringOptions(original.items[i][types[j]], 0);
    item.setDataForType(data, types[j]);
  }
  items.addObject(item);
}
const pb = $.NSPasteboard.generalPasteboard;
pb.clearContents;
if (items.count > 0 && !pb.writeObjects(items)) throw Error('restoring clipboard failed');
'restored';
"""


def jxa(script, env=None):
    result = subprocess.run(["/usr/bin/osascript", "-l", "JavaScript", "-e", script],
                            capture_output=True, text=True, env=env, timeout=20)
    if result.returncode:
        raise RuntimeError(result.stderr.strip())
    return result.stdout.strip()


def snapshot_when(predicate):
    deadline = time.monotonic() + 2
    while True:
        snapshot = json.loads(jxa(SNAPSHOT))
        if predicate(snapshot) or time.monotonic() >= deadline:
            return snapshot
        time.sleep(0.05)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("binaries", nargs="+", type=Path)
    args = parser.parse_args()
    if sys.platform != "darwin":
        parser.error("requires macOS")
    binaries = [b.resolve() for b in args.binaries]
    with tempfile.TemporaryDirectory(prefix="leo-native-clipboard-") as tmp:
        root = Path(tmp).resolve()
        backup = root / "original.json"
        backup.touch(mode=0o600)
        backup.write_text(jxa(SNAPSHOT))
        original = json.loads(backup.read_text())
        assert isinstance(original["items"], list)
        env = {k:v for k,v in os.environ.items() if not k.startswith("LEO_")}
        env.update(XDG_CONFIG_HOME=str(root / "xdg"), LEO_CONFIG="", LEO_PATH="")
        png = root / "image.png"
        png.write_bytes(base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII="))
        pdf = root / "report.pdf"
        pdf.write_bytes(b"%PDF-1.4\n")
        other = root / "other.txt"
        other.write_text("another file")
        results = []
        try:
            for binary in binaries:
                def run(*cmd):
                    return subprocess.run([str(binary),*cmd],env=env,capture_output=True,timeout=20,check=True).stdout
                run("kv", "-f", "note", "native clipboard text\nsecond line")
                run("clip", "note")
                text = subprocess.run(["/usr/bin/pbpaste"],capture_output=True,timeout=10,check=True).stdout
                assert text == b"native clipboard text\nsecond line", (binary, "text mismatch")
                run("clip", str(pdf), str(other))
                items = snapshot_when(lambda s: len(s["items"]) == 2)["items"]
                assert items and all("public.file-url" in item for item in items), (binary, "file objects missing")
                run("clip", str(png))
                images = snapshot_when(lambda s: len(s["items"]) == 1 and "public.png" in s["items"][0])["items"]
                assert len(images) == 1 and "public.file-url" in images[0]
                assert base64.b64decode(images[0]["public.png"]) == png.read_bytes(), (binary, "image bytes mismatch")
                results.append((items,images))
            if not all(result == results[0] for result in results):
                for binary, result in zip(binaries, results):
                    summary = [[{k:hashlib.sha256(base64.b64decode(v)).hexdigest()[:12] for k,v in item.items()} for item in group] for group in result]
                    print(binary.name, "test pasteboard type/hash summary:", summary)
                raise AssertionError("Go and Rust pasteboard items differ")
        finally:
            restored = jxa(RESTORE, dict(os.environ, LEO_CLIP_SNAPSHOT=str(backup)))
            assert restored == "restored"
            assert snapshot_when(lambda s: s == original) == original, "clipboard restoration did not round-trip"
        count = len(results[0][0])
        label = "parity" if len(binaries) > 1 else "checks"
        print(f"Native clipboard {label} passed: text, file URLs, PNG data; original clipboard restored. Observed {count} item(s) for two requested files in each tested binary.")


if __name__ == "__main__":
    main()
