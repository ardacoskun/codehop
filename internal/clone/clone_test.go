package clone

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		ok   bool
		url  string
		name string
	}{
		{"gh:owner/repo", true, "https://github.com/owner/repo.git", "repo"},
		{"gh:owner/repo.git", true, "https://github.com/owner/repo.git", "repo"},
		{"gl:group/sub/repo", true, "https://gitlab.com/group/sub/repo.git", "repo"},
		{"https://github.com/owner/repo", true, "https://github.com/owner/repo", "repo"},
		{"https://example.com/x/repo.git", true, "https://example.com/x/repo.git", "repo"},
		{"git@github.com:owner/repo.git", true, "git@github.com:owner/repo.git", "repo"},
		{"ssh://git@host/owner/repo.git", true, "ssh://git@host/owner/repo.git", "repo"},
		{"https://example.com/page", false, "", ""},
		{"https://github.com/owner", false, "", ""},
		{"gh:", false, "", ""},
		{"gh:owner", false, "", ""},
		{"color-shuffle", false, "", ""},
		{"notes.md", false, "", ""},
		{"C:/Users/me", false, "", ""},
		{"gh:owner/..", false, "", ""},
		{"gh:owner/.hidden", false, "", ""},
		{`https://h/o/..\\..\\evil.git`, false, "", ""},
		{"https://h/o/..git", false, "", ""},
	}
	for _, tt := range tests {
		r, ok := Parse(tt.in)
		if ok != tt.ok || r.URL != tt.url || r.Name != tt.name {
			t.Errorf("Parse(%q) = %+v, %v; want url=%q name=%q ok=%v", tt.in, r, ok, tt.url, tt.name, tt.ok)
		}
	}
}

func TestDestinationStaysInRoot(t *testing.T) {
	root := t.TempDir()
	if _, err := (Remote{Name: "repo"}).Destination(root); err != nil {
		t.Fatalf("plain name rejected: %v", err)
	}
	for _, name := range []string{"..", "../x", `..\\x`, "a/b"} {
		if dest, err := (Remote{Name: name}).Destination(root); err == nil {
			t.Errorf("Destination(%q) = %s, want error", name, dest)
		}
	}
}

func TestDisplayHidesCredentials(t *testing.T) {
	r := Remote{URL: "https://user:ghp_secret@github.com/o/r.git"}
	if got := r.Display(); got != "https://github.com/o/r.git" {
		t.Errorf("Display = %q", got)
	}
}

func TestIdentity(t *testing.T) {
	same := []string{
		"https://github.com/Owner/Repo.git",
		"https://user@github.com/owner/repo",
		"git@github.com:owner/repo.git",
		"ssh://git@github.com/owner/repo.git\n",
	}
	for _, u := range same {
		if identity(u) != "github.com/owner/repo" {
			t.Errorf("identity(%q) = %q", u, identity(u))
		}
	}
	if identity("https://github.com/other/repo") == identity(same[0]) {
		t.Error("different repositories compare equal")
	}
}
