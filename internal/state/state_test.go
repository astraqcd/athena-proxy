package state_test

import (
	"os"
	"testing"

	"github.com/astraqcd/athena-proxy/internal/state"
)

func TestLoadReturnsAnEmptyStateWhenNothingIsSaved(t *testing.T) {
	t.Setenv("ATHENA_PROXY_HOME", t.TempDir())

	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ControlPort != 0 || len(loaded.Targets) != 0 {
		t.Fatalf("loaded %+v, want an empty state", loaded)
	}
}

func TestSaveRoundTrips(t *testing.T) {
	t.Setenv("ATHENA_PROXY_HOME", t.TempDir())

	want := state.State{
		ControlPort: 40000,
		Targets: []state.Target{
			{Hostname: "s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz", Name: "pwn", LocalPort: 13370},
			{Hostname: "n1ubqh2xsyb1q00lwgctna75.tcp.challs.ctf-platform.xyz", LocalPort: 13371},
		},
	}
	if err := state.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := state.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ControlPort != want.ControlPort || len(got.Targets) != len(want.Targets) {
		t.Fatalf("loaded %+v, want %+v", got, want)
	}
	for i, target := range got.Targets {
		if target != want.Targets[i] {
			t.Errorf("target %d = %+v, want %+v", i, target, want.Targets[i])
		}
	}
}

func TestSaveOverwritesInPlace(t *testing.T) {
	t.Setenv("ATHENA_PROXY_HOME", t.TempDir())

	if err := state.Save(state.State{ControlPort: 40000}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := state.Save(state.State{ControlPort: 40001}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := state.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ControlPort != 40001 {
		t.Fatalf("control port is %d, want 40001", got.ControlPort)
	}

	dir, err := state.Dir()
	if err != nil {
		t.Fatalf("dir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("state directory holds %d entries, want only state.json", len(entries))
	}
}
