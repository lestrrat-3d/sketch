package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// imagesDir is the committed gallery directory, relative to this package.
func imagesDir() string { return filepath.Join("..", "..", "..", "docs", "images") }

// TestImagesInSync regenerates every hero and byte-compares it against the
// committed file, so an example change that alters an image fails CI until the
// author reruns `go generate ./...`.
func TestImagesInSync(t *testing.T) {
	images, err := build()
	require.NoError(t, err)
	require.NotEmpty(t, images)

	dir := imagesDir()
	for name, want := range images {
		got, err := os.ReadFile(filepath.Join(dir, name+".svg"))
		require.NoErrorf(t, err, "missing committed image %s.svg; run: go generate ./...", name)
		require.Equalf(t, want, string(got), "%s.svg is out of date; run: go generate ./...", name)
	}

	// No orphan committed images (every .svg has a builder).
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".svg")
		_, ok := images[base]
		require.Truef(t, ok, "orphan committed image %s has no builder", e.Name())
	}
}

// TestImagesWellFormed checks every generated hero parses as XML.
func TestImagesWellFormed(t *testing.T) {
	images, err := build()
	require.NoError(t, err)
	for name, svg := range images {
		require.NotEmpty(t, svg, name)
		require.NoErrorf(t, xml.Unmarshal([]byte(svg), new(struct {
			XMLName xml.Name
		})), "%s is not well-formed XML", name)
		require.NotContainsf(t, svg, "NaN", "%s contains NaN", name)
		require.NotContainsf(t, svg, "Inf", "%s contains Inf", name)
	}
}
