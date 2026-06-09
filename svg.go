package sketch

import (
	"fmt"
	"math"
	"strings"
)

// SVGOptions controls SVG rendering.
type SVGOptions struct {
	Margin       float64 // blank border around the geometry, in sketch units
	StrokeWidth  float64 // stroke width in sketch units
	ShowPoints   bool    // draw a small marker at each point
	PointRadius  float64 // marker radius in sketch units
	ArcSegments  int     // polyline segments used to approximate an arc
	Background   string  // background fill (e.g. "white"); empty for none
	Stroke       string  // geometry color
	Construction string  // construction-geometry color
}

// DefaultSVGOptions returns sensible SVG rendering defaults.
func DefaultSVGOptions() SVGOptions {
	return SVGOptions{
		Margin:       10,
		StrokeWidth:  1,
		ShowPoints:   true,
		PointRadius:  2,
		ArcSegments:  64,
		Background:   "white",
		Stroke:       "#1a73e8",
		Construction: "#bbbbbb",
	}
}

type bbox struct{ minX, minY, maxX, maxY float64 }

func (b *bbox) add(x, y float64) {
	b.minX, b.minY = math.Min(b.minX, x), math.Min(b.minY, y)
	b.maxX, b.maxY = math.Max(b.maxX, x), math.Max(b.maxY, y)
}

// bounds returns the axis-aligned bounding box of all geometry. ok is false
// when the sketch has nothing to draw.
func (s *Sketch) bounds() (bbox, bool) {
	b := bbox{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	any := false
	for _, p := range s.points {
		b.add(p.x(), p.y())
		any = true
	}
	for _, e := range s.ents {
		switch t := e.(type) {
		case *Circle:
			b.add(t.Center.x()-t.r(), t.Center.y()-t.r())
			b.add(t.Center.x()+t.r(), t.Center.y()+t.r())
			any = true
		case *Arc:
			for _, p := range arcPolyline(t, 32) {
				b.add(p[0], p[1])
			}
			any = true
		}
	}
	return b, any
}

// arcPolyline samples the arc counter-clockwise from start to end.
func arcPolyline(a *Arc, segments int) [][2]float64 {
	if segments < 2 {
		segments = 2
	}
	cx, cy := a.Center.x(), a.Center.y()
	r := a.R()
	start := a.StartAngle()
	sweep := a.Sweep()
	pts := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		ang := start + sweep*float64(i)/float64(segments)
		pts[i] = [2]float64{cx + r*math.Cos(ang), cy + r*math.Sin(ang)}
	}
	return pts
}

// SVG renders the sketch to an SVG document. The y-axis is flipped so the
// output matches conventional math orientation (y up).
func (s *Sketch) SVG(opts SVGOptions) (string, error) {
	b, ok := s.bounds()
	if !ok {
		b = bbox{0, 0, 1, 1}
	}
	w := (b.maxX - b.minX) + 2*opts.Margin
	h := (b.maxY - b.minY) + 2*opts.Margin
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}

	// Map sketch coords to SVG coords (flip y).
	tx := func(x float64) float64 { return x - b.minX + opts.Margin }
	ty := func(y float64) float64 { return b.maxY - y + opts.Margin }

	var sb strings.Builder
	fmt.Fprintf(&sb,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s">`,
		f(w), f(h), f(w), f(h))
	sb.WriteByte('\n')
	if opts.Background != "" {
		fmt.Fprintf(&sb, `  <rect width="100%%" height="100%%" fill="%s"/>`+"\n", opts.Background)
	}

	color := func(construction bool) string {
		if construction {
			return opts.Construction
		}
		return opts.Stroke
	}
	dash := func(construction bool) string {
		if construction {
			return fmt.Sprintf(` stroke-dasharray="%s,%s"`, f(opts.StrokeWidth*4), f(opts.StrokeWidth*3))
		}
		return ""
	}

	for _, e := range s.ents {
		switch t := e.(type) {
		case *Line:
			fmt.Fprintf(&sb,
				`  <line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"%s/>`+"\n",
				f(tx(t.Start.x())), f(ty(t.Start.y())), f(tx(t.End.x())), f(ty(t.End.y())),
				color(t.isConstruction()), f(opts.StrokeWidth), dash(t.isConstruction()))
		case *Circle:
			fmt.Fprintf(&sb,
				`  <circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				f(tx(t.Center.x())), f(ty(t.Center.y())), f(t.r()),
				color(t.isConstruction()), f(opts.StrokeWidth), dash(t.isConstruction()))
		case *Arc:
			pts := arcPolyline(t, opts.ArcSegments)
			var d strings.Builder
			for i, p := range pts {
				cmd := "L"
				if i == 0 {
					cmd = "M"
				}
				fmt.Fprintf(&d, "%s%s %s ", cmd, f(tx(p[0])), f(ty(p[1])))
			}
			fmt.Fprintf(&sb,
				`  <path d="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				strings.TrimSpace(d.String()), color(t.isConstruction()), f(opts.StrokeWidth), dash(t.isConstruction()))
		}
	}

	if opts.ShowPoints {
		for _, p := range s.points {
			fill := "#d93025"
			if p.IsFixed() {
				fill = "#202124"
			}
			fmt.Fprintf(&sb,
				`  <circle cx="%s" cy="%s" r="%s" fill="%s"/>`+"\n",
				f(tx(p.x())), f(ty(p.y())), f(opts.PointRadius), fill)
		}
	}

	sb.WriteString("</svg>\n")
	return sb.String(), nil
}

// f formats a float compactly without a trailing ".000000".
func f(v float64) string { return trimFloat(v, 4) }

// trimFloat formats v with prec decimals and drops trailing zeros (and a bare
// trailing decimal point).
func trimFloat(v float64, prec int) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.*f", prec, v), "0"), ".")
}
