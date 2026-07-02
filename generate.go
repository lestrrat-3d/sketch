package sketch

// Regenerate the embedded code blocks in README.md from the compiled example
// tests. Run `go generate ./...` (or the command below directly) after changing
// any example referenced by an <!-- INCLUDE(...) --> directive.
//go:generate go run ./internal/cmd/genreadme README.md

// Regenerate the annotated SVG hero images embedded in the README gallery. The
// genimages in-sync test fails if the committed files drift from this output.
//go:generate go run ./internal/cmd/genimages docs/images
