package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// A worldless zero-value sketch must not panic when its placement is read
// (regression: plane() once returned nil, nil-panicking through MarshalJSON/DXF).
func TestZeroValueSketchPlaneSafe(t *testing.T) {
	var s sketch.Sketch
	require.NotNil(t, s.Plane())
	_, err := json.Marshal(&s)
	require.NoError(t, err)
	_, err = s.DXF()
	require.NoError(t, err)
}
