package cgroups

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// cgroupFiles décrit le contenu des fichiers cgroup d'un conteneur factice.
type cgroupFiles struct {
	memoryCurrent string
	memoryMax     string
	cpuStat       string
	pidsCurrent   string
	pidsMax       string
}

// writeTree crée un arbre cgroup factice pour name sous une racine temporaire
// et renvoie cette racine, prête à être passée à WithRoot.
func writeTree(t *testing.T, name string, f cgroupFiles) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, defaultSlice, config.UnitName(name)+unitSuffix)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir cgroup tree: %v", err)
	}
	files := map[string]string{
		"memory.current": f.memoryCurrent,
		"memory.max":     f.memoryMax,
		"cpu.stat":       f.cpuStat,
		"pids.current":   f.pidsCurrent,
		"pids.max":       f.pidsMax,
	}
	for name, content := range files {
		if content == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

// validTree renvoie un arbre cohérent et complet.
func validFiles() cgroupFiles {
	return cgroupFiles{
		memoryCurrent: "134217728\n", // 128 MiB
		memoryMax:     "536870912\n", // 512 MiB
		cpuStat:       "usage_usec 5000000\nuser_usec 3000000\nsystem_usec 2000000\n",
		pidsCurrent:   "42\n",
		pidsMax:       "256\n",
	}
}

func TestReadParsesAllCounters(t *testing.T) {
	root := writeTree(t, "user01", validFiles())
	r := NewReader(WithRoot(root))

	s, err := r.Read(context.Background(), "user01")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if s.MemoryCurrentBytes != 134217728 {
		t.Errorf("MemoryCurrentBytes = %d, want 134217728", s.MemoryCurrentBytes)
	}
	if s.MemoryMaxBytes != 536870912 {
		t.Errorf("MemoryMaxBytes = %d, want 536870912", s.MemoryMaxBytes)
	}
	if s.CPUUsageUsec != 5000000 {
		t.Errorf("CPUUsageUsec = %d, want 5000000", s.CPUUsageUsec)
	}
	if s.PidsCurrent != 42 {
		t.Errorf("PidsCurrent = %d, want 42", s.PidsCurrent)
	}
	if s.PidsMax != 256 {
		t.Errorf("PidsMax = %d, want 256", s.PidsMax)
	}
	if s.ReadAt.IsZero() {
		t.Error("ReadAt not set")
	}
}

func TestReadNotRunning(t *testing.T) {
	// Arbre vide : aucun répertoire pour le conteneur.
	r := NewReader(WithRoot(t.TempDir()))
	_, err := r.Read(context.Background(), "ghost")
	if !apperrors.Is(err, apperrors.ErrNotRunning) {
		t.Fatalf("err = %v, want ErrNotRunning", err)
	}
}

func TestReadUnlimitedLimits(t *testing.T) {
	f := validFiles()
	f.memoryMax = "max\n"
	f.pidsMax = "max\n"
	r := NewReader(WithRoot(writeTree(t, "lim", f)))

	s, err := r.Read(context.Background(), "lim")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if s.MemoryMaxBytes != 0 {
		t.Errorf("MemoryMaxBytes = %d, want 0 (unlimited)", s.MemoryMaxBytes)
	}
	if s.PidsMax != 0 {
		t.Errorf("PidsMax = %d, want 0 (unlimited)", s.PidsMax)
	}
}

func TestReadCancelledContext(t *testing.T) {
	r := NewReader(WithRoot(writeTree(t, "ctx", validFiles())))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Read(ctx, "ctx"); !apperrors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestReadMalformed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*cgroupFiles)
	}{
		{"bad memory.current", func(f *cgroupFiles) { f.memoryCurrent = "abc\n" }},
		{"bad memory.max", func(f *cgroupFiles) { f.memoryMax = "huge\n" }},
		{"bad pids.current", func(f *cgroupFiles) { f.pidsCurrent = "x\n" }},
		{"cpu.stat without usage_usec", func(f *cgroupFiles) { f.cpuStat = "user_usec 1\n" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := validFiles()
			tt.mutate(&f)
			r := NewReader(WithRoot(writeTree(t, "bad", f)))
			if _, err := r.Read(context.Background(), "bad"); err == nil {
				t.Fatal("Read = nil, want error")
			}
		})
	}
}

func TestCPUPercent(t *testing.T) {
	base := time.Now()
	tests := []struct {
		name string
		prev Stats
		cur  Stats
		want float64
	}{
		{
			name: "full core",
			prev: Stats{CPUUsageUsec: 0, ReadAt: base},
			cur:  Stats{CPUUsageUsec: 1_000_000, ReadAt: base.Add(time.Second)},
			want: 100,
		},
		{
			name: "half core",
			prev: Stats{CPUUsageUsec: 0, ReadAt: base},
			cur:  Stats{CPUUsageUsec: 500_000, ReadAt: base.Add(time.Second)},
			want: 50,
		},
		{
			name: "two cores",
			prev: Stats{CPUUsageUsec: 0, ReadAt: base},
			cur:  Stats{CPUUsageUsec: 2_000_000, ReadAt: base.Add(time.Second)},
			want: 200,
		},
		{
			name: "no wall time",
			prev: Stats{CPUUsageUsec: 0, ReadAt: base},
			cur:  Stats{CPUUsageUsec: 1_000_000, ReadAt: base},
			want: 0,
		},
		{
			name: "counter reset",
			prev: Stats{CPUUsageUsec: 1_000_000, ReadAt: base},
			cur:  Stats{CPUUsageUsec: 0, ReadAt: base.Add(time.Second)},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CPUPercent(tt.prev, tt.cur); got != tt.want {
				t.Errorf("CPUPercent() = %.2f, want %.2f", got, tt.want)
			}
		})
	}
}

func TestSampleSingleSnapshot(t *testing.T) {
	r := NewReader(WithRoot(writeTree(t, "snap", validFiles())))
	s, cpu, err := r.Sample(context.Background(), "snap", 0)
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if cpu != 0 {
		t.Errorf("cpu = %.2f, want 0 (no interval)", cpu)
	}
	if s.PidsCurrent != 42 {
		t.Errorf("PidsCurrent = %d, want 42", s.PidsCurrent)
	}
}

func TestSampleStaticUsage(t *testing.T) {
	r := NewReader(WithRoot(writeTree(t, "static", validFiles())))
	// Les fichiers ne changent pas : usage CPU constant → 0%.
	_, cpu, err := r.Sample(context.Background(), "static", 5*time.Millisecond)
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if cpu != 0 {
		t.Errorf("cpu = %.2f, want 0 (static usage)", cpu)
	}
}

func TestSampleContextCancelledDuringInterval(t *testing.T) {
	r := NewReader(WithRoot(writeTree(t, "cancel", validFiles())))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	// Premier Read OK, puis l'attente dépasse la deadline.
	if _, _, err := r.Sample(ctx, "cancel", 500*time.Millisecond); !apperrors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}
