package sketch

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfileArrangementHardCapsCoverGroupReplayUsage(t *testing.T) {
	require.GreaterOrEqual(t, maxBoundedSketchWork, 1_603_678)
	require.GreaterOrEqual(t, maxBoundedSketchRecords, 19_811)
	require.GreaterOrEqual(t, maxBoundedSketchBytes, 7_679_504)
}
