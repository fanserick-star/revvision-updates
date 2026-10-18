// Rev Visión: verified online updates, exclusive installer lock,
// crash-safe recovery and rollback after live health checks.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"revvision/updater"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Bound at build time. Private signing material never enters the executable.
var updatePublicKey string
var launcherBuildVersion string

type channel struct {
	ManifestURL string `json:"manifest_url"`
}

func popup(msg string) {
	u := syscall.NewLazyDLL("user32.dll")
	p := u.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(msg)
	title, _ := syscall.UTF16PtrFromString("Rev Visión · Launcher")
	p.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(title)), 0x30)
}
func report(base, msg string) {
	f, e := os.OpenFile(filepath.Join(base, "launcher-updates.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e == nil {
		fmt.Fprintln(f, time.Now().Format(time.RFC3339), msg)
		f.Close()
	}
}
func mutex() (syscall.Handle, bool, error) {
	k := syscall.NewLazyDLL("kernel32.dll")
	p := k.NewProc("CreateMutexW")
	name, _ := syscall.UTF16PtrFromString("Local\\RevVision-Pruebas-Launcher-Update-438932")
	h, _, e := p.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return 0, false, fmt.Errorf("no se pudo bloquear el launcher: %w", e)
	}
	already := e == syscall.Errno(183)
	return syscall.Handle(h), already, nil
}
func appPaths() (base, program string, e error) {
	exe, e := os.Executable()
	if e != nil {
		return
	}
	dir := filepath.Dir(exe)
	if strings.EqualFold(filepath.Base(dir), "program") {
		base = filepath.Dir(dir)
	} else {
		base = dir
	}
	program = filepath.Join(base, "program")
	if _, err := os.Stat(filepath.Join(program, "release.json")); err != nil {
		if _, err = os.Stat(filepath.Join(dir, "release.json")); err == nil {
			return dir, dir, nil
		}
	}
	return base, program, nil
}
func readChannel(base string) (string, error) {
	b, e := os.ReadFile(filepath.Join(base, "update-channel.json"))
	if os.IsNotExist(e) {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	if len(b) > 4096 {
		return "", errors.New("configuración demasiado grande")
	}
	var x channel
	if e = json.Unmarshal(b, &x); e != nil {
		return "", e
	}
	return x.ManifestURL, nil
}
func healthVersion() (string, bool) {
	c := http.Client{Timeout: 900 * time.Millisecond}
	r, e := c.Get("http://127.0.0.1:58437/api/health")
	if e != nil {
		return "", false
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return "", false
	}
	var info struct {
		App     string `json:"app"`
		Version string `json:"version"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&info) != nil {
		return "", false
	}
	return info.Version, info.App == "RevVision"
}
func portOccupied() bool {
	conn, e := net.DialTimeout("tcp", "127.0.0.1:58437", 250*time.Millisecond)
	if e != nil {
		return false
	}
	conn.Close()
	return true
}
// reopenWindow restores the app-mode window when the local server is already
// healthy. Starting the launcher again must never silently do nothing.
// The dedicated Edge profile is the same one the application uses.
func reopenWindow() error {
	const address = "http://127.0.0.1:58437"
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		return errors.New("no se encontro LOCALAPPDATA para mostrar Rev Vision")
	}
	profile := filepath.Join(root, "RevVision", "Pruebas", "edge-window-profile")
	if e := os.MkdirAll(profile, 0700); e != nil {
		return fmt.Errorf("perfil de ventana: %w", e)
	}
	paths := []string{}
	for _, env := range []string{"PROGRAMFILES(X86)", "PROGRAMFILES", "LOCALAPPDATA"} {
		if dir := os.Getenv(env); dir != "" {
			paths = append(paths, filepath.Join(dir, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
	}
	edge := ""
	for _, path := range paths {
		if st, e := os.Stat(path); e == nil && !st.IsDir() {
			edge = path
			break
		}
	}
	if edge == "" {
		if path, e := exec.LookPath("msedge.exe"); e == nil {
			edge = path
		}
	}
	if edge == "" {
		return errors.New("no se encontro Microsoft Edge para abrir la ventana; mientras tanto usa http://127.0.0.1:58437")
	}
	cmd := exec.Command(edge,
		"--app="+address,
		"--user-data-dir="+profile,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-mode",
	)
	if e := cmd.Start(); e != nil {
		return fmt.Errorf("no se pudo abrir la ventana de Rev Vision: %w", e)
	}
	return cmd.Process.Release()
}

func updateIfAvailable(base string, local updater.AppRelease) (bool, error) {
	if portOccupied() {
		return false, errors.New("Rev Visión está atendiendo, se pospone la actualización")
	}
	url, e := readChannel(base)
	if e != nil {
		return false, e
	}
	if url == "" {
		return false, nil
	}
	m, e := updater.ReadRemoteManifest(url)
	if e != nil {
		return false, e
	}
	if e = updater.Verify(m, updatePublicKey); e != nil {
		return false, e
	}
	if updater.BlockedVersion(base, m.Version) {
		return false, fmt.Errorf("versión %s suspendida porque falló su arranque anterior", m.Version)
	}
	cmp, e := updater.VersionCompare(m.Version, local.AppVersion)
	if e != nil {
		return false, e
	}
	if cmp <= 0 {
		return false, nil
	}
	stage, e := updater.StageRemoteRelease(base, m)
	if e != nil {
		return false, e
	}
	if portOccupied() {
		return false, errors.New("el programa abrió durante la descarga; no se tocaron archivos")
	}
	if e = updater.ActivateRelease(base, stage); e != nil {
		return false, e
	}
	report(base, "Candidato instalado pendiente de prueba de salud: "+m.Version)
	return true, nil
}
func repairIfRequired(base, program string) error {
	if _, e := updater.ReadAppRelease(program); e == nil {
		return nil
	}
	url, e := readChannel(base)
	if e != nil {
		return e
	}
	if url == "" {
		return errors.New("ejecutable dañado y no hay canal firmado configurado")
	}
	m, e := updater.ReadRemoteManifest(url)
	if e != nil {
		return e
	}
	if e = updater.Verify(m, updatePublicKey); e != nil {
		return e
	}
	stage, e := updater.StageRemoteRelease(base, m)
	if e != nil {
		return e
	}
	return updater.RepairBroken(base, stage)
}
func launchAndWait(program, version string) error {
	if portOccupied() {
		v, ok := healthVersion()
		if !ok {
			return errors.New("puerto 58437 ocupado por otro servicio")
		}
		if v != version {
			return fmt.Errorf("ya hay otra versión abierta (%s): no se reemplazará", v)
		}
		return reopenWindow()
	}
	cmd := exec.Command(filepath.Join(program, "RevVisionApp.exe"))
	cmd.Dir = program
	if e := cmd.Start(); e != nil {
		return fmt.Errorf("no se pudo iniciar ejecutable: %w", e)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.Now().Add(22 * time.Second)
	stableSince := time.Time{}
	for time.Now().Before(deadline) {
		select {
		case e := <-done:
			return fmt.Errorf("la aplicación terminó durante el arranque: %v", e)
		default:
		}
		v, ok := healthVersion()
		if ok && v == version {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= 2400*time.Millisecond {
				return nil
			}
		} else {
			stableSince = time.Time{}
		}
		time.Sleep(250 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
	}
	return errors.New("el programa no alcanzó un estado saludable y estable en 22 segundos")
}
func scheduleLauncherRefresh(base, program string, current updater.AppRelease) error {
	if current.LauncherSHA256 == "" || current.HelperSHA256 == "" {
		return nil
	}
	target := filepath.Join(base, "RevVisionLauncher.exe")
	oldHash, e := updater.FileHash(target)
	if e != nil {
		return e
	}
	if oldHash == current.LauncherSHA256 {
		return nil
	}
	src := filepath.Join(program, "RevVisionLauncher.exe")
	srcHash, e := updater.FileHash(src)
	if e != nil || srcHash != current.LauncherSHA256 {
		return errors.New("launcher de actualización no supera SHA-256")
	}
	helper := filepath.Join(program, "LauncherUpdater.exe")
	helperHash, e := updater.FileHash(helper)
	if e != nil || helperHash != current.HelperSHA256 {
		return errors.New("auxiliar de actualización no supera SHA-256")
	}
	pid := strconv.Itoa(os.Getpid())
	cmd := exec.Command(helper, "--parent", pid, "--base", base)
	cmd.Dir = program
	if e = cmd.Start(); e != nil {
		return e
	}
	report(base, "Actualización propia del launcher delegada a auxiliar firmado")
	return nil
}
func run() error {
	lock, exists, e := mutex()
	if e != nil {
		return e
	}
	if exists {
		syscall.CloseHandle(lock)
		return nil
	}
	defer syscall.CloseHandle(lock)
	base, program, e := appPaths()
	if e != nil {
		return e
	}
	if base == program {
		rel, e := updater.ReadAppRelease(program)
		if e != nil {
			return e
		}
		return launchAndWait(program, rel.AppVersion)
	}
	if e = updater.RecoverInterrupted(base); e != nil {
		report(base, "Recuperación: "+e.Error())
		return e
	}
	// A verified earlier version can start even when no Internet is available.
	if e = repairIfRequired(base, program); e != nil {
		return fmt.Errorf("recuperación del programa: %w", e)
	}
	rel, e := updater.ReadAppRelease(program)
	if e != nil {
		return e
	}
	if portOccupied() {
		v, ok := healthVersion()
		if !ok {
			return errors.New("el puerto de Rev Visión está ocupado por otro programa")
		}
		if v != rel.AppVersion {
			report(base, fmt.Sprintf("Servidor existente %s; instalacion local %s. Se pospone actualizar y se muestra la ventana", v, rel.AppVersion))
		}
		return reopenWindow()
	}
	if updated, err := updateIfAvailable(base, rel); err != nil {
		report(base, "No se pudo actualizar: "+err.Error())
		popup("No se pudo completar la comprobación de actualizaciones.\n\nSe abrirá la última versión local verificada.\n\nDetalle: " + err.Error())
	} else if updated {
		report(base, "Candidato recibido; verificando inicio")
	}
	rel, e = updater.ReadAppRelease(program)
	if e != nil {
		return e
	}
	if e = launchAndWait(program, rel.AppVersion); e != nil {
		report(base, "Fallo al arrancar: "+e.Error())
		if r := updater.RollbackRelease(base, "falló la prueba de inicio: "+e.Error()); r != nil {
			return fmt.Errorf("falló inicio; restauración no disponible (%v): %w", r, e)
		}
		prev, e2 := updater.ReadAppRelease(program)
		if e2 != nil {
			return fmt.Errorf("rollback inválido: %w", e2)
		}
		if e2 = launchAndWait(program, prev.AppVersion); e2 != nil {
			return fmt.Errorf("tampoco arrancó la última versión buena: %w", e2)
		}
		popup("La versión nueva falló al arrancar. Restauré la última versión estable (" + prev.AppVersion + ").")
		return nil
	}
	if e = updater.ConfirmRelease(base, rel.AppVersion); e != nil && !strings.Contains(e.Error(), "no hay actualización pendiente") {
		report(base, "Confirmación de versión: "+e.Error())
		return e
	}
	if e = scheduleLauncherRefresh(base, program, rel); e != nil {
		report(base, "Autoactualización de launcher pendiente: "+e.Error())
		popup("Rev Visión abrió correctamente, pero su launcher aún no pudo actualizarse.\n" + e.Error())
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		popup("Rev Visión no pudo iniciar.\n\n" + e.Error())
		os.Exit(1)
	}
}
