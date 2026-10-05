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
	for _, index := range []int{0, 16, int(bannerHoldStart * bannerFPS),
		int(bannerExitStart * bannerFPS), bannerFrames - 1} {
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
	require.Greater(t, len(animation.Image), 10)
	require.LessOrEqual(t, len(animation.Image), bannerFrames)
	require.Equal(t, 720, animation.Config.Width)
	require.Equal(t, 299, animation.Config.Height)
	require.Equal(t, 0, animation.LoopCount)
	fullBlue := bannerBluePixels(bannerGIFFrameAt(animation, int(bannerHoldStart*100)))
	require.Zero(t, bannerBluePixels(bannerGIFFrameAt(animation, 0)))
	require.Greater(t, fullBlue, 100)
	require.Equal(t, fullBlue, bannerBluePixels(bannerGIFFrameAt(animation, int(bannerExitStart*100)-1)))
	require.Less(t, bannerBluePixels(bannerGIFFrameAt(animation, int((bannerExitStart+0.5)*100))), fullBlue)
	require.Zero(t, bannerBluePixels(animation.Image[len(animation.Image)-1]))
	totalCentiseconds := 0
	for _, delay := range animation.Delay {
		totalCentiseconds += delay
	}
	require.InDelta(t, bannerExitEnd*100, totalCentiseconds, 10)
}

func bannerGIFFrameAt(animation *gif.GIF, centiseconds int) image.Image {
	elapsed := 0
	for i, delay := range animation.Delay {
		elapsed += delay
		if centiseconds < elapsed {
			return animation.Image[i]
		}
	}
	return animation.Image[len(animation.Image)-1]
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
