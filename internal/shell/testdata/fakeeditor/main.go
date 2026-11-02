// Command fakeeditor records its arguments, one call per line, in the file
// named by FAKE_EDITOR_LOG. The shell integration tests use it as the editor.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	f, err := os.OpenFile(os.Getenv("FAKE_EDITOR_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer f.Close()
	fmt.Fprintln(f, strings.Join(os.Args[1:], " "))
}
