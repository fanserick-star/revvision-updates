package updater

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV2SignedComponentsCannotBeAltered(t *testing.T) {
	pub, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	m := Manifest{Format: 2, Version: "0.4.1", PackageURL: "https://github.com/org/repo/releases/download/v0.4.1/update.zip", PackageSHA256: strings.Repeat("a", 64), AppSHA256: strings.Repeat("b", 64), LauncherSHA256: strings.Repeat("c", 64), HelperSHA256: strings.Repeat("d", 64)}
	m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, SignaturePayload(m)))
	k := base64.StdEncoding.EncodeToString(pub)
	if e := Verify(m, k); e != nil {
		t.Fatal(e)
	}
	m.LauncherSHA256 = strings.Repeat("e", 64)
	if Verify(m, k) == nil {
		t.Fatal("altered launcher signature was accepted")
	}
	m.LauncherSHA256 = strings.Repeat("c", 64)
	m.HelperSHA256 = strings.Repeat("e", 64)
	if Verify(m, k) == nil {
		t.Fatal("altered helper accepted")
	}
	m.HelperSHA256 = strings.Repeat("d", 64)
	m.Format = 1
	if Verify(m, k) == nil {
		t.Fatal("format downgrade accepted")
	}
}
func TestV2PackageRejectsChangedLauncher(t *testing.T) {
	d := t.TempDir()
	version := "0.4.1"
	files := map[string][]byte{"RevVisionApp.exe": []byte("app"), "RevVisionLauncher.exe": []byte("launcher"), "LauncherUpdater.exe": []byte("helper")}
	hash := func(x []byte) string { b := sha256.Sum256(x); return hex.EncodeToString(b[:]) }
	rel := AppRelease{AppName: "RevVisionApp.exe", AppVersion: version, AppSHA256: hash(files["RevVisionApp.exe"]), LauncherSHA256: hash(files["RevVisionLauncher.exe"]), HelperSHA256: hash(files["LauncherUpdater.exe"])}
	b, _ := json.Marshal(rel)
	files["release.json"] = b
	zipPath := filepath.Join(d, "pkg.zip")
	f, e := os.Create(zipPath)
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	for name, data := range files {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write(data); e != nil {
			t.Fatal(e)
		}
	}
	z.Close()
	f.Close()
	m := Manifest{Format: 2, Version: version, AppSHA256: rel.AppSHA256, LauncherSHA256: rel.LauncherSHA256, HelperSHA256: rel.HelperSHA256}
	good := filepath.Join(d, "good")
	os.Mkdir(good, 0700)
	if e := UnpackVerified(zipPath, good, m); e != nil {
		t.Fatal(e)
	}
	m.LauncherSHA256 = hash([]byte("launcher2"))
	bad := filepath.Join(d, "bad")
	os.Mkdir(bad, 0700)
	if e := UnpackVerified(zipPath, bad, m); e == nil {
		t.Fatal("tampered launcher SHA accepted")
	}
}
func TestPatientOrderPaymentDataPreservedThroughInterruptedUpdate(t *testing.T) {
	d := t.TempDir()
	config := filepath.Join(d, "config", "RevVision", "Pruebas", "data")
	os.MkdirAll(config, 0700)
	secret := []byte(`{"patients":{"p1":{"name":"PACIENTE DE PRUEBA"}},"orders":{"o1":{"total_cents":10000}},"payments":{"a1":{"amount_cents":3000}}}`)
	data := filepath.Join(config, "revvision-local.json")
	os.WriteFile(data, secret, 0600)
	installFixture(t, filepath.Join(d, "program"), "0.4.0")
	installFixture(t, filepath.Join(d, ".app-staging"), "0.4.1")
	if e := ActivateRelease(d, filepath.Join(d, ".app-staging")); e != nil {
		t.Fatal(e)
	}
	if e := RecoverInterrupted(d); e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(data)
	if e != nil || string(got) != string(secret) {
		t.Fatal("clinical or commerce data changed")
	}
}
