//go:build !darwin && !linux

package config

func checkHomePerm(string) error {
	return nil
}
