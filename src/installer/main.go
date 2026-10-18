package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"revvision/updater"
)

//go:embed bundle.zip
var embedded []byte

type manifest struct {
	AppVersion  string `json:"app_version"`
	AppName     string `json:"app_name"`
	AppSHA      string `json:"app_sha256"`
	LauncherSHA string `json:"launcher_sha256"`
	HelperSHA   string `json:"helper_sha256"`
}
type controlFile struct {
	PID   int    `json:"pid"`
	Token string `json:"token"`
}

var errCancelled = errors.New("actualización cancelada por el usuario")

func ask(title, msg string) bool {
	user32 := syscall.NewLazyDLL("user32.dll")
	pr := user32.NewProc("MessageBoxW")
	m, _ := syscall.UTF16PtrFromString(msg)
	t, _ := syscall.UTF16PtrFromString(title)
	r, _, _ := pr.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x134) // YESNO | ICONWARNING | DEFBUTTON2
	return r == 6
}
func currentProcessPath(handle syscall.Handle) (string, error) {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	query := kernel.NewProc("QueryFullProcessImageNameW")
	buf := make([]uint16, 32768)
	sz := uint32(len(buf))
	ret, _, err := query.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&sz)))
	if ret == 0 {
		return "", fmt.Errorf("no se pudo comprobar la ruta del proceso: %w", err)
	}
	return syscall.UTF16ToString(buf[:sz]), nil
}
func openedInstalledProcesses(wanted string) ([]syscall.Handle, error) {
	snap, e := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if e != nil {
		return nil, e
	}
	defer syscall.CloseHandle(snap)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	e = syscall.Process32First(snap, &pe)
	if e != nil {
		return nil, e
	}
	handles := []syscall.Handle{}
	for e == nil {
		if strings.EqualFold(syscall.UTF16ToString(pe.ExeFile[:]), "RevVisionApp.exe") {
			// Only open programs whose exact executable path is inside this installation.
			handle, openErr := syscall.OpenProcess(0x1000|0x0001|0x00100000, false, pe.ProcessID)
			if openErr == nil {
				actual, qErr := currentProcessPath(handle)
				if qErr == nil && strings.EqualFold(filepath.Clean(actual), filepath.Clean(wanted)) {
					handles = append(handles, handle)
				} else {
					syscall.CloseHandle(handle)
				}
			}
		}
		e = syscall.Process32Next(snap, &pe)
	}
	return handles, nil
}
func closeHandles(handles []syscall.Handle) {
	for _, h := range handles {
		syscall.CloseHandle(h)
	}
}
func isListening() bool {
	c, e := net.DialTimeout("tcp", "127.0.0.1:58437", 350*time.Millisecond)
	if e != nil {
		return false
	}
	c.Close()
	return true
}
func ownedServer() bool {
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, e := client.Get("http://127.0.0.1:58437/api/health")
	if e != nil {
		return false
	}
	defer resp.Body.Close()
	var out struct {
		App string `json:"app"`
	}
	return resp.StatusCode == 200 && json.NewDecoder(io.LimitReader(resp.Body, 512)).Decode(&out) == nil && out.App == "RevVision"
}
func shutdownGracefully(base string, handles []syscall.Handle) bool {
	b, e := os.ReadFile(filepath.Join(os.Getenv("APPDATA"), "RevVision", "Pruebas", "data", "installer-control.json"))
	if e != nil {
		return false
	}
	var c controlFile
	if json.Unmarshal(b, &c) != nil || c.PID <= 0 || len(c.Token) != 64 {
		return false
	}
	found := false
	for _, h := range handles { // The token must belong to the same installed process we identified.
		kernel := syscall.NewLazyDLL("kernel32.dll")
		pidfn := kernel.NewProc("GetProcessId")
		pid, _, _ := pidfn.Call(uintptr(h))
		if int(pid) == c.PID {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	req, e := http.NewRequest(http.MethodPost, "http://127.0.0.1:58437/api/internal/shutdown", nil)
	if e != nil {
		return false
	}
	req.Header.Set("X-RevVision-Control", c.Token)
	req.Header.Set("X-RevVision-Client", "revvision-local")
	client := &http.Client{Timeout: 3 * time.Second}
	resp, e := client.Do(req)
	if e != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}
func waitForStop(handles []syscall.Handle, ms uint32) bool {
	for _, h := range handles {
		result, err := syscall.WaitForSingleObject(h, ms)
		if err != nil || result != syscall.WAIT_OBJECT_0 {
			return false
		}
	}
	return true
}
func stopOwnApp(base string) error {
	target := filepath.Join(base, "program", "RevVisionApp.exe")
	handles, e := openedInstalledProcesses(target)
	if e != nil {
		return fmt.Errorf("no se pudieron inspeccionar los procesos: %w", e)
	}
	defer closeHandles(handles)
	if !isListening() && len(handles) == 0 {
		return nil
	}
	if isListening() && !ownedServer() {
		return errors.New("El puerto local 58437 está ocupado por otro servicio. No se modificó nada")
	}
	if len(handles) == 0 {
		return errors.New("Rev Visión sigue abierto desde una carpeta distinta (posiblemente portátil). Ciérralo antes de actualizar. No se modificó nada")
	}
	if !ask("Rev Visión · Actualizar", "Rev Visión está abierto.\n\nGuarda primero cualquier formulario sin terminar.\n\n¿Quieres que el instalador cierre SOLO la aplicación Rev Visión de esta instalación y continúe?\n\nLas fichas y consultas ya guardadas no se borrarán.\nLa pestaña del navegador puede permanecer abierta, pero se actualizará al reiniciar.") {
		return errCancelled
	}
	if shutdownGracefully(base, handles) && waitForStop(handles, 10000) && !isListening() {
		return nil
	}
	// Older builds (0.1/0.2) had no authenticated shutdown endpoint.
	// After explicit consent, terminate only exact installed-app processes.
	for _, h := range handles {
		status, _ := syscall.WaitForSingleObject(h, 0)
		if status == syscall.WAIT_OBJECT_0 {
			continue
		}
		actual, e := currentProcessPath(h)
		if e != nil || !strings.EqualFold(filepath.Clean(actual), filepath.Clean(target)) {
			return errors.New("La identidad de Rev Visión cambió durante el cierre. No se modificó nada")
		}
		if e := syscall.TerminateProcess(h, 0); e != nil {
			return fmt.Errorf("No se pudo cerrar el programa. No se modificó nada: %w", e)
		}
	}
	if !waitForStop(handles, 10000) {
		return errors.New("Rev Visión no terminó de cerrarse. No se modificó nada")
	}
	for i := 0; i < 30 && isListening(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if isListening() {
		return errors.New("El puerto continúa ocupado; la actualización se detuvo antes de modificar archivos")
	}
	return nil
}
func notify(msg string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	pr := user32.NewProc("MessageBoxW")
	m, _ := syscall.UTF16PtrFromString(msg)
	t, _ := syscall.UTF16PtrFromString("Rev Visión · Instalador de pruebas")
	pr.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x40)
}
func actualHash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), e
}
func installedServerVersion(want string) bool {
	c := &http.Client{Timeout: 1100 * time.Millisecond}
	r, e := c.Get("http://127.0.0.1:58437/api/health")
	if e != nil {
		return false
	}
	defer r.Body.Close()
	var x struct {
		App     string `json:"app"`
		Version string `json:"version"`
	}
	return r.StatusCode == 200 && json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&x) == nil && x.App == "RevVision" && x.Version == want
}
func replaceFileWindows(src, dst string) error {
	k := syscall.NewLazyDLL("kernel32.dll")
	p := k.NewProc("MoveFileExW")
	a, e := syscall.UTF16PtrFromString(src)
	if e != nil {
		return e
	}
	b, e := syscall.UTF16PtrFromString(dst)
	if e != nil {
		return e
	}
	ok, _, err := p.Call(uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b)), uintptr(1|8))
	if ok == 0 {
		return fmt.Errorf("reemplazo no autorizado: %w", err)
	}
	return nil
}
func acquireInstallerLock() (syscall.Handle, error) {
	k := syscall.NewLazyDLL("kernel32.dll")
	p := k.NewProc("CreateMutexW")
	name, _ := syscall.UTF16PtrFromString("Local\\RevVision-Pruebas-Launcher-Update-438932")
	h, _, err := p.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return 0, fmt.Errorf("no se pudo bloquear instalador: %w", err)
	}
	if err == syscall.Errno(183) {
		syscall.CloseHandle(syscall.Handle(h))
		return 0, errors.New("Ya hay una actualización o instalación de Rev Visión en curso")
	}
	return syscall.Handle(h), nil
}
func install() error {
	lock, e := acquireInstallerLock()
	if e != nil {
		return e
	}
	defer syscall.CloseHandle(lock)
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		return errors.New("No se encontró LOCALAPPDATA")
	}
	base := filepath.Join(root, "RevVision", "Pruebas")
	if e := stopOwnApp(base); e != nil {
		return e
	}
	if e := os.MkdirAll(base, 0700); e != nil {
		return e
	}
	program := filepath.Join(base, "program")
	if err := updater.RecoverInterrupted(base); err != nil {
		return fmt.Errorf("recuperación antes de instalar: %w", err)
	}
	stage := filepath.Join(base, ".app-staging")

	if _, e := os.Stat(stage); e == nil {
		return errors.New("Existe una instalación incompleta previa; se detuvo para no borrar archivos inesperados")
	}
	if e := os.Mkdir(stage, 0700); e != nil {
		return e
	}
	staged := false
	defer func() {
		if !staged {
			_ = os.RemoveAll(stage)
		}
	}()
	r, e := zip.NewReader(bytes.NewReader(embedded), int64(len(embedded)))
	if e != nil {
		return e
	}
	expected := map[string]bool{"RevVisionApp.exe": false, "RevVisionLauncher.exe": false, "LauncherUpdater.exe": false, "release.json": false}
	for _, f := range r.File {
		if done, ok := expected[f.Name]; !ok || done || f.FileInfo().IsDir() || f.UncompressedSize64 > uint64(updater.MaxAppSize) {
			return fmt.Errorf("paquete contiene archivo no permitido o duplicado: %s", f.Name)
		}
		expected[f.Name] = true
		src, e := f.Open()
		if e != nil {
			return e
		}
		dest := filepath.Join(stage, f.Name)
		dst, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			src.Close()
			return e
		}
		n, copyerr := io.Copy(dst, io.LimitReader(src, updater.MaxAppSize+1))
		if n > updater.MaxAppSize {
			return errors.New("componente interno demasiado grande")
		}
		closeerr := dst.Close()
		src.Close()
		if copyerr != nil {
			return copyerr
		}
		if closeerr != nil {
			return closeerr
		}
	}
	for k, v := range expected {
		if !v {
			return fmt.Errorf("falta archivo %s", k)
		}
	}
	b, e := os.ReadFile(filepath.Join(stage, "release.json"))
	if e != nil {
		return e
	}
	var m manifest
	if e = json.Unmarshal(b, &m); e != nil {
		return e
	}
	if m.AppName != "RevVisionApp.exe" || len(m.AppSHA) != 64 {
		return errors.New("manifiesto inválido")
	}
	hash, e := actualHash(filepath.Join(stage, m.AppName))
	if e != nil {
		return e
	}
	if !strings.EqualFold(hash, m.AppSHA) {
		return errors.New("archivo alterado: SHA256 inválido")
	}
	if len(m.LauncherSHA) != 64 {
		return errors.New("falta hash del launcher")
	}
	lh, e := actualHash(filepath.Join(stage, "RevVisionLauncher.exe"))
	if e != nil {
		return e
	}
	if !strings.EqualFold(lh, m.LauncherSHA) {
		return errors.New("launcher alterado: SHA256 inválido")
	}
	if len(m.HelperSHA) != 64 {
		return errors.New("falta hash del auxiliar")
	}
	helperHash, e := actualHash(filepath.Join(stage, "LauncherUpdater.exe"))
	if e != nil || !strings.EqualFold(helperHash, m.HelperSHA) {
		return errors.New("auxiliar alterado: SHA256 inválido")
	}
	// Reinstalar un EXE antiguo jamás debe retroceder una versión ya publicada.
	if prior, priorErr := updater.ReadAppRelease(program); priorErr == nil {
		order, orderErr := updater.VersionCompare(m.AppVersion, prior.AppVersion)
		if orderErr != nil {
			return orderErr
		}
		if order < 0 {
			return errors.New("Ya tienes una versión más reciente. No se instalará una versión anterior ni se tocarán los datos")
		}
	}
	// Revalidar justo antes del reemplazo: el usuario pudo haber abierto otra vez la aplicación.
	if isListening() {
		return errors.New("Rev Visión volvió a abrirse durante la preparación. No se modificó nada; vuelve a ejecutar la actualización")
	}
	active, e := openedInstalledProcesses(filepath.Join(program, "RevVisionApp.exe"))
	if e != nil {
		return fmt.Errorf("no se pudo volver a comprobar Rev Visión: %w", e)
	}
	if len(active) != 0 {
		closeHandles(active)
		return errors.New("Rev Visión volvió a iniciarse. No se modificó nada")
	}
	closeHandles(active)
	// Swap verified application only. If already installed at this same version,
	// keep it and merely repair the separate launcher if needed.
	shouldPromote := true
	if prior, pe := updater.ReadAppRelease(program); pe == nil && prior.AppVersion == m.AppVersion {
		shouldPromote = false
	}
	if shouldPromote {
		if e = updater.ActivateRelease(base, stage); e != nil {
			return fmt.Errorf("activación controlada: %w", e)
		}
		staged = true
		// Do not launch a pending upgrade through the launcher: the launcher must
		// first recover unfinished transactions. Verify actual runtime health here.
		app := exec.Command(filepath.Join(program, "RevVisionApp.exe"))
		app.Dir = program
		if e = app.Start(); e != nil {
			_ = updater.RollbackRelease(base, "no abrió tras instalación")
			return e
		}
		deadline := time.Now().Add(22 * time.Second)
		healthy := false
		since := time.Time{}
		for time.Now().Before(deadline) {
			if installedServerVersion(m.AppVersion) {
				if since.IsZero() {
					since = time.Now()
				}
				if time.Since(since) > 2500*time.Millisecond {
					healthy = true
					break
				}
			}
			time.Sleep(250 * time.Millisecond)
		}
		if !healthy {
			_ = app.Process.Kill()
			_ = app.Wait()
			if r := updater.RollbackRelease(base, "instalador: no respondió tras actualización"); r != nil {
				return fmt.Errorf("no abrió la aplicación y falló restauración: %w", r)
			}
			return errors.New("falló el arranque; se restauró la versión anterior")
		}
		if e = updater.ConfirmRelease(base, m.AppVersion); e != nil {
			return fmt.Errorf("no pudo confirmar aplicación: %w", e)
		}
	} else {
		staged = false // allow removing only the temporary staged folder
	}
	// Install permanent launcher by atomic replacement. Never delete a running
	// launcher before the new one is verified and ready.
	exe := filepath.Join(base, "RevVisionLauncher.exe")
	launcherData, e := os.ReadFile(filepath.Join(program, "RevVisionLauncher.exe"))
	if e != nil {
		return e
	}
	lh2 := sha256.Sum256(launcherData)
	if hex.EncodeToString(lh2[:]) != m.LauncherSHA {
		return errors.New("launcher empaquetado no supera integridad")
	}
	launcherStage := exe + ".new"
	if e = os.WriteFile(launcherStage, launcherData, 0600); e != nil {
		return e
	}
	if hh, e2 := actualHash(launcherStage); e2 != nil || hh != m.LauncherSHA {
		return errors.New("copia del launcher alterada")
	}
	// MoveFileExW can atomically replace an unlocked target (or fail closed).
	if e = replaceFileWindows(launcherStage, exe); e != nil {
		return fmt.Errorf("launcher en uso; aplicación segura instalada, reintenta cuando cierre: %w", e)
	}
	// Dedicated public, download-only update channel. Do not change the URL
	// after the first install, since users may have configured their own host.
	channelPath := filepath.Join(base, "update-channel.json")
	if _, e := os.Stat(channelPath); os.IsNotExist(e) {
		cfg := `{"manifest_url":"https://raw.githubusercontent.com/fanserick-star/revvision-updates/main/update.json"}` + "\n"
		if e = os.WriteFile(channelPath, []byte(cfg), 0600); e != nil {
			return fmt.Errorf("no se pudo configurar canal de actualización: %w", e)
		}
	}
	// Desktop link using the standard Windows shell; installer itself needs no admin rights.
	quoted := strings.ReplaceAll(exe, "'", "''")
	script := `$w=New-Object -ComObject WScript.Shell; $d=[Environment]::GetFolderPath('Desktop'); $s=$w.CreateShortcut((Join-Path $d 'Rev Vision.lnk')); $s.TargetPath='` + quoted + `'; $s.WorkingDirectory='` + strings.ReplaceAll(program, "'", "''") + `'; $s.Description='Rev Visión · Historia Clínica (escritorio, pruebas)'; $s.Save()`
	_ = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Run()
	if !shouldPromote {
		cmd := exec.Command(filepath.Join(program, "RevVisionApp.exe"))
		cmd.Dir = program
		if e = cmd.Start(); e != nil {
			return fmt.Errorf("instalado, pero no abrió aplicación: %w", e)
		}
	}
	return nil
}
func main() {
	if e := install(); e != nil {
		if errors.Is(e, errCancelled) {
			return
		}
		notify("No se pudo instalar Rev Visión.\n\n" + e.Error())
		os.Exit(1)
	}
}
