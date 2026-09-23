// Package clip implements leo's macOS clipboard mechanics: copying existing
// files as file objects (public.file-url, so Cmd+V pastes the actual file) and
// copying strings / JSON values as text.
package clip

import (
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

// jxaFileURLScript writes the file paths passed as argv onto the general
// pasteboard as NSURLs, so a paste in Finder produces the real files. Paths
// arrive as separate process arguments (never interpolated into the script),
// which keeps them out of any shell/script parsing path.
const jxaFileURLScript = `
ObjC.import('AppKit');
function run(argv) {
  const pb = $.NSPasteboard.generalPasteboard;
  pb.clearContents;
  const urls = argv.map(p => $.NSURL.fileURLWithPath(p));
  pb.writeObjects(urls);
  return 'ok';
}
`

// CopyFiles copies one or more existing files to the clipboard as file
// objects. Paths are made absolute and must exist.
func CopyFiles(paths []string) error {
	if !Supported() {
		return ErrUnsupported
	}
	if len(paths) == 0 {
		return fmt.Errorf("no files to copy")
	}
	abs := make([]string, 0, len(paths))
	for _, p := range paths {
		ap := expand(p)
		if !filepath.IsAbs(ap) {
			if wd, err := filepath.Abs(ap); err == nil {
				ap = wd
			}
		}
		if _, err := os.Stat(ap); err != nil {
			return fmt.Errorf("not an existing file: %s", p)
		}
		abs = append(abs, ap)
	}
	// osascript -l JavaScript -e <script> <path>...  → paths become run()'s argv.
	args := append([]string{"-l", "JavaScript", "-e", jxaFileURLScript}, abs...)
	cmd := exec.Command("osascript", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copying files to clipboard: %w: %s", err, strings.TrimSpace(string(out)))
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
