package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// LocalPath is the project-scope config file — the existing registration
// file; config only ever touches its own sections in it.
func LocalPath(root string) string {
	return filepath.Join(root, ".ai-fleet", "ai-fleet.ini")
}

// GlobalPath is the user-scope config file. Same name and format as the
// local file, minus [project].
func GlobalPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ai-fleet", "ai-fleet.ini"), nil
}

// Load reads a scope. A missing file is an empty scope, not an error.
func Load(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	m, err := ParseOwned(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// Set edits one key and writes the file back atomically at 0600 — it may
// hold a token, and a reader polling it must never see a partial file.
func Set(path, key, value string) error {
	return rewrite(path, func(content string) string { return SetLine(content, key, value) })
}

// Remove deletes one key, writing back atomically.
func Remove(path, key string) error {
	return rewrite(path, func(content string) string { return RemoveLine(content, key) })
}

func rewrite(path string, edit func(string) string) error {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	_, werr := tmp.WriteString(edit(string(b)))
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp.Name(), 0o600)
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil {
		os.Remove(tmp.Name())
	}
	return werr
}
