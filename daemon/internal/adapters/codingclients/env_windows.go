//go:build windows

package codingclients

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var modUser32 = windows.NewLazySystemDLL("user32.dll")

var procSendMessageTimeout = modUser32.NewProc("SendMessageTimeoutW")

func envPublished(_ Paths, agent Agent) bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer func() { _ = key.Close() }()
	value, _, err := key.GetStringValue(agent.EnvKey)
	return err == nil && strings.TrimSpace(value) != ""
}

func writeShellEnv(Paths, Agent) error { return nil }

func stripShellEnv(Paths, Agent) error { return nil }

func applyLiveEnv(paths Paths, agent Agent) error {
	if !liveHome(paths.Home) {
		return nil
	}
	value, err := keyValue(paths, agent.ID)
	if err != nil {
		return nil
	}
	_ = os.Setenv(agent.EnvKey, value)
	return setUserEnv(agent.EnvKey, value)
}

func clearLiveEnv(paths Paths, agent Agent) error {
	if !liveHome(paths.Home) {
		return nil
	}
	_ = os.Unsetenv(agent.EnvKey)
	return deleteUserEnv(agent.EnvKey)
}

func setUserEnv(name, value string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, "Environment", registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open the user environment: %w", err)
	}
	defer func() { _ = key.Close() }()
	if err := key.SetStringValue(name, value); err != nil {
		return fmt.Errorf("set %s: %w", name, err)
	}
	broadcastEnvChange()
	return nil
}

func deleteUserEnv(name string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer func() { _ = key.Close() }()
	_ = key.DeleteValue(name)
	broadcastEnvChange()
	return nil
}

func broadcastEnvChange() {
	param, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	var result uintptr
	_, _, _ = procSendMessageTimeout.Call(
		uintptr(0xFFFF),
		uintptr(0x001A),
		0,
		uintptr(unsafe.Pointer(param)),
		uintptr(0x0002),
		uintptr(5000),
		uintptr(unsafe.Pointer(&result)),
	)
}
