//go:build !darwin || !cgo

package api

// processFootprint is read on macOS alone, where the footprint counts what a
// model holds on the GPU. Elsewhere resident memory would not (a sidecar on
// CUDA keeps its models in the card's memory): the figures are absent rather
// than wrong.
func processFootprint(pids []int) map[int]int64 { return map[int]int64{} }

// memoryPressure is said on macOS alone.
func memoryPressure() string { return "" }
