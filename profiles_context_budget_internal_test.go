package sketch

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfileArrangementHardCapsCoverGroupReplayUsage(t *testing.T) {
	require.GreaterOrEqual(t, maxProfileWork, 1_603_678)
	require.GreaterOrEqual(t, maxProfileRecords, 19_811)
	require.GreaterOrEqual(t, maxProfileBytes, 7_679_504)
}
