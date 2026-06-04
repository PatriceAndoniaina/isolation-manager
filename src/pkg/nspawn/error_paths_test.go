package nspawn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// --- store: chemins d'erreur ---

func TestStoreSaveMkdirError(t *testing.T) {
	// Un ancêtre du répertoire est un fichier → MkdirAll échoue (ENOTDIR).
	tmp := t.TempDir()
	file := filepath.Join(tmp, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newFileStore(filepath.Join(file, "sub"))
	if err := s.save(&container.Container{Name: "x"}); err == nil {
		t.Fatal("save = nil, want mkdir error")
	}
}

func TestStoreLoadCorrupt(t *testing.T) {
	s := newFileStore(t.TempDir())
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path("bad"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.load("bad"); err == nil {
		t.Fatal("load = nil, want unmarshal error")
	}
}

func TestStoreDeleteMissing(t *testing.T) {
	s := newFileStore(t.TempDir())
	if err := s.delete("ghost"); !apperrors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestStoreListAbsentDir(t *testing.T) {
	s := newFileStore(filepath.Join(t.TempDir(), "does-not-exist"))
	list, err := s.list()
	if err != nil || list != nil {
		t.Fatalf("list = %v, %v; want nil, nil", list, err)
	}
}

func TestStoreListCorrupt(t *testing.T) {
	s := newFileStore(t.TempDir())
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path("bad"), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.list(); err == nil {
		t.Fatal("list = nil, want error from corrupt entry")
	}
}

// --- lifecycle: chemins d'erreur ---

func TestCreateStoreListError(t *testing.T) {
	// Une métadonnée corrompue fait échouer le list() interne de Create.
	m := newTestManager(t, nil)
	if err := os.MkdirAll(m.store.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.store.path("corrupt"), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), container.CreateOptions{Name: "fresh"}); err == nil {
		t.Fatal("Create = nil, want error from corrupt store")
	}
}

func TestCreateMkdirError(t *testing.T) {
	// machinesDir est un fichier → la création du rootfs échoue.
	tmp := t.TempDir()
	machinesFile := filepath.Join(tmp, "machines")
	if err := os.WriteFile(machinesFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(WithRunner(&fakeRunner{}), WithStateDir(t.TempDir()), WithMachinesDir(machinesFile))
	if _, err := m.Create(context.Background(), container.CreateOptions{Name: "rootfs"}); err == nil {
		t.Fatal("Create = nil, want mkdir error")
	}
}

func TestStartExecError(t *testing.T) {
	fr := &fakeRunner{err: errors.New("systemd-nspawn failed")}
	m := newTestManager(t, fr)
	ctx := context.Background()
	if _, err := m.Create(ctx, container.CreateOptions{Name: "se"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Start(ctx, "se"); err == nil {
		t.Fatal("Start = nil, want exec error")
	}
}

func TestStopNotFound(t *testing.T) {
	m := newTestManager(t, &fakeRunner{})
	if err := m.Stop(context.Background(), "ghost"); !apperrors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetTreatsRunnerErrorAsStopped(t *testing.T) {
	// machinectl show échoue → isRunning renvoie false (machine non active).
	sr := &scriptedRunner{failOn: "show"}
	m := newTestManager(t, sr)
	ctx := context.Background()
	if _, err := m.Create(ctx, container.CreateOptions{Name: "ge"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	c, err := m.Get(ctx, "ge")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if c.State == container.StateRunning {
		t.Error("state = running, want non-running when machinectl show errors")
	}
}
