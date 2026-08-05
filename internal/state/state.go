package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	dirName  = "athena-proxy"
	fileName = "state.json"

	dirPerm fs.FileMode = 0o700
)

type Target struct {
	Hostname  string `json:"hostname"`
	Name      string `json:"name,omitempty"`
	LocalPort int    `json:"localPort"`
}

type State struct {
	ControlPort int      `json:"controlPort"`
	Targets     []Target `json:"targets"`
}

func Dir() (string, error) {
	if override := os.Getenv("ATHENA_PROXY_HOME"); override != "" {
		return override, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config directory: %w", err)
	}
	return filepath.Join(base, dirName), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

func Load() (State, error) {
	path, err := Path()
	if err != nil {
		return State{}, err
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read %s: %w", path, err)
	}

	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return State{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return s, nil
}

func Save(s State) error {
	path, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	if s.Targets == nil {
		s.Targets = []Target{}
	}
	encoded, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	tmp, err := os.CreateTemp(dir, fileName+".*")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := writeAndClose(tmp, encoded); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeAndClose(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write %s: %w", f.Name(), err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync %s: %w", f.Name(), err)
	}
	return f.Close()
}
