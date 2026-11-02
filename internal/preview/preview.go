// Package preview renders the short project summary shown in the picker.
package preview

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ardacoskun/codehop/internal/gitstat"
	"github.com/ardacoskun/codehop/internal/paths"
	"github.com/ardacoskun/codehop/internal/safetext"
)

// readmeLines is how much of the README the preview shows.
const readmeLines = 30

var readmeNames = []string{"README.md", "readme.md", "Readme.md", "README", "README.txt", "README.rst"}

// Write prints the path, git state and README head of dir to w.
func Write(ctx context.Context, w io.Writer, dir string, now time.Time) {
	fmt.Fprintf(w, "\x1b[1m%s\x1b[0m\n", safetext.Sanitize(paths.Pretty(dir)))

	if st := gitstat.Inspect(ctx, dir); st.IsRepo && st.Err == nil {
		fmt.Fprintf(w, "\x1b[35m%s\x1b[0m%s\n", safetext.Sanitize(st.Branch), describeState(st))
		if c := st.LastCommit; c != nil {
			fmt.Fprintf(w, "\x1b[33m%s\x1b[0m %s \x1b[2m(%s ago)\x1b[0m\n",
				safetext.Sanitize(c.Hash), safetext.Sanitize(c.Subject), gitstat.Age(c.Time, now))
		}
	}

	readme := findReadme(dir)
	if readme == "" {
		return
	}
	f, err := openRegular(readme)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(w)
	sc := bufio.NewScanner(f)
	for n := 0; n < readmeLines && sc.Scan(); n++ {
		fmt.Fprintln(w, safetext.Sanitize(sc.Text()))
	}
}

func describeState(st gitstat.Status) string {
	var parts []string
	if st.Changed > 0 {
		parts = append(parts, fmt.Sprintf("%d changed", st.Changed))
	}
	if st.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", st.Untracked))
	}
	if st.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", st.Ahead))
	}
	if st.Behind > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", st.Behind))
	}
	if len(parts) == 0 {
		return " \x1b[32m✓ clean\x1b[0m"
	}
	return " \x1b[31m" + strings.Join(parts, ", ") + "\x1b[0m"
}

// findReadme returns the project's README if it is a regular file inside
// the project. A FIFO would block the preview, and a symlink pointing
// elsewhere (README.md -> ~/.ssh/id_rsa) would display an unrelated file.
func findReadme(dir string) string {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return ""
	}
	for _, name := range readmeNames {
		p := filepath.Join(dir, name)
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if rel, err := filepath.Rel(root, resolved); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return resolved
	}
	return ""
}

// openRegular opens path without blocking (a FIFO swapped in after
// findReadme checked it would otherwise hang the preview) and confirms it is
// still a regular file.
func openRegular(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("not a regular file")
	}
	return f, nil
}
