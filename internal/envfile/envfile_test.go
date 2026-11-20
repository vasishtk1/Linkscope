package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("# comment\nLINKSCOPE_TEST_A=\"from file\"\nLINKSCOPE_TEST_AB=other\n"), 0o600)
	if got := Get(path, "LINKSCOPE_TEST_A"); got != "from file" {
		t.Errorf("file: got %q", got)
	}
	if got := Get(path, "LINKSCOPE_TEST_MISSING"); got != "" {
		t.Errorf("missing: got %q", got)
	}
	t.Setenv("LINKSCOPE_TEST_A", "from env")
	if got := Get(path, "LINKSCOPE_TEST_A"); got != "from env" {
		t.Errorf("env: got %q", got)
	}
	if got := Get(filepath.Join(t.TempDir(), "none"), "LINKSCOPE_TEST_B"); got != "" {
		t.Errorf("no file: got %q", got)
	}
}
