package clip

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSupported(t *testing.T) {
	if Supported() != (runtime.GOOS == "darwin") {
		t.Errorf("Supported() = %v on %s", Supported(), runtime.GOOS)
	}
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := expand("~/x"); got != filepath.Join(home, "x") {
		t.Errorf("expand(~/x) = %q", got)
	}
	if got := expand("~"); got != home {
		t.Errorf("expand(~) = %q", got)
	}
	if got := expand("/abs/path"); got != "/abs/path" {
		t.Errorf("expand(/abs/path) = %q", got)
	}
}

func TestImageUTI(t *testing.T) {
	cases := map[string]string{
		"/x/a.png":  "public.png",
		"/x/a.PNG":  "public.png",
		"/x/a.jpg":  "public.jpeg",
		"/x/a.jpeg": "public.jpeg",
		"/x/a.gif":  "com.compuserve.gif",
		"/x/a.txt":  "",
		"/x/noext":  "",
	}
	for p, want := range cases {
		if got := imageUTI(p); got != want {
			t.Errorf("imageUTI(%q) = %q, want %q", p, got, want)
		}
	}
}

// TestCopyFilesValidation checks the error paths that never touch the real
// clipboard (empty input, and a path that does not exist). On non-macOS both
// return ErrUnsupported, which is still a non-nil error.
func TestCopyFilesValidation(t *testing.T) {
	if err := CopyFiles(nil); err == nil {
		t.Error("CopyFiles(nil) should error")
	}
	if err := CopyFiles([]string{filepath.Join(t.TempDir(), "does-not-exist")}); err == nil {
		t.Error("CopyFiles on a missing file should error")
	}
}
