//go:build !windows

package scanner

import (
	"path/filepath"
	"sort"
)

// ListCOMPorts pada platform non-Windows mengembalikan daftar perangkat serial
// umum (/dev/ttyUSB* dan /dev/ttyACM* di Linux/macOS).
func ListCOMPorts() ([]string, error) {
	paths, err := filepath.Glob("/dev/ttyUSB*")
	if err != nil {
		return nil, err
	}
	acm, err := filepath.Glob("/dev/ttyACM*")
	if err != nil {
		return nil, err
	}
	paths = append(paths, acm...)
	sort.Strings(paths)
	return paths, nil
}