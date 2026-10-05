package main

import (
	"encoding/xml"
	"image"
	"image/gif"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBannerAnimationFramesAreSVG(t *testing.T) {
	static, err := banner()
	require.NoError(t, err)
	animation, err := newBannerAnimation(static)
	require.NoError(t, err)
	for _, index := range []int{0, 16, 48, bannerFrames - 1} {
		var root struct {
			XMLName xml.Name
		}
		require.NoError(t, xml.Unmarshal([]byte(animation.frame(index)), &root))
		require.Equal(t, "svg", root.XMLName.Local)
	}
}

func TestBannerGIF(t *testing.T) {
	path := filepath.Join(imagesDir(), "banner.gif")
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	animation, err := gif.DecodeAll(file)
	require.NoError(t, err)
	require.Equal(t, bannerFrames, len(animation.Image))
	require.Equal(t, 720, animation.Config.Width)
	require.Equal(t, 299, animation.Config.Height)
	require.Equal(t, 0, animation.LoopCount)
	require.Zero(t, bannerBluePixels(animation.Image[0]))
	require.Greater(t, bannerBluePixels(animation.Image[48]), 100)
	require.Zero(t, bannerBluePixels(animation.Image[bannerFrames-1]))
}

func bannerBluePixels(frame image.Image) int {
	count := 0
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			r, g, b, a := frame.At(x, y).RGBA()
			if a > 0 && b > g*3/2 && g > r*2 {
				count++
			}
		}
	}
	return count
}
