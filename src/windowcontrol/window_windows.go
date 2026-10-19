//go:build windows

// Package windowcontrol brings the one existing Rev Vision app window to the
// foreground. Opening an additional shortcut must not create another Edge
// app window when the existing one can simply be restored.
package windowcontrol

import (
    "syscall"
    "time"
    "unsafe"
)

const Title = "Rev Visión | Historia Clínica"
var winuser = syscall.NewLazyDLL("user32.dll")
var winFind = winuser.NewProc("FindWindowW")
var winRestore = winuser.NewProc("ShowWindowAsync")
var winFront = winuser.NewProc("SetForegroundWindow")
var winVisible = winuser.NewProc("IsWindowVisible")
var winWindow = winuser.NewProc("IsWindow")

func find() uintptr {
    title, err := syscall.UTF16PtrFromString(Title)
    if err != nil {return 0}
    hwnd, _, _ := winFind.Call(0,uintptr(unsafe.Pointer(title)))
    if hwnd==0 {return 0}
    valid,_,_:=winWindow.Call(hwnd)
    if valid==0 {return 0}
    visible,_,_:=winVisible.Call(hwnd)
    if visible==0 {return 0}
    return hwnd
}
func Exists() bool {return find()!=0}
func ActivateExisting() bool {
    hwnd:=find()
    if hwnd==0 {return false}
    // SW_RESTORE also restores an app window minimized to the taskbar.
    winRestore.Call(hwnd,9)
    winFront.Call(hwnd)
    return true
}
// WaitForExisting keeps the launcher mutex alive while Edge creates its first
// window, preventing very rapid repeated shortcut clicks from spawning extras.
func WaitForExisting(timeout time.Duration) bool {
    deadline:=time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if Exists() {return true}
        time.Sleep(100*time.Millisecond)
    }
    return Exists()
}
