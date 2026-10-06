//go:build windows && cgo

package main

/*
#include <io.h>
#include <windows.h>

static intptr_t get_osf_stdout() {
    return _get_osfhandle(1);
}

static int write_c_stdout(const void *buf, int count) {
    return _write(1, buf, count);
}
*/
import "C"
import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

const stdOutputHandle = uint32(0xfffffff5) // STD_OUTPUT_HANDLE (-11)

var debugLogF *os.File

func initDebugLog() {
	logPath := filepath.Join(os.TempDir(), "inkanim-ext-debug.log")
	debugLogF, _ = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
}

func logDebug(format string, args ...any) {
	if debugLogF != nil {
		_, _ = fmt.Fprintf(debugLogF, format+"\n", args...)
		_ = debugLogF.Sync()
	}
}

func closeDebugLog() {
	if debugLogF != nil {
		_ = debugLogF.Close()
		debugLogF = nil
	}
}

// writeOutput writes the modified SVG to Inkscape's stdout pipe.
// When compiled as a Windows GUI application (-H windowsgui), Windows does not
// bind standard handles to a console. Furthermore, Inkscape (GLib gspawn) passes
// inherited pipe handles via MSVCRT's lpReserved2 rather than STARTF_USESTDHANDLES.
// We query C file descriptor 1 from MSVCRT via _get_osfhandle(1) and write directly
// via Win32 WriteFile (with fallbacks to C _write and Go os.Stdout).
func writeOutput(data []byte) error {
	logDebug("writeOutput called with %d bytes", len(data))

	// Retrieve OS handle from C runtime fd 1 (populated by CRT from lpReserved2)
	cHandle := uintptr(C.get_osf_stdout())
	logDebug("_get_osfhandle(1): 0x%x (invalid=%v)", cHandle, cHandle == ^uintptr(0))

	if cHandle != ^uintptr(0) && cHandle != 0 {
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		procSetStdHandle := kernel32.NewProc("SetStdHandle")
		procSetStdHandle.Call(uintptr(stdOutputHandle), cHandle)

		var written uint32
		err := syscall.WriteFile(syscall.Handle(cHandle), data, &written, nil)
		logDebug("syscall.WriteFile to cHandle 0x%x: written=%d, err=%v", cHandle, written, err)
		if err == nil && int(written) == len(data) {
			return nil
		}
	}

	// Fallback to C runtime _write(1, ...)
	if len(data) > 0 {
		n := C.write_c_stdout(unsafe.Pointer(&data[0]), C.int(len(data)))
		logDebug("C.write_c_stdout: written=%d", int(n))
		if int(n) == len(data) {
			return nil
		}
	}

	// Fallback to os.Stdout.Write
	n, err := os.Stdout.Write(data)
	logDebug("os.Stdout.Write: written=%d, err=%v", n, err)
	return err
}
