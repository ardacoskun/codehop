// Package paths resolves the per-platform locations codehop reads and writes,
// and expands user-supplied paths such as "~/projects".
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const appName = "codehop"

// ConfigFile returns the config file path. CODEHOP_CONFIG wins; otherwise
// $XDG_CONFIG_HOME/codehop/config.toml (~/.config on macOS and Linux) or
// %APPDATA%\codehop\config.toml on Windows.
func ConfigFile() string {
	if p := os.Getenv("CODEHOP_CONFIG"); p != "" {
		return Expand(p)
	}
	return filepath.Join(baseDir("XDG_CONFIG_HOME", ".config", "APPDATA"), appName, "config.toml")
}

// DataDir returns the directory holding the frecency database.
// CODEHOP_DATA_DIR wins; otherwise $XDG_DATA_HOME/codehop or %LOCALAPPDATA%\codehop.
func DataDir() string {
	if p := os.Getenv("CODEHOP_DATA_DIR"); p != "" {
		return Expand(p)
	}
	return filepath.Join(baseDir("XDG_DATA_HOME", filepath.Join(".local", "share"), "LOCALAPPDATA"), appName)
}

// CacheDir returns the directory holding the project discovery cache.
// CODEHOP_CACHE_DIR wins; otherwise $XDG_CACHE_HOME/codehop or %LOCALAPPDATA%\codehop\cache.
func CacheDir() string {
	if p := os.Getenv("CODEHOP_CACHE_DIR"); p != "" {
		return Expand(p)
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(baseDir("", "", "LOCALAPPDATA"), appName, "cache")
	}
	return filepath.Join(baseDir("XDG_CACHE_HOME", ".cache", ""), appName)
}

// baseDir picks the XDG variable on Unix-likes (falling back to ~/unixDefault)
// and the given environment variable on Windows (falling back to the home dir).
func baseDir(xdgVar, unixDefault, windowsVar string) string {
	if runtime.GOOS == "windows" {
		if v := os.Getenv(windowsVar); v != "" {
			return v
		}
		return home()
	}
	if v := os.Getenv(xdgVar); v != "" && filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(home(), unixDefault)
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// Expand replaces a leading "~" with the home directory, expands environment
// variables ($VAR, ${VAR} and, on Windows, %VAR%) and cleans the result.
func Expand(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p == "~" {
		return home()
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		p = filepath.Join(home(), p[2:])
	}
	if runtime.GOOS == "windows" {
		p = expandPercentVars(p)
	}
	return filepath.Clean(os.ExpandEnv(p))
}

// expandPercentVars expands Windows-style %VAR% references; unknown ones are kept.
func expandPercentVars(p string) string {
	var b strings.Builder
	for {
		start := strings.IndexByte(p, '%')
		if start < 0 {
			break
		}
		end := strings.IndexByte(p[start+1:], '%')
		if end < 0 {
			break
		}
		name := p[start+1 : start+1+end]
		b.WriteString(p[:start])
		if v, ok := os.LookupEnv(name); ok && name != "" {
			b.WriteString(v)
		} else {
			b.WriteString(p[start : start+end+2])
		}
		p = p[start+end+2:]
	}
	b.WriteString(p)
	return b.String()
}

// Pretty shortens a path under the home directory to "~/..." for display.
func Pretty(p string) string {
	h := home()
	if h == "" || h == "." {
		return p
	}
	if p == h {
		return "~"
	}
	if rel, ok := strings.CutPrefix(p, h+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rel
	}
	return p
}

// WriteFileAtomic writes data to a temp file in the target directory and
// renames it into place, so readers never observe a partial file.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	// Config, history and cache are private to the user.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
