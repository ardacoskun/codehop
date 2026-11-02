package editor

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxWorkspaceSize bounds how much of a .code-workspace file is read.
const maxWorkspaceSize = 1 << 20

type workspaceFile struct {
	Folders []struct {
		Path string `json:"path"`
		URI  string `json:"uri"`
	} `json:"folders"`
}

// checkWorkspace returns nil when every folder the workspace file opens lies
// inside dir. A workspace file comes with the project, so it could otherwise
// open your home folder or anything else in the editor's workspace.
func checkWorkspace(file, dir string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxWorkspaceSize+1))
	if err != nil {
		return err
	}
	if len(data) > maxWorkspaceSize {
		return fmt.Errorf("%s is larger than %d bytes", filepath.Base(file), maxWorkspaceSize)
	}
	var ws workspaceFile
	if err := json.Unmarshal(stripJSONC(data), &ws); err != nil {
		return fmt.Errorf("%s is not valid JSON: %w", filepath.Base(file), err)
	}
	if len(ws.Folders) == 0 {
		return fmt.Errorf("%s lists no folders", filepath.Base(file))
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	for _, folder := range ws.Folders {
		if folder.URI != "" || folder.Path == "" {
			return fmt.Errorf("%s opens a folder by URI", filepath.Base(file))
		}
		p := filepath.FromSlash(folder.Path)
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(file), p)
		}
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		if !within(root, filepath.Clean(p)) {
			return fmt.Errorf("%s opens %q, outside the project", filepath.Base(file), folder.Path)
		}
	}
	return nil
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// stripJSONC turns VS Code's JSON-with-comments into plain JSON: it removes
// // and /* */ comments and trailing commas, leaving string contents intact.
func stripJSONC(in []byte) []byte {
	out := make([]byte, 0, len(in))
	inString, escaped := false, false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(in) && in[i+1] == '/':
			for i < len(in) && in[i] != '\n' {
				i++
			}
			if i < len(in) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(in) && in[i+1] == '*':
			i += 2
			for i+1 < len(in) && (in[i] != '*' || in[i+1] != '/') {
				i++
			}
			i++
		case c == ']' || c == '}':
			// Drop a trailing comma before the closing bracket.
			j := len(out) - 1
			for j >= 0 && (out[j] == ' ' || out[j] == '\t' || out[j] == '\n' || out[j] == '\r') {
				j--
			}
			if j >= 0 && out[j] == ',' {
				out = append(out[:j], out[j+1:]...)
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}
