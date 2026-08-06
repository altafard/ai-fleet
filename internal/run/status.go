package run

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Status is status.json — written once, after all phases have finished,
// for any outcome where the run directory exists.
type Status struct {
	RunID       string `json:"run_id"`
	BaselineRef string `json:"baseline_ref"`
	BaselineSHA string `json:"baseline_sha"`
	Branch      string `json:"branch"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	ImageID     string `json:"image_id"`
	ExitCode    int    `json:"exit_code"`
	CommitCount int    `json:"commit_count"`
	PRURL       string `json:"pr_url,omitempty"`
	Error       string `json:"error,omitempty"`
	StartedAt   string `json:"started_at"`
	FinishedAt  string `json:"finished_at"`
}

// WriteStatus writes s to path as indented JSON with a trailing newline,
// replacing any existing file. The write goes to a temp file in the same
// directory and is renamed into place, so a reader polling the path never
// observes a truncated file.
func WriteStatus(path string, s Status) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".status-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(b, '\n'))
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	// CreateTemp makes the file 0600; open it up to the 0644 the status file
	// has always had before it becomes visible under the real name.
	if werr == nil {
		werr = os.Chmod(tmp.Name(), 0o644)
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil {
		os.Remove(tmp.Name())
	}
	return werr
}
