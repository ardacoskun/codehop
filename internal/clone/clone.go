// Package clone recognizes remote repository references and clones them.
package clone

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ardacoskun/codehop/internal/safetext"
)

// shorthands map "prefix:owner/repo" to a clone URL template.
var shorthands = map[string]string{
	"gh": "https://github.com/%s.git",
	"gl": "https://gitlab.com/%s.git",
	"bb": "https://bitbucket.org/%s.git",
	"cb": "https://codeberg.org/%s.git",
}

// knownHosts are hosts whose https URLs are treated as repositories even
// without a ".git" suffix.
var knownHosts = []string{"github.com", "gitlab.com", "bitbucket.org", "codeberg.org"}

var (
	ownerRepo = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)+$`)
	scpLike   = regexp.MustCompile(`^[A-Za-z0-9_.-]+@([A-Za-z0-9_.-]+):([^\s]+)$`)
)

// Remote is a parsed remote reference.
type Remote struct {
	// URL is what git clones.
	URL string
	// Name is the directory name the clone gets (the repository name).
	Name string
}

// Parse recognizes "gh:owner/repo" style shorthands, git@host:owner/repo
// (scp-like) URLs, ssh:// and git:// URLs, and https URLs that end in .git
// or point at a known forge. ok is false for anything else.
func Parse(ref string) (Remote, bool) {
	ref = strings.TrimSpace(ref)
	if prefix, rest, found := strings.Cut(ref, ":"); found {
		if tmpl, isShorthand := shorthands[prefix]; isShorthand {
			rest = strings.TrimSuffix(strings.Trim(rest, "/"), ".git")
			name := repoName(rest)
			if !ownerRepo.MatchString(rest) || name == "" {
				return Remote{}, false
			}
			return Remote{URL: fmt.Sprintf(tmpl, rest), Name: name}, true
		}
	}
	switch {
	case scpLike.MatchString(ref),
		strings.HasPrefix(ref, "ssh://"),
		strings.HasPrefix(ref, "git://"),
		strings.HasPrefix(ref, "https://") && isRepoURL(ref):
		name := repoName(ref)
		if name == "" {
			return Remote{}, false
		}
		return Remote{URL: ref, Name: name}, true
	}
	return Remote{}, false
}

func isRepoURL(u string) bool {
	if strings.HasSuffix(u, ".git") {
		return true
	}
	rest := strings.TrimPrefix(u, "https://")
	host, path, _ := strings.Cut(rest, "/")
	for _, h := range knownHosts {
		if host == h {
			return strings.Count(strings.Trim(path, "/"), "/") >= 1
		}
	}
	return false
}

func repoName(ref string) string {
	ref = strings.TrimSuffix(strings.TrimRight(ref, "/"), ".git")
	if i := strings.LastIndexAny(ref, "/:"); i >= 0 {
		ref = ref[i+1:]
	}
	if !validName(ref) {
		return ""
	}
	return ref
}

// validName accepts a repository name as a single, plain folder name: no
// separators of any platform, no drive letters, no leading dot ("..",
// ".hidden") and no control characters.
func validName(name string) bool {
	return name != "" && !strings.HasPrefix(name, ".") &&
		!strings.ContainsAny(name, `/\:`) && !safetext.HasUnsafe(name)
}

// Destination returns where r is cloned under root, and fails if that would
// escape root.
func (r Remote) Destination(root string) (string, error) {
	dest := filepath.Join(root, r.Name)
	rel, err := filepath.Rel(root, dest)
	if !validName(r.Name) || err != nil || rel != r.Name {
		return "", fmt.Errorf("refusing to clone %q outside %s", r.Name, root)
	}
	return dest, nil
}

// Display is the URL with any user:password@ part removed, for messages.
func (r Remote) Display() string {
	u, err := url.Parse(r.URL)
	if err != nil || u.Scheme == "" || u.User == nil {
		return r.URL
	}
	u.User = nil
	return u.String()
}

// identity reduces a remote URL to "host/owner/repo" (lower-cased, no
// scheme, user or .git) so https and ssh forms of one repository compare equal.
func identity(raw string) string {
	raw = strings.TrimSpace(raw)
	if m := scpLike.FindStringSubmatch(raw); m != nil {
		raw = m[1] + "/" + m[2]
	} else if u, err := url.Parse(raw); err == nil && u.Host != "" {
		raw = u.Hostname() + u.Path
	}
	raw = strings.TrimSuffix(strings.TrimRight(raw, "/"), ".git")
	return strings.ToLower(raw)
}

// IsCloneOf reports whether dir is a git repository whose origin is r.
// Reading the remote URL executes nothing from the repository.
func IsCloneOf(dir string, r Remote) bool {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	return err == nil && identity(string(out)) == identity(r.URL)
}

// Clone runs `git clone` into dest. Authentication is left to git's own
// credential helpers; nothing secret is placed on the command line.
func Clone(r Remote, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", "--", r.URL, dest)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", r.Display(), err)
	}
	return nil
}
