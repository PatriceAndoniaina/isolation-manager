package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	// Arborescence mêlant .conf (à divers niveaux) et autres fichiers.
	must := func(p, content string) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("nginx.conf", "")
	must("conf.d/api.conf", "")
	must("sites-enabled/site.conf", "")
	must("README.md", "")        // ignoré
	must("conf.d/notes.txt", "") // ignoré

	files, err := ListFiles(dir)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("got %d files, want 3: %v", len(files), files)
	}
	// Triés et récursifs.
	joined := strings.Join(files, "\n")
	for _, want := range []string{"nginx.conf", "conf.d/api.conf", "sites-enabled/site.conf"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, files)
		}
	}
	if !sortedAscending(files) {
		t.Errorf("files not sorted: %v", files)
	}
}

func TestListFilesEmpty(t *testing.T) {
	files, err := ListFiles(t.TempDir())
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("got %v, want empty", files)
	}
}

func TestListFilesMissingDir(t *testing.T) {
	if _, err := ListFiles(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("ListFiles on missing dir = nil, want error")
	}
}

func sortedAscending(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}
