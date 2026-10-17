package fixture

import "unsafe"

func Load(addr uintptr) int {
	return *(*int)(unsafe.Pointer(addr))
}
