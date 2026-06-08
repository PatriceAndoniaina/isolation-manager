package nginx

import (
	"os"
	"path/filepath"
	"testing"
)

// setupSite crée base/sites-available/name.conf et renvoie (file, enabledDir).
func setupSite(t *testing.T, name string) (string, string) {
	t.Helper()
	base := t.TempDir()
	avail := filepath.Join(base, "sites-available")
	if err := os.MkdirAll(avail, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(avail, name)
	if err := os.WriteFile(file, []byte("http {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return file, filepath.Join(base, "sites-enabled")
}

func TestEnabledDir(t *testing.T) {
	got := EnabledDir("/etc/nginx/sites-available/site.conf")
	want := filepath.FromSlash("/etc/nginx/sites-enabled")
	if got != want {
		t.Errorf("EnabledDir = %q, want %q", got, want)
	}
}

func TestEnableCreatesSymlink(t *testing.T) {
	file, enabled := setupSite(t, "site.conf")
	link, created, err := Enable(file, enabled)
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !created {
		t.Error("created = false, want true on first enable")
	}
	abs, _ := filepath.Abs(file)
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != abs {
		t.Errorf("symlink → %q, want %q", target, abs)
	}
}

func TestEnableIdempotent(t *testing.T) {
	file, enabled := setupSite(t, "site.conf")
	if _, _, err := Enable(file, enabled); err != nil {
		t.Fatalf("first Enable: %v", err)
	}
	_, created, err := Enable(file, enabled)
	if err != nil {
		t.Fatalf("second Enable: %v", err)
	}
	if created {
		t.Error("created = true on re-enable, want false (idempotent)")
	}
}

func TestEnableConflict(t *testing.T) {
	file, enabled := setupSite(t, "site.conf")
	// Un lien du même nom pointe déjà ailleurs.
	if err := os.MkdirAll(enabled, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/autre/cible", filepath.Join(enabled, "site.conf")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Enable(file, enabled); err == nil {
		t.Fatal("Enable = nil, want conflict error")
	}
}

func TestEnableMissingFile(t *testing.T) {
	if _, _, err := Enable(filepath.Join(t.TempDir(), "ghost.conf"), t.TempDir()); err == nil {
		t.Fatal("Enable missing file = nil, want error")
	}
}

func TestDisableRemovesSymlink(t *testing.T) {
	file, enabled := setupSite(t, "site.conf")
	link, _, err := Enable(file, enabled)
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if _, err := Disable(file, enabled); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("symlink still present after Disable")
	}
}

func TestDisableNotEnabled(t *testing.T) {
	file, enabled := setupSite(t, "site.conf")
	if _, err := Disable(file, enabled); err == nil {
		t.Fatal("Disable not-enabled = nil, want error")
	}
}

func TestDisableRefusesRegularFile(t *testing.T) {
	file, enabled := setupSite(t, "site.conf")
	// Un vrai fichier (pas un lien) dans enabled : Disable doit refuser.
	if err := os.MkdirAll(enabled, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(enabled, "site.conf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Disable(file, enabled); err == nil {
		t.Fatal("Disable on regular file = nil, want refusal")
	}
	// Le fichier régulier ne doit pas avoir été supprimé.
	if _, err := os.Stat(filepath.Join(enabled, "site.conf")); err != nil {
		t.Error("regular file was removed despite refusal")
	}
}
