package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func installFixture(t *testing.T, dir, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	app := []byte("fixture:" + version)
	sum := sha256.Sum256(app)
	rel := AppRelease{AppName: "RevVisionApp.exe", AppVersion: version, AppSHA256: hex.EncodeToString(sum[:])}
	b, _ := json.Marshal(rel)
	if err := os.WriteFile(filepath.Join(dir, "RevVisionApp.exe"), app, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "release.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}
func installed(t *testing.T, base string) string {
	t.Helper()
	v, e := ReadAppRelease(filepath.Join(base, "program"))
	if e != nil {
		t.Fatal(e)
	}
	return v.AppVersion
}
func TestUpdateSuccessAndVersionBlock(t *testing.T) {
	d := t.TempDir()
	installFixture(t, filepath.Join(d, "program"), "0.4.0")
	installFixture(t, filepath.Join(d, ".app-staging"), "0.4.1")
	if e := ActivateRelease(d, filepath.Join(d, ".app-staging")); e != nil {
		t.Fatal(e)
	}
	if got := installed(t, d); got != "0.4.1" {
		t.Fatal(got)
	}
	if e := ConfirmRelease(d, "0.4.1"); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(d, ".program-previous", "RevVisionApp.exe")); e != nil {
		t.Fatal(e)
	}
	if e := RecoverInterrupted(d); e != nil {
		t.Fatal(e)
	}
	if got := installed(t, d); got != "0.4.1" {
		t.Fatal(got)
	}
	installFixture(t, filepath.Join(d, ".app-staging"), "0.4.0")
	if e := ActivateRelease(d, filepath.Join(d, ".app-staging")); e == nil {
		t.Fatal("accepted downgrade")
	}
}
func TestInterruptedPreRenameClearsPending(t *testing.T) {
	d := t.TempDir()
	installFixture(t, filepath.Join(d, "program"), "0.4.0")
	if e := atomicJSON(filepath.Join(d, pendingFile), Pending{From: "0.4.0", To: "0.4.1"}); e != nil {
		t.Fatal(e)
	}
	installFixture(t, filepath.Join(d, ".app-staging"), "0.4.1")
	if e := os.WriteFile(filepath.Join(d, ".download-release.zip"), []byte("incomplete"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := RecoverInterrupted(d); e != nil {
		t.Fatal(e)
	}
	if installed(t, d) != "0.4.0" {
		t.Fatal("old program lost")
	}
	if _, e := os.Stat(filepath.Join(d, pendingFile)); !os.IsNotExist(e) {
		t.Fatal("pending marker not removed")
	}
}
func TestInterruptedBetweenRenamesRestoresOld(t *testing.T) {
	d := t.TempDir()
	installFixture(t, filepath.Join(d, ".program-previous"), "0.4.0")
	if e := atomicJSON(filepath.Join(d, pendingFile), Pending{From: "0.4.0", To: "0.4.1"}); e != nil {
		t.Fatal(e)
	}
	if e := RecoverInterrupted(d); e != nil {
		t.Fatal(e)
	}
	if installed(t, d) != "0.4.0" || !BlockedVersion(d, "0.4.1") {
		t.Fatal("old version not restored or new not blocked")
	}
}
func TestFailedHealthRollbackAndIdempotency(t *testing.T) {
	d := t.TempDir()
	installFixture(t, filepath.Join(d, "program"), "0.4.0")
	installFixture(t, filepath.Join(d, ".app-staging"), "0.4.1")
	if e := ActivateRelease(d, filepath.Join(d, ".app-staging")); e != nil {
		t.Fatal(e)
	}
	if e := RollbackRelease(d, "prueba de salud fallida"); e != nil {
		t.Fatal(e)
	}
	if installed(t, d) != "0.4.0" || !BlockedVersion(d, "0.4.1") {
		t.Fatal("rollback failure")
	}
	if e := RecoverInterrupted(d); e != nil {
		t.Fatal(e)
	}
	if installed(t, d) != "0.4.0" {
		t.Fatal("second recovery altered program")
	}
}
func TestCrashAfterPromotionBeforeHealthRestores(t *testing.T) {
	d := t.TempDir()
	installFixture(t, filepath.Join(d, "program"), "0.4.0")
	installFixture(t, filepath.Join(d, ".app-staging"), "0.4.1")
	if e := ActivateRelease(d, filepath.Join(d, ".app-staging")); e != nil {
		t.Fatal(e)
	}
	if e := RecoverInterrupted(d); e != nil {
		t.Fatal(e)
	}
	if installed(t, d) != "0.4.0" {
		t.Fatal("did not recover")
	}
}
func TestDamagedLocalRestoredFromKnownGood(t *testing.T) {
	d := t.TempDir()
	installFixture(t, filepath.Join(d, "program"), "0.4.1")
	installFixture(t, filepath.Join(d, ".program-previous"), "0.4.0")
	os.WriteFile(filepath.Join(d, "program", "RevVisionApp.exe"), []byte("corrupt"), 0600)
	if e := RecoverInterrupted(d); e != nil {
		t.Fatal(e)
	}
	if installed(t, d) != "0.4.0" {
		t.Fatal("did not restore validated backup")
	}
	if _, e := os.Stat(filepath.Join(d, ".program-quarantine", "RevVisionApp.exe")); e != nil {
		t.Fatal(e)
	}
}
func TestUnknownPendingContentIsNeverDeleted(t *testing.T) {
	d := t.TempDir()
	os.Mkdir(filepath.Join(d, ".app-staging"), 0700)
	os.WriteFile(filepath.Join(d, ".app-staging", "patients.json"), []byte("MUST PRESERVE"), 0600)
	if e := RecoverInterrupted(d); e == nil {
		t.Fatal("unexpected folder content accepted")
	}
	if _, e := os.Stat(filepath.Join(d, ".app-staging", "patients.json")); e != nil {
		t.Fatal("deleted unknown file")
	}
}
func TestDamagedApplicationWithoutBackupDoesNotExecute(t *testing.T) {
	d := t.TempDir()
	installFixture(t, filepath.Join(d, "program"), "0.4.1")
	os.WriteFile(filepath.Join(d, "program", "RevVisionApp.exe"), []byte("damaged"), 0600)
	if e := RecoverInterrupted(d); e != nil {
		t.Fatal(e)
	}
	if _, e := ReadAppRelease(filepath.Join(d, "program")); e == nil {
		t.Fatal("corrupt binary trusted")
	}
}
func TestConcurrentActivationStateIsRejected(t *testing.T) {
	d := t.TempDir()
	installFixture(t, filepath.Join(d, "program"), "0.4.0")
	installFixture(t, filepath.Join(d, ".app-staging"), "0.4.1")
	if e := ActivateRelease(d, filepath.Join(d, ".app-staging")); e != nil {
		t.Fatal(e)
	}
	installFixture(t, filepath.Join(d, ".app-staging"), "0.4.2")
	if e := ActivateRelease(d, filepath.Join(d, ".app-staging")); e == nil {
		t.Fatal("concurrent activation accepted")
	}
}

func TestFirstInstallPendingCanBeHealthCheckedLater(t *testing.T) {
	d := t.TempDir()
	installFixture(t, filepath.Join(d, ".app-staging"), "0.4.1")
	if e := ActivateRelease(d, filepath.Join(d, ".app-staging")); e != nil {
		t.Fatal(e)
	}
	if e := RecoverInterrupted(d); e != nil {
		t.Fatalf("first install was blocked: %v", e)
	}
	if e := ConfirmRelease(d, "0.4.1"); e != nil {
		t.Fatal(e)
	}
	if installed(t, d) != "0.4.1" {
		t.Fatal("first install lost")
	}
}

func TestQuarantineRefusesUnknownClinicalFile(t *testing.T) {
	d := t.TempDir()
	installFixture(t, filepath.Join(d, "program"), "0.4.0")
	installFixture(t, filepath.Join(d, ".app-staging"), "0.4.1")
	if e := ActivateRelease(d, filepath.Join(d, ".app-staging")); e != nil {
		t.Fatal(e)
	}
	q := filepath.Join(d, ".program-quarantine")
	os.Mkdir(q, 0700)
	os.WriteFile(filepath.Join(q, "patients.json"), []byte("keep"), 0600)
	if e := RollbackRelease(d, "failure"); e == nil {
		t.Fatal("deleted unknown quarantined data")
	}
	if _, e := os.Stat(filepath.Join(q, "patients.json")); e != nil {
		t.Fatal("patient data lost")
	}
}
