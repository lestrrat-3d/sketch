package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	bannerFPS    = 16
	bannerFrames = 80
)

// bannerAnimation keeps the frame layout anchored to the SVG rendered by
// Sketch. The clip covers only its blue geometry, leaving the grid, frame and
// provenance footer visible throughout the loop.
type bannerAnimation struct {
	svg          string
	geometryFrom int
	geometryTo   int
	width        float64
	height       float64
}

func newBannerAnimation(svg string) (bannerAnimation, error) {
	marker := `stroke="` + bannerStroke + `"`
	first := strings.Index(svg, marker)
	last := strings.LastIndex(svg, marker)
	if first < 0 {
		return bannerAnimation{}, fmt.Errorf("banner SVG has no blue geometry")
	}
	from := strings.LastIndex(svg[:first], "  <")
	lineEnd := strings.IndexByte(svg[last:], '\n')
	if from < 0 || lineEnd < 0 {
		return bannerAnimation{}, fmt.Errorf("banner SVG geometry is not line-separated")
	}
	to := last + lineEnd + 1
	for _, line := range strings.Split(strings.TrimSuffix(svg[from:to], "\n"), "\n") {
		if !strings.Contains(line, marker) {
			return bannerAnimation{}, fmt.Errorf("banner SVG has non-geometry between blue elements")
		}
	}

	viewBoxStart := strings.Index(svg, `viewBox="`)
	if viewBoxStart < 0 {
		return bannerAnimation{}, fmt.Errorf("banner SVG has no viewBox")
	}
	viewBoxStart += len(`viewBox="`)
	viewBoxEnd := strings.IndexByte(svg[viewBoxStart:], '"')
	if viewBoxEnd < 0 {
		return bannerAnimation{}, fmt.Errorf("banner SVG has an incomplete viewBox")
	}
	var minX, minY, width, height float64
	if _, err := fmt.Sscanf(svg[viewBoxStart:viewBoxStart+viewBoxEnd], "%f %f %f %f",
		&minX, &minY, &width, &height); err != nil {
		return bannerAnimation{}, fmt.Errorf("parse banner viewBox: %w", err)
	}
	if minX != 0 || minY != 0 || width <= 0 || height <= 0 {
		return bannerAnimation{}, fmt.Errorf("unexpected banner viewBox")
	}
	return bannerAnimation{svg: svg, geometryFrom: from, geometryTo: to, width: width, height: height}, nil
}

func bannerProgress(t, start, end float64) float64 {
	u := (t - start) / (end - start)
	if u <= 0 {
		return 0
	}
	if u >= 1 {
		return 1
	}
	return u * u * (3 - 2*u)
}

func (a bannerAnimation) frame(index int) string {
	t := float64(index) / bannerFPS
	exit := 1 - bannerProgress(t, 4, 5)
	word := bannerProgress(t, 0, 1.6) * exit
	tagline := bannerProgress(t, 1.5, 2.4) * exit
	split := a.height * 0.61 // between the wordmark and tagline in banner.go
	clip := fmt.Sprintf(
		"  <defs><clipPath id=\"banner-reveal\">"+
			"<rect x=\"0\" y=\"0\" width=\"%.4f\" height=\"%.4f\"/>"+
			"<rect x=\"0\" y=\"%.4f\" width=\"%.4f\" height=\"%.4f\"/>"+
			"</clipPath></defs>\n",
		a.width*word, split, split, a.width*tagline, a.height-split,
	)
	return a.svg[:a.geometryFrom] + clip +
		"  <g clip-path=\"url(#banner-reveal)\">\n" +
		a.svg[a.geometryFrom:a.geometryTo] + "  </g>\n" + a.svg[a.geometryTo:]
}

// renderBannerGIF writes a looping README image from the same Sketch-generated
// SVG as banner.svg. ffmpeg needs SVG decoding support (librsvg).
func renderBannerGIF(out string) error {
	static, err := banner()
	if err != nil {
		return err
	}
	animation, err := newBannerAnimation(static)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(".tmp", 0o755); err != nil {
		return fmt.Errorf("create scratch directory: %w", err)
	}
	framesDir, err := os.MkdirTemp(".tmp", "banner-frames-")
	if err != nil {
		return fmt.Errorf("create frame directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(framesDir) }()
	for i := range bannerFrames {
		path := filepath.Join(framesDir, fmt.Sprintf("frame_%04d.svg", i))
		if err := os.WriteFile(path, []byte(animation.frame(i)), 0o644); err != nil {
			return fmt.Errorf("write banner frame %d: %w", i, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("create GIF directory: %w", err)
	}
	filter := "[0:v]split[a][b];[a]palettegen=max_colors=64[p];[b][p]paletteuse[v]"
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-framerate", strconv.Itoa(bannerFPS),
		"-i", filepath.Join(framesDir, "frame_%04d.svg"), "-filter_complex", filter,
		"-map", "[v]", "-loop", "0", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("encode banner GIF with ffmpeg: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
