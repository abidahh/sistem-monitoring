//go:build windows

package main

import (
	"errors"
	"io"
	"sort"

	"golang.org/x/sys/windows/registry"
)

// listCOMPorts membaca daftar port COM yang terdaftar pada Windows lewat
// registry HARDWARE\DEVICEMAP\SERIALCOMM (pola yang sama dengan cmd/usbcheck).
func listCOMPorts() ([]string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`HARDWARE\DEVICEMAP\SERIALCOMM`, registry.READ)
	if err != nil {
		return nil, err
	}
	defer key.Close()

	names, err := key.ReadValueNames(128)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	var ports []string
	for _, n := range names {
		if v, _, err := key.GetStringValue(n); err == nil {
			ports = append(ports, v)
		}
	}
	sort.Strings(ports)
	return ports, nil
}