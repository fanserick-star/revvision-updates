// External launcher replacement helper. Runs only after the active launcher has
// exited; verifies both components against the installed signed release.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"revvision/updater"
	"syscall"
	"time"
	"unsafe"
)

func moveReplace(src, dest string) error {
	k := syscall.NewLazyDLL("kernel32.dll")
	p := k.NewProc("MoveFileExW")
	a, e := syscall.UTF16PtrFromString(src)
	if e != nil {
		return e
	}
	b, e := syscall.UTF16PtrFromString(dest)
	if e != nil {
		return e
	}
	ok, _, err := p.Call(uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b)), uintptr(1|8))
	if ok == 0 {
		return fmt.Errorf("reemplazo atómico denegado: %w", err)
	}
	return nil
}
func waitParent(pid int) error {
	k := syscall.NewLazyDLL("kernel32.dll")
	open := k.NewProc("OpenProcess")
	handle, _, e := open.Call(uintptr(0x00100000), 0, uintptr(pid))
	if handle == 0 {
		if e == syscall.Errno(87) {
			return nil
		}
		return fmt.Errorf("no se pudo comprobar el proceso del launcher: %w", e)
	}
	defer syscall.CloseHandle(syscall.Handle(handle))
	r, e := syscall.WaitForSingleObject(syscall.Handle(handle), 20000)
	if e != nil || r != syscall.WAIT_OBJECT_0 {
		return errors.New("launcher aún ejecutándose; no se reemplaza")
	}
	return nil
}
func run() error {
	var pid int
	var base string
	flag.IntVar(&pid, "parent", 0, "ID del launcher que está cerrando")
	flag.StringVar(&base, "base", "", "ruta de instalación")
	flag.Parse()
	if pid <= 0 || base == "" {
		return errors.New("argumentos incompletos")
	}
	rel, e := updater.ReadAppRelease(filepath.Join(base, "program"))
	if e != nil {
		return e
	}
	if rel.LauncherSHA256 == "" || rel.HelperSHA256 == "" {
		return errors.New("no hay actualización firmada del launcher")
	}
	source := filepath.Join(base, "program", "RevVisionLauncher.exe")
	h, e := updater.FileHash(source)
	if e != nil || h != rel.LauncherSHA256 {
		return errors.New("SHA-256 del launcher no coincide")
	}
	self, e := os.Executable()
	if e != nil {
		return e
	}
	h, e = updater.FileHash(self)
	if e != nil || h != rel.HelperSHA256 {
		return errors.New("SHA-256 del auxiliar no coincide")
	}
	dest := filepath.Join(base, "RevVisionLauncher.exe")
	// Stage a complete verified binary, never edit in place.
	next := dest + ".next"
	backup := dest + ".last"
	if _, e = os.Stat(next); e == nil {
		return errors.New("archivo de actualización anterior sin revisar")
	}
	b, e := os.ReadFile(source)
	if e != nil {
		return e
	}
	if e = os.WriteFile(next, b, 0600); e != nil {
		return e
	}
	h, e = updater.FileHash(next)
	if e != nil || h != rel.LauncherSHA256 {
		return errors.New("copia de launcher dañada")
	}
	if e = waitParent(pid); e != nil {
		return e
	}
	// Save the current launcher in a single bounded backup without making the active path disappear.
	old, e := os.ReadFile(dest)
	if e != nil {
		return e
	}
	oldHash, e := updater.FileHash(dest)
	if e != nil {
		return e
	}
	backupNew := backup + ".new"
	if e = os.WriteFile(backupNew, old, 0600); e != nil {
		return e
	}
	if saved, e := updater.FileHash(backupNew); e != nil || saved != oldHash {
		return errors.New("respaldo de launcher alterado")
	}
	if e = moveReplace(backupNew, backup); e != nil {
		return e
	}
	// MOVEFILE_REPLACE_EXISTING changes the verified executable atomically.
	// On failure Windows leaves the old launcher untouched.
	if e = moveReplace(next, dest); e != nil {
		return e
	}
	h, e = updater.FileHash(dest)
	if e != nil || h != rel.LauncherSHA256 {
		_ = moveReplace(backup, dest)
		return errors.New("launcher nuevo no coincide después del reemplazo")
	}
	c := exec.Command(dest)
	c.Dir = filepath.Join(base, "program")
	if e = c.Start(); e != nil {
		_ = moveReplace(backup, dest)
		return e
	}
	// Ensure the new launcher returns a successful startup, otherwise restore.
	finished := make(chan error, 1)
	go func() { finished <- c.Wait() }()
	select {
	case e = <-finished:
		if e != nil {
			_ = moveReplace(backup, dest)
			return fmt.Errorf("launcher nuevo no pudo iniciar: %w", e)
		}
	case <-time.After(28 * time.Second):
		// It may still be checking the remote channel. Do not replace a running exe.
	}

	// Do not accumulate historical chains; only this one launcher fallback exists.
	return nil
}
func main() {
	if e := run(); e != nil {
		f, _ := os.OpenFile(filepath.Join(os.Getenv("LOCALAPPDATA"), "RevVision", "Pruebas", "launcher-self-update.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if f != nil {
			fmt.Fprintln(f, time.Now().Format(time.RFC3339), e)
			f.Close()
		}
		os.Exit(1)
	}
}
