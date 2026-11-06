package sketch

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddExportBuilderCapacityIsBounded(t *testing.T) {
	tests := []struct {
		name         string
		capacity     int
		count        int
		bytesPerItem int
		want         int
	}{
		{name: "small estimate", capacity: 256, count: 3, bytesPerItem: 80, want: 496},
		{name: "empty count", capacity: 256, count: 0, bytesPerItem: 80, want: 256},
		{name: "large estimate", capacity: 256, count: math.MaxInt, bytesPerItem: 80, want: maxExportBuilderCapacity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, addExportBuilderCapacity(tt.capacity, tt.count, tt.bytesPerItem))
		})
	}
}
