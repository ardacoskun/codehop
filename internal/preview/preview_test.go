package preview

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteShowsReadmeSanitized(t *testing.T) {
	dir := t.TempDir()
	readme := "# Title\n\x1b]52;c;ZXZpbA==\x07clipboard\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	Write(context.Background(), &out, dir, time.Now())
	if !strings.Contains(out.String(), "# Title") {
		t.Fatalf("README missing:\n%s", out.String())
	}
	if strings.Contains(out.String(), "\x1b]52") || strings.Contains(out.String(), "\x07") {
		t.Fatalf("escape sequence from README reached the output: %q", out.String())
	}
}
