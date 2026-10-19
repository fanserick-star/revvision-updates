//go:build windows
package windowcontrol

import (
    "syscall"
    "testing"
    "time"
    "unsafe"
)
func TestFocusExistingAppInsteadOfOpeningDuplicate(t *testing.T) {
    user:=syscall.NewLazyDLL("user32.dll")
    create:=user.NewProc("CreateWindowExW")
    destroy:=user.NewProc("DestroyWindow")
    title,_:=syscall.UTF16PtrFromString(Title)
    kind,_:=syscall.UTF16PtrFromString("STATIC")
    hwnd,_,err:=create.Call(0,uintptr(unsafe.Pointer(kind)),uintptr(unsafe.Pointer(title)),
        uintptr(0x10000000|0x00CF0000),10,10,300,120,0,0,0,0)
    if hwnd==0 {t.Skipf("Windows test desktop unavailable: %v",err)}
    defer destroy.Call(hwnd)
    if !Exists() {t.Fatal("Did not find existing Rev Vision window")}
    if !ActivateExisting() {t.Fatal("Did not reuse existing window")}
    if !WaitForExisting(250*time.Millisecond) {t.Fatal("WaitForExisting failed")}
}
