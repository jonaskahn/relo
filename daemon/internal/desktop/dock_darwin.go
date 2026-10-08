//go:build darwin

package desktop

// The tray is a menu bar presence alone, so the process takes AppKit's
// accessory activation policy: the regular policy would put a Dock icon on
// screen with no window behind it. The systray fork registers the selector
// but never calls it, which is why this goes through the Objective-C runtime
// directly.

import (
	"log/slog"
	"runtime"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

const nsApplicationAccessory = 1

var objc struct {
	once sync.Once
	err  error
	send unsafe.Pointer
	get  unsafe.Pointer
	sel  unsafe.Pointer
}

func applyAccessoryPresence(logger *slog.Logger) {
	if err := initObjC(); err != nil {
		logger.Warn("could not reach the Objective-C runtime for the Dock presence", "error", err)
		return
	}
	app := objcClass("NSApplication").send("sharedApplication")
	if app == 0 {
		logger.Warn("AppKit has no shared application for the Dock presence")
		return
	}
	if app.send("setActivationPolicy:", nsApplicationAccessory) == 0 {
		logger.Warn("AppKit refused the accessory activation policy")
	}
}

func initObjC() error {
	objc.once.Do(func() {
		objc.err = loadObjC()
	})
	return objc.err
}

func loadObjC() error {
	lib, err := ffi.LoadLibrary("/usr/lib/libobjc.A.dylib")
	if err != nil {
		return err
	}
	if _, err := ffi.LoadLibrary("/System/Library/Frameworks/AppKit.framework/AppKit"); err != nil {
		return err
	}
	if objc.send, err = ffi.GetSymbol(lib, "objc_msgSend"); err != nil {
		return err
	}
	if objc.get, err = ffi.GetSymbol(lib, "objc_getClass"); err != nil {
		return err
	}
	objc.sel, err = ffi.GetSymbol(lib, "sel_registerName")
	return err
}

type objcID uintptr

func objcClass(name string) objcID {
	if initObjC() != nil {
		return 0
	}
	bytes := append([]byte(name), 0)
	namePtr := uintptr(unsafe.Pointer(&bytes[0]))
	defer runtime.KeepAlive(bytes)
	return objcID(objcCall(objc.get, []*types.TypeDescriptor{types.PointerTypeDescriptor}, namePtr))
}

func objcSelector(name string) uintptr {
	if initObjC() != nil {
		return 0
	}
	bytes := append([]byte(name), 0)
	namePtr := uintptr(unsafe.Pointer(&bytes[0]))
	defer runtime.KeepAlive(bytes)
	return objcCall(objc.sel, []*types.TypeDescriptor{types.PointerTypeDescriptor}, namePtr)
}

func (id objcID) send(sel string, args ...uintptr) objcID {
	if id == 0 {
		return 0
	}
	all := make([]uintptr, 0, 2+len(args))
	all = append(all, uintptr(id), objcSelector(sel))
	all = append(all, args...)
	typeset := make([]*types.TypeDescriptor, len(all))
	for i := range typeset {
		typeset[i] = types.PointerTypeDescriptor
	}
	return objcID(objcCall(objc.send, typeset, all...))
}

func objcCall(fn unsafe.Pointer, argTypes []*types.TypeDescriptor, args ...uintptr) uintptr {
	cif := &types.CallInterface{}
	if err := ffi.PrepareCallInterface(cif, types.DefaultCall, types.PointerTypeDescriptor, argTypes); err != nil {
		return 0
	}
	ptrs := make([]unsafe.Pointer, len(args))
	for i := range args {
		ptrs[i] = unsafe.Pointer(&args[i])
	}
	var result uintptr
	if _, err := ffi.CallFunction(cif, fn, unsafe.Pointer(&result), ptrs); err != nil {
		return 0
	}
	return result
}
