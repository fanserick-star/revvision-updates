package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Updates touch only these application directories. Patient data is stored under APPDATA.
const pendingFile = ".update-pending.json"
const blockedFile = ".update-blocked.json"

type Pending struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type blocked struct {
	Version string `json:"version"`
	Cause   string `json:"cause"`
}

func atomicJSON(path string, item any) error {
	b, e := json.MarshalIndent(item, "", "  ")
	if e != nil {
		return e
	}
	b = append(b, '\n')
	tmp := path + ".writing"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	f, e := os.OpenFile(tmp, os.O_RDWR, 0)
	if e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	f.Close()
	if e = os.Rename(tmp, path); e != nil {
		_ = os.Remove(tmp)
		return e
	}
	return nil
}
func exists(path string) (bool, error) {
	_, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return false, nil
	}
	return e == nil, e
}
func safeFolder(path string, allowed map[string]bool) error {
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("carpeta no confiable: %s", path)
	}
	ent, e := os.ReadDir(path)
	if e != nil {
		return e
	}
	for _, x := range ent {
		if !allowed[x.Name()] || x.IsDir() || x.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("contenido inesperado en %s: %s", path, x.Name())
		}
	}
	return nil
}
func removeKnownStage(base string) error {
	stage := filepath.Join(base, ".app-staging")
	ok, e := exists(stage)
	if e != nil {
		return e
	}
	if ok {
		if e = safeFolder(stage, map[string]bool{"RevVisionApp.exe": true, "release.json": true, "RevVisionLauncher.exe": true, "LauncherUpdater.exe": true}); e != nil {
			return e
		}
		if e = os.RemoveAll(stage); e != nil {
			return e
		}
	}
	dl := filepath.Join(base, ".download-release.zip")
	ok, e = exists(dl)
	if e != nil {
		return e
	}
	if ok {
		st, e := os.Lstat(dl)
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() {
			return errors.New("descarga pendiente no es archivo regular")
		}
		return os.Remove(dl)
	}
	return nil
}
func removeKnownProgram(path string) error {
	if e := safeFolder(path, map[string]bool{"RevVisionApp.exe": true, "release.json": true, "RevVisionLauncher.exe": true, "LauncherUpdater.exe": true}); e != nil {
		return e
	}
	return os.RemoveAll(path)
}
func getPending(base string) (*Pending, error) {
	b, e := os.ReadFile(filepath.Join(base, pendingFile))
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var p Pending
	if len(b) > 4096 || json.Unmarshal(b, &p) != nil || !versionPattern.MatchString(p.To) || p.From != "" && !versionPattern.MatchString(p.From) {
		return nil, errors.New("estado de actualización ilegible; se requiere reparación controlada")
	}
	return &p, nil
}
func block(base, version, cause string) error {
	return atomicJSON(filepath.Join(base, blockedFile), blocked{Version: version, Cause: cause})
}
func BlockedVersion(base, version string) bool {
	b, e := os.ReadFile(filepath.Join(base, blockedFile))
	if e != nil {
		return false
	}
	var x blocked
	return json.Unmarshal(b, &x) == nil && x.Version == version
}

// One quarantine is kept; a subsequent recovery may retire it only when it
// contains exactly updater-owned executable files, never arbitrary user data.
func prepareQuarantine(path string) error {
	ok, e := exists(path)
	if e != nil {
		return e
	}
	if !ok {
		return nil
	}
	return removeKnownProgram(path)
}

func readValid(path string) bool { _, e := ReadAppRelease(path); return e == nil }

// RecoverInterrupted is idempotent. It treats an unconfirmed launch as a failed
// upgrade and never trusts a downloaded payload without all hash/signature checks.
func RecoverInterrupted(base string) error {
	if e := removeKnownStage(base); e != nil {
		return fmt.Errorf("limpieza de descarga: %w", e)
	}
	prog := filepath.Join(base, "program")
	prev := filepath.Join(base, ".program-previous")
	quarantine := filepath.Join(base, ".program-quarantine")
	pending, e := getPending(base)
	if e != nil {
		return e
	}
	haveProg, e := exists(prog)
	if e != nil {
		return e
	}
	havePrev, e := exists(prev)
	if e != nil {
		return e
	}
	if pending != nil {
		if havePrev && readValid(prev) {
			if haveProg {
				// Never overwrite quarantine or destroy a directory containing unexpected files.
				if e := prepareQuarantine(quarantine); e != nil {
					return fmt.Errorf("cuarentena protegida: %w", e)
				}
				if e = os.Rename(prog, quarantine); e != nil {
					return e
				}
			}
			if e = os.Rename(prev, prog); e != nil {
				return e
			}
			if e = block(base, pending.To, "actualización no confirmada o interrumpida"); e != nil {
				return e
			}
			return os.Remove(filepath.Join(base, pendingFile))
		}
		// If an install is in-flight and no last-good exists, fail closed unless
		// the new executable was verified and will be health-checked on launch.
		if haveProg && readValid(prog) {
			rel, _ := ReadAppRelease(prog)
			if rel.AppVersion == pending.From {
				return os.Remove(filepath.Join(base, pendingFile))
			}
			if pending.From == "" && rel.AppVersion == pending.To {
				return nil
			} // new clean install, require health check
			return errors.New("actualización pendiente sin respaldo válido; usar reparación firmada")
		}
		return errors.New("instalación incompleta sin versión recuperable")
	}
	if !haveProg && havePrev && readValid(prev) {
		return os.Rename(prev, prog)
	}
	if haveProg && !readValid(prog) && havePrev && readValid(prev) {
		if e := prepareQuarantine(quarantine); e != nil {
			return fmt.Errorf("cuarentena protegida: %w", e)
		}
		if e = os.Rename(prog, quarantine); e != nil {
			return e
		}
		return os.Rename(prev, prog)
	}
	return nil
}

// ActivateRelease promotes only a previously verified staging directory.
// The durable pending marker is written BEFORE either rename, so a power loss
// can always be detected and recovered on the next launch.
func ActivateRelease(base, stage string) error {
	if filepath.Clean(stage) != filepath.Join(base, ".app-staging") {
		return errors.New("carpeta de actualización inválida")
	}
	incoming, e := ReadAppRelease(stage)
	if e != nil {
		return fmt.Errorf("actualización no validada: %w", e)
	}
	if p, e := getPending(base); e != nil || p != nil {
		return errors.New("hay un reemplazo pendiente: recuperar antes de continuar")
	}
	prog := filepath.Join(base, "program")
	prev := filepath.Join(base, ".program-previous")
	had, e := exists(prog)
	if e != nil {
		return e
	}
	from := ""
	if had {
		old, e := ReadAppRelease(prog)
		if e != nil {
			return fmt.Errorf("aplicación anterior no validada: %w", e)
		}
		from = old.AppVersion
		cmp, e := VersionCompare(incoming.AppVersion, from)
		if e != nil || cmp <= 0 {
			return errors.New("versión no es posterior a la instalada")
		}
	}
	if yes, _ := exists(prev); yes {
		if !readValid(prev) {
			return errors.New("respaldo anterior inválido: no se borrará")
		}
		if e = removeKnownProgram(prev); e != nil {
			return fmt.Errorf("respaldo anterior protegido: %w", e)
		}
	}
	if e = atomicJSON(filepath.Join(base, pendingFile), Pending{From: from, To: incoming.AppVersion}); e != nil {
		return e
	}
	if had {
		if e = os.Rename(prog, prev); e != nil {
			return fmt.Errorf("no se pudo respaldar: %w", e)
		}
	}
	if e = os.Rename(stage, prog); e != nil {
		if had {
			_ = os.Rename(prev, prog)
		}
		return e
	}
	return nil
}

// ConfirmRelease is called ONLY after the expected app has passed a live health
// check; the last-good backup remains available for a subsequent update.
func ConfirmRelease(base, version string) error {
	p, e := getPending(base)
	if e != nil {
		return e
	}
	if p == nil || p.To != version {
		return errors.New("no hay actualización pendiente para confirmar")
	}
	rel, e := ReadAppRelease(filepath.Join(base, "program"))
	if e != nil || rel.AppVersion != version {
		return errors.New("versión activa no coincide al confirmar")
	}
	if e = os.Remove(filepath.Join(base, pendingFile)); e != nil {
		return e
	}
	_ = os.Remove(filepath.Join(base, blockedFile))
	return nil
}

// RollbackRelease restores the last validated executable on failed health check.
func RollbackRelease(base string, reason string) error {
	p, e := getPending(base)
	if e != nil || p == nil {
		return errors.New("no existe reemplazo pendiente")
	}
	prev := filepath.Join(base, ".program-previous")
	prog := filepath.Join(base, "program")
	quarantine := filepath.Join(base, ".program-quarantine")
	if !readValid(prev) {
		return errors.New("respaldo no verificable; no se eliminará la versión activa")
	}
	if e := prepareQuarantine(quarantine); e != nil {
		return fmt.Errorf("cuarentena protegida: %w", e)
	}
	if yes, _ := exists(prog); yes {
		if e = os.Rename(prog, quarantine); e != nil {
			return e
		}
	}
	if e = os.Rename(prev, prog); e != nil {
		return e
	}
	if e = block(base, p.To, reason); e != nil {
		return e
	}
	return os.Remove(filepath.Join(base, pendingFile))
}

// Safety limit on stale diagnostic files: keep one quarantined executable only.
func QuarantineInfo(base string) string {
	path := filepath.Join(base, ".program-quarantine")
	if _, e := os.Lstat(path); e == nil {
		return fmt.Sprintf("Se conservó una instalación defectuosa en %s", path)
	}
	return ""
}

var _ = strings.TrimSpace
var _ = time.Now

// RepairBroken accepts only a verified staged release and never treats a
// damaged executable as a valid rollback target.
func RepairBroken(base, stage string) error {
	if filepath.Clean(stage) != filepath.Join(base, ".app-staging") {
		return errors.New("ruta de reparación no autorizada")
	}
	incoming, e := ReadAppRelease(stage)
	if e != nil {
		return e
	}
	if p, e := getPending(base); e != nil || p != nil {
		return errors.New("reparación pendiente")
	}
	prog := filepath.Join(base, "program")
	quarantine := filepath.Join(base, ".program-quarantine")
	if e := prepareQuarantine(quarantine); e != nil {
		return fmt.Errorf("cuarentena protegida: %w", e)
	}
	if ok, _ := exists(filepath.Join(base, ".program-previous")); ok {
		return errors.New("respaldo previo existente: usar recuperación")
	}
	if ok, _ := exists(prog); ok {
		if readValid(prog) {
			return errors.New("instalación sana: no requiere reparación")
		}
	}
	if e = atomicJSON(filepath.Join(base, pendingFile), Pending{To: incoming.AppVersion}); e != nil {
		return e
	}
	if ok, _ := exists(prog); ok {
		if e = os.Rename(prog, quarantine); e != nil {
			return e
		}
	}
	return os.Rename(stage, prog)
}
