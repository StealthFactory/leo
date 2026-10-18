// Package clip implements leo's macOS clipboard mechanics. It copies existing
// files as a pasteboard item carrying both the file URL (public.file-url, so
// Finder pastes the real file) and, for images, the image bytes under their
// type (public.png, com.compuserve.gif, ...) so apps like Slack and Telegram
// paste the image inline. Strings and JSON values are copied as text.
package clip

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrUnsupported is returned on non-macOS platforms.
var ErrUnsupported = fmt.Errorf("leo clip is only supported on macOS")

// Supported reports whether the clipboard mechanics work on this platform.
func Supported() bool { return runtime.GOOS == "darwin" }

// imageUTIByExt maps a lowercase file extension to the macOS pasteboard type
// for its image bytes. Files not listed here are copied as a file URL only.
var imageUTIByExt = map[string]string{
	".png":  "public.png",
	".jpg":  "public.jpeg",
	".jpeg": "public.jpeg",
	".gif":  "com.compuserve.gif",
	".tiff": "public.tiff",
	".tif":  "public.tiff",
	".bmp":  "com.microsoft.bmp",
	".heic": "public.heic",
	".heif": "public.heic",
	".webp": "org.webmproject.webp",
}

// imageUTI returns the pasteboard image type for path, or "" if it is not a
// recognized image.
func imageUTI(path string) string {
	return imageUTIByExt[strings.ToLower(filepath.Ext(path))]
}

// jxaCopyScript reads a JSON array of {path,type} items from the LEO_CLIP_ITEMS
// environment variable and writes each onto the general pasteboard as one item
// carrying its file URL (so Finder pastes the file) and, when type is set, the
// raw image bytes under that type (so image-aware apps paste the image inline).
// Paths travel through the environment as JSON, never interpolated into the
// script, so nothing in a path is parsed as code.
const jxaCopyScript = `
ObjC.import('AppKit');
function run() {
  const raw = $.NSProcessInfo.processInfo.environment.objectForKey('LEO_CLIP_ITEMS');
  if (raw.isNil()) return 'no-items';
  const items = JSON.parse(raw.js);
  const pb = $.NSPasteboard.generalPasteboard;
  pb.clearContents;
  const arr = $.NSMutableArray.alloc.init;
  for (let i = 0; i < items.length; i++) {
    const url = $.NSURL.fileURLWithPath(items[i].path);
    const item = $.NSPasteboardItem.alloc.init;
    item.setStringForType(url.absoluteString, 'public.file-url');
    if (items[i].type) {
      const data = $.NSData.dataWithContentsOfFile(items[i].path);
      if (!data.isNil()) item.setDataForType(data, items[i].type);
    }
    arr.addObject(item);
  }
  return pb.writeObjects(arr) ? 'ok' : 'write-failed';
}
`

// clipItem is one file to place on the pasteboard: its absolute path and, if it
// is an image, the pasteboard type for its bytes ("" otherwise).
type clipItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// CopyFiles copies one or more existing files to the clipboard. Each file is
// written as a pasteboard item with its file URL (so Finder pastes the file)
// and, for images, its bytes under the image type (so Slack, Telegram, and
// other image-aware apps paste the image inline). Paths are made absolute and
// must exist.
func CopyFiles(paths []string) error {
	if !Supported() {
		return ErrUnsupported
	}
	if len(paths) == 0 {
		return fmt.Errorf("no files to copy")
	}
	items := make([]clipItem, 0, len(paths))
	for _, p := range paths {
		ap := expand(p)
		if !filepath.IsAbs(ap) {
			if abs, err := filepath.Abs(ap); err == nil {
				ap = abs
			}
		}
		if _, err := os.Stat(ap); err != nil {
			return fmt.Errorf("not an existing file: %s", p)
		}
		items = append(items, clipItem{Path: ap, Type: imageUTI(ap)})
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return err
	}

	// Paths are passed via the environment as JSON (not interpolated into the
	// script), so nothing in a path is ever parsed as code.
	cmd := exec.Command("osascript", "-l", "JavaScript", "-e", jxaCopyScript)
	cmd.Env = append(os.Environ(), "LEO_CLIP_ITEMS="+string(payload))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copying files to clipboard: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if strings.TrimSpace(stdout.String()) != "ok" {
		return fmt.Errorf("copying files to clipboard failed: %s", strings.TrimSpace(stdout.String()+" "+stderr.String()))
	}
	return nil
}

// CopyText copies text to the clipboard via pbcopy.
func CopyText(text string) error {
	if !Supported() {
		return ErrUnsupported
	}
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copying text to clipboard: %w", err)
	}
	return nil
}

// expand expands a leading ~ to the user's home directory.
func expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				return h
			}
			return filepath.Join(h, p[2:])
		}
	}
	return p
}
