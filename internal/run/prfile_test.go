package run

import (
	"os"
	"path/filepath"
	"testing"
)

func writePR(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pull-request.md")
	os.WriteFile(p, []byte(content), 0o644)
	return p
}

func TestParsePRFileOK(t *testing.T) {
	p := writePR(t, "feat: add health endpoint\n\n## Summary\n\nAdds /health.\n")
	title, body, err := ParsePRFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if title != "feat: add health endpoint" || body != "## Summary\n\nAdds /health." {
		t.Fatalf("title=%q body=%q", title, body)
	}
}

func TestParsePRFileErrors(t *testing.T) {
	if _, _, err := ParsePRFile(filepath.Join(t.TempDir(), "absent.md")); err == nil {
		t.Fatal("want error for missing file")
	}
	if _, _, err := ParsePRFile(writePR(t, "\n\n")); err == nil {
		t.Fatal("want error for empty title")
	}
	if _, _, err := ParsePRFile(writePR(t, "feat: only a title\n")); err == nil {
		t.Fatal("want error for empty body — PR body is required")
	}
}
