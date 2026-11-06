package sketch

const maxExportBuilderCapacity = 1 << 20

// addExportBuilderCapacity adds a count-based estimate without allowing the
// initial allocation to exceed the exporter's fixed one-megabyte limit.
func addExportBuilderCapacity(capacity, count, bytesPerItem int) int {
	if count <= 0 || bytesPerItem <= 0 || capacity >= maxExportBuilderCapacity {
		return capacity
	}
	remaining := maxExportBuilderCapacity - capacity
	if count > remaining/bytesPerItem {
		return maxExportBuilderCapacity
	}
	return capacity + count*bytesPerItem
}
