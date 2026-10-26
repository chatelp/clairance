//go:build !linux && !darwin

package api

// processMemory is not read on this system: the figures are absent.
func processMemory(pids []int) map[int]int64 { return map[int]int64{} }

// memoryPressure is said on macOS alone.
func memoryPressure() string { return "" }
