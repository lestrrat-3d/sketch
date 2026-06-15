// Command genreadme expands embedded-code directives in Markdown files in
// place, the way lestrrat-go/jwx keeps its README code blocks in sync with
// real, compiled example tests.
//
// In a Markdown source, mark a region with:
//
//	<!-- INCLUDE(examples/foo_example_test.go) -->
//	<!-- END INCLUDE -->
//
// Running the tool replaces everything between the two markers with a fenced
// Go code block containing the referenced file, followed by a source link.
// Re-running is idempotent: the markers stay in place, so the README itself is
// the source of truth and a human or an agent can regenerate it at any time
// with no network access and no CI:
//
//	go generate ./...                    # via the //go:generate directive
//	go run ./internal/cmd/genreadme README.md
//
// An optional second argument names a single top-level function to embed
// instead of the whole file, so several focused snippets can be drawn from one
// compiled, go test-verified example file:
//
//	<!-- INCLUDE(examples/foo_example_test.go,Example_sketch_quickstart) -->
//	<!-- END INCLUDE -->
//
// Paths are resolved relative to the directory of the Markdown file being
// processed. Leading tabs are converted to two spaces each so the embedded
// code renders without runaway indentation.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// includeRe matches an INCLUDE directive: a file path and an optional
// comma-separated function name.
var includeRe = regexp.MustCompile(`^<!-- INCLUDE\(([^,)]+)(?:,([^)]+))?\) -->$`)

const endMarker = "<!-- END INCLUDE -->"

func main() {
	files := os.Args[1:]
	if len(files) == 0 {
		files = []string{"README.md"}
	}
	for _, f := range files {
		if err := process(f); err != nil {
			fmt.Fprintf(os.Stderr, "genreadme: %s: %s\n", f, err)
			os.Exit(1)
		}
	}
}

// process rewrites a single Markdown file in place, expanding every INCLUDE
// region it contains.
func process(mdPath string) error {
	src, err := os.ReadFile(mdPath)
	if err != nil {
		return err
	}
	baseDir := filepath.Dir(mdPath)

	var out bytes.Buffer
	lines := strings.Split(string(src), "\n")
	inFence := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// A directive inside a ```-delimited code block is literal text (e.g.
		// documentation showing the directive syntax), never expanded.
		var m []string
		if !inFence {
			m = includeRe.FindStringSubmatch(line)
		}
		if m == nil {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
			}
			out.WriteString(line)
			if i < len(lines)-1 {
				out.WriteByte('\n')
			}
			continue
		}

		incPath, fn := m[1], m[2]
		block, err := render(baseDir, incPath, fn)
		if err != nil {
			return err
		}
		out.WriteString(line)
		out.WriteByte('\n')
		out.WriteString(block)
		out.WriteString(endMarker)
		out.WriteByte('\n')

		// Skip the previously generated region up to and including END INCLUDE.
		for i++; i < len(lines); i++ {
			if strings.TrimRight(lines[i], "\r") == endMarker {
				break
			}
		}
		if i == len(lines) {
			return fmt.Errorf("unterminated INCLUDE for %q: missing %s", incPath, endMarker)
		}
	}

	return os.WriteFile(mdPath, out.Bytes(), 0o644)
}

// render produces the fenced code block and source link for one directive.
func render(baseDir, incPath, fn string) (string, error) {
	full := filepath.Join(baseDir, incPath)
	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("include %q: %w", incPath, err)
	}

	code := string(data)
	if fn != "" {
		code, err = extractFunc(full, data, fn)
		if err != nil {
			return "", err
		}
	}
	code = detab(strings.TrimRight(code, "\n"))

	var b strings.Builder
	b.WriteString("```go\n")
	b.WriteString(code)
	b.WriteString("\n```\n")
	fmt.Fprintf(&b, "source: [%s](%s)\n", incPath, incPath)
	return b.String(), nil
}

// extractFunc returns the source of a single top-level function, including any
// preceding doc comment.
func extractFunc(path string, data []byte, name string) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, parser.ParseComments)
	if err != nil {
		return "", err
	}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != name {
			continue
		}
		start := fd.Pos()
		if fd.Doc != nil {
			start = fd.Doc.Pos()
		}
		lo := fset.Position(start).Offset
		hi := fset.Position(fd.End()).Offset
		return string(data[lo:hi]), nil
	}
	return "", fmt.Errorf("function %q not found in %s", name, path)
}

// detab replaces leading tabs on each line with two spaces apiece, matching the
// jwx autodoc convention so embedded code keeps a sane indentation width.
func detab(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		n := 0
		for n < len(line) && line[n] == '\t' {
			n++
		}
		if n > 0 {
			lines[i] = strings.Repeat("  ", n) + line[n:]
		}
	}
	return strings.Join(lines, "\n")
}
