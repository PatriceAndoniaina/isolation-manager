package preflight

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// fakeCommander simule la présence de binaires et enregistre les installations.
type fakeCommander struct {
	present  map[string]bool // binaires présents
	installs [][]string      // commandes d'installation reçues
	runErr   error
	// installed : binaires qui deviennent présents après une installation.
	installed map[string]bool
}

func (f *fakeCommander) Look(name string) bool {
	return f.present[name]
}

func (f *fakeCommander) Run(_ context.Context, argv []string) ([]byte, error) {
	f.installs = append(f.installs, argv)
	if f.runErr != nil {
		return []byte("install error"), f.runErr
	}
	// L'installation rend les binaires demandés présents.
	for k := range f.installed {
		f.present[k] = true
	}
	return nil, nil
}

func newChecker(f *fakeCommander) *Checker { return New(WithCommander(f)) }

func TestEnsureAllPresent(t *testing.T) {
	f := &fakeCommander{present: map[string]bool{"ssh": true, "ssh-keygen": true}}
	if err := newChecker(f).Ensure(context.Background(), io.Discard, []string{"ssh", "ssh-keygen"}, true); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if len(f.installs) != 0 {
		t.Errorf("no install expected, got %v", f.installs)
	}
}

func TestEnsureInstallsMissing(t *testing.T) {
	f := &fakeCommander{
		present:   map[string]bool{"apt-get": true},
		installed: map[string]bool{"systemd-nspawn": true, "machinectl": true},
	}
	var buf strings.Builder
	err := newChecker(f).Ensure(context.Background(), &buf, []string{"systemd-nspawn", "machinectl"}, true)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if len(f.installs) != 1 {
		t.Fatalf("expected 1 install, got %d", len(f.installs))
	}
	got := strings.Join(f.installs[0], " ")
	// systemd-nspawn et machinectl proviennent du même paquet → dédupliqué.
	if got != "sudo apt-get install -y systemd-container" {
		t.Errorf("install cmd = %q", got)
	}
	if !strings.Contains(buf.String(), "systemd-container") {
		t.Errorf("progress not reported: %s", buf.String())
	}
}

func TestEnsureNoAutoInstall(t *testing.T) {
	f := &fakeCommander{present: map[string]bool{}}
	err := newChecker(f).Ensure(context.Background(), io.Discard, []string{"systemd-nspawn"}, false)
	if !apperrors.Is(err, apperrors.ErrDependencyMissing) {
		t.Fatalf("err = %v, want ErrDependencyMissing", err)
	}
	if len(f.installs) != 0 {
		t.Error("must not install when autoInstall is false")
	}
}

func TestEnsureNoPackageManager(t *testing.T) {
	f := &fakeCommander{present: map[string]bool{}} // aucun apt/dnf/pacman
	err := newChecker(f).Ensure(context.Background(), io.Discard, []string{"rsync"}, true)
	if !apperrors.Is(err, apperrors.ErrDependencyMissing) {
		t.Fatalf("err = %v, want ErrDependencyMissing", err)
	}
	if !strings.Contains(err.Error(), "gestionnaire de paquets") {
		t.Errorf("err = %v, want mention of package manager", err)
	}
}

func TestEnsureInstallFails(t *testing.T) {
	f := &fakeCommander{present: map[string]bool{"dnf": true}, runErr: errors.New("boom")}
	err := newChecker(f).Ensure(context.Background(), io.Discard, []string{"rsync"}, true)
	if !apperrors.Is(err, apperrors.ErrDependencyMissing) {
		t.Fatalf("err = %v, want ErrDependencyMissing", err)
	}
}

func TestEnsureStillMissingAfterInstall(t *testing.T) {
	// L'installation "réussit" mais le binaire reste absent.
	f := &fakeCommander{present: map[string]bool{"pacman": true}} // installed vide → toujours absent
	err := newChecker(f).Ensure(context.Background(), io.Discard, []string{"nginx"}, true)
	if !apperrors.Is(err, apperrors.ErrDependencyMissing) {
		t.Fatalf("err = %v, want ErrDependencyMissing", err)
	}
}

func TestInstallCmdPerManager(t *testing.T) {
	tests := []struct {
		mgr  string
		want string
	}{
		{"apt", "sudo apt-get install -y rsync"},
		{"dnf", "sudo dnf install -y rsync"},
		{"pacman", "sudo pacman -S --noconfirm rsync"},
	}
	for _, tt := range tests {
		if got := strings.Join(installCmd(tt.mgr, []string{"rsync"}), " "); got != tt.want {
			t.Errorf("installCmd(%q) = %q, want %q", tt.mgr, got, tt.want)
		}
	}
	if installCmd("unknown", []string{"x"}) != nil {
		t.Error("installCmd(unknown) should be nil")
	}
}

func TestPkgFor(t *testing.T) {
	tests := []struct {
		cmd, mgr, want string
	}{
		{"ssh", "apt", "openssh-client"},
		{"ssh", "dnf", "openssh-clients"},
		{"ssh", "pacman", "openssh"},
		{"machinectl", "apt", "systemd-container"},
		{"unknown-bin", "apt", "unknown-bin"}, // repli sur le nom de commande
	}
	for _, tt := range tests {
		if got := pkgFor(tt.cmd, tt.mgr); got != tt.want {
			t.Errorf("pkgFor(%q,%q) = %q, want %q", tt.cmd, tt.mgr, got, tt.want)
		}
	}
}

func TestDetectManagerOrder(t *testing.T) {
	// apt prioritaire si plusieurs présents.
	f := &fakeCommander{present: map[string]bool{"apt-get": true, "dnf": true}}
	if got := newChecker(f).detectManager(); got != "apt" {
		t.Errorf("detectManager = %q, want apt", got)
	}
	f2 := &fakeCommander{present: map[string]bool{"pacman": true}}
	if got := newChecker(f2).detectManager(); got != "pacman" {
		t.Errorf("detectManager = %q, want pacman", got)
	}
	f3 := &fakeCommander{present: map[string]bool{}}
	if got := newChecker(f3).detectManager(); got != "" {
		t.Errorf("detectManager = %q, want empty", got)
	}
}

func TestSystemCommander(t *testing.T) {
	sc := systemCommander{}
	if !sc.Look("go") {
		t.Error("go should be found on PATH")
	}
	if sc.Look("definitely-not-a-real-binary-xyz") {
		t.Error("nonexistent binary should not be found")
	}
	out, err := sc.Run(context.Background(), []string{"echo", "ok"})
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("Run = %q, %v", out, err)
	}
	if _, err := sc.Run(context.Background(), nil); err == nil {
		t.Error("Run(nil) should error")
	}
}
