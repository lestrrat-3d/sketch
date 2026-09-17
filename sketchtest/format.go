package sketchtest

import (
	"fmt"
	"strings"

	"github.com/lestrrat-3d/sketch"
)

func pointName(point *sketch.Point) string {
	name := fmt.Sprintf("point[%d]", point.ID())
	if point.Name() != "" {
		name += fmt.Sprintf(" %q", point.Name())
	}
	return name
}

func reasonBlock(reasons sketch.Reasons) string {
	if reasons == nil {
		return "0 reason(s)"
	}
	return errorsBlock(reasons.Unwrap())
}

func errorsBlock(items []error) string {
	lines := make([]string, len(items))
	for i, reason := range items {
		lines[i] = fmt.Sprintf("  [%d] %s", i+1, reason)
	}
	return fmt.Sprintf("%d reason(s):\n%s", len(items), strings.Join(lines, "\n"))
}

func pointsText(points []*sketch.Point) string {
	names := make([]string, len(points))
	for i, point := range points {
		if point == nil {
			names[i] = "<nil point>"
			continue
		}
		names[i] = pointName(point)
	}
	return "[" + strings.Join(names, ", ") + "]"
}

func constraintName(constraint sketch.Constraint) string {
	kind := sketch.ConstraintKind(constraint)
	if kind == "" {
		return "constraint"
	}
	return fmt.Sprintf("constraint %q", kind)
}
