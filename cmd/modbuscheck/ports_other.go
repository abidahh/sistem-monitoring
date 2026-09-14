//go:build !windows

package main

import "errors"

// listCOMPorts pada platform selain Windows hanya memberi tahu bahwa enumerasi
// port COM khusus Windows (registry), agar build tetap hijau lintas-platform.
func listCOMPorts() ([]string, error) {
	return nil, errors.New("pencarian port COM hanya didukung di Windows")
}