//go:build windows

package i18n

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const localeNameMaxLength = 85

// System returns the user's Windows display locale as a language tag, or
// nothing when the system does not report one.
func System() string {
	name, err := userDefaultLocaleName()
	if err != nil {
		return ""
	}
	return name
}

func userDefaultLocaleName() (string, error) {
	procedure := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")
	buffer := make([]uint16, localeNameMaxLength)
	length, _, callErr := procedure.Call(
		uintptr(unsafe.Pointer(&buffer[0])), uintptr(localeNameMaxLength))
	if length == 0 {
		return "", callErr
	}
	return windows.UTF16ToString(buffer), nil
}
