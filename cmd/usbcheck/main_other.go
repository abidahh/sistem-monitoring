//go:build !windows

// usbcheck versi non-Windows: hanya pemberitahuan bahwa tool ini khusus
// Windows (SetupAPI), agar project tetap bisa di-build di platform lain.
package main

import "fmt"

func main() {
	fmt.Println("usbcheck hanya berjalan di Windows (memakai SetupAPI).")
}