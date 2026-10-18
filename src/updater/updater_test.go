package updater

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{{"0.3.5", "0.4.0", -1}, {"0.4.0", "0.4.0", 0}, {"1.0.0", "0.9.99", 1}, {"0.9.99", "0.10.0", -1}}
	for _, c := range cases {
		x, e := VersionCompare(c.a, c.b)
		if e != nil || x != c.want {
			t.Fatalf("%s vs %s %d %v", c.a, c.b, x, e)
		}
	}
}
func TestSignatureVerification(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	m := Manifest{Format: 1, Version: "0.4.0", PackageURL: "https://example.org/release.zip", PackageSHA256: strings.Repeat("a", 64), AppSHA256: strings.Repeat("b", 64)}
	m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, SignaturePayload(m)))
	pk := base64.StdEncoding.EncodeToString(pub)
	if e := Verify(m, pk); e != nil {
		t.Fatal(e)
	}
	m.Version = "0.4.1"
	if Verify(m, pk) == nil {
		t.Fatal("tampered manifest accepted")
	}
	m.Version = "0.4.0"
	m.PackageURL = "http://example.org/release.zip"
	if Verify(m, pk) == nil {
		t.Fatal("HTTP accepted")
	}
}

// Signed downloads contain only the executable and its integrity manifest. A
// damaged archive or an extra file must not be promoted to an installation.
func TestVerifiedPackageAndUpgradeRollbackData(t *testing.T) {
	dir := t.TempDir()
	app := []byte("TEST EXE - NO PATIENT DATA")
	sh := sha256.Sum256(app)
	appHash := hex.EncodeToString(sh[:])
	rel := []byte(fmt.Sprintf(`{"app_version":"0.4.0","app_name":"RevVisionApp.exe","app_sha256":"%s"}`, appHash))
	path := filepath.Join(dir, "pkg.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, data := range map[string][]byte{"RevVisionApp.exe": app, "release.json": rel} {
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
	m := Manifest{Version: "0.4.0", AppSHA256: appHash}
	stage := filepath.Join(dir, "stage")
	os.Mkdir(stage, 0700)
	if err := UnpackVerified(path, stage, m); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAppRelease(stage); err != nil {
		t.Fatal(err)
	}
	// Verify patient file is not in any folder used for the updater swap.
	patients := filepath.Join(dir, "config", "data", "revvision-local.json")
	os.MkdirAll(filepath.Dir(patients), 0700)
	os.WriteFile(patients, []byte("patient fixture remains unchanged"), 0600)
	prog := filepath.Join(dir, "program")
	os.Mkdir(prog, 0700)
	old := []byte("old program")
	os.WriteFile(filepath.Join(prog, "RevVisionApp.exe"), old, 0600)
	oldSum := sha256.Sum256(old)
	os.WriteFile(filepath.Join(prog, "release.json"), []byte(fmt.Sprintf(`{"app_version":"0.3.9","app_name":"RevVisionApp.exe","app_sha256":"%x"}`, oldSum)), 0600)
	// Use the official stage directory for promotion.
	actualStage := filepath.Join(dir, ".app-staging")
	if err := os.Rename(stage, actualStage); err != nil {
		t.Fatal(err)
	}
	if err := ActivateRelease(dir, actualStage); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(patients)
	if err != nil || string(stored) != "patient fixture remains unchanged" {
		t.Fatalf("patient data changed %v", err)
	}
	prior, err := os.ReadFile(filepath.Join(dir, ".program-previous", "RevVisionApp.exe"))
	if err != nil || string(prior) != "old program" {
		t.Fatalf("rollback snapshot missing: %v", err)
	}
}
