//go:build !darwin && !linux && !windows

package platform

func portOwnerPID(int) (int, bool) {
	return 0, false
}
