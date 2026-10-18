// Package updater validates and installs signed, versioned Rev Vision application releases.
// It never accesses, changes or deletes patient data.
package updater

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const MaxPackageSize int64 = 45 << 20
const MaxAppSize int64 = 35 << 20

var versionPattern = regexp.MustCompile(`^\d{1,5}\.\d{1,5}\.\d{1,5}$`)
var shaPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Manifest struct {
	Format         int    `json:"format"`
	Version        string `json:"version"`
	PackageURL     string `json:"package_url"`
	PackageSHA256  string `json:"package_sha256"`
	AppSHA256      string `json:"app_sha256"`
	Signature      string `json:"signature"`
	LauncherSHA256 string `json:"launcher_sha256,omitempty"`
	HelperSHA256   string `json:"helper_sha256,omitempty"`
}
type AppRelease struct {
	AppVersion     string `json:"app_version"`
	AppName        string `json:"app_name"`
	AppSHA256      string `json:"app_sha256"`
	LauncherSHA256 string `json:"launcher_sha256,omitempty"`
	HelperSHA256   string `json:"helper_sha256,omitempty"`
}

func SignaturePayload(m Manifest) []byte {
	if m.Format == 2 {
		return []byte(fmt.Sprintf("revvision-update-v2\n%d\n%s\n%s\n%s\n%s\n%s\n%s\n", m.Format, m.Version, m.PackageURL, m.PackageSHA256, m.AppSHA256, m.LauncherSHA256, m.HelperSHA256))
	}
	return []byte(fmt.Sprintf("revvision-update-v1\n%d\n%s\n%s\n%s\n%s\n", m.Format, m.Version, m.PackageURL, m.PackageSHA256, m.AppSHA256))
}
func VersionCompare(a, b string) (int, error) {
	if !versionPattern.MatchString(a) || !versionPattern.MatchString(b) {
		return 0, errors.New("versión inválida")
	}
	x := strings.Split(a, ".")
	y := strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		u, _ := strconv.Atoi(x[i])
		v, _ := strconv.Atoi(y[i])
		if u < v {
			return -1, nil
		}
		if u > v {
			return 1, nil
		}
	}
	return 0, nil
}
func Verify(m Manifest, publicKeyB64 string) error {
	if (m.Format != 1 && m.Format != 2) || !versionPattern.MatchString(m.Version) || !shaPattern.MatchString(m.PackageSHA256) || !shaPattern.MatchString(m.AppSHA256) {
		return errors.New("formato de manifiesto inválido")
	}
	if m.Format == 2 && (!shaPattern.MatchString(m.LauncherSHA256) || !shaPattern.MatchString(m.HelperSHA256)) {
		return errors.New("firmas de componentes del launcher incompletas")
	}
	parsed, err := url.Parse(m.PackageURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("dirección HTTPS de paquete inválida")
	}
	pk, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(pk) != ed25519.PublicKeySize {
		return errors.New("clave pública no configurada")
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(ed25519.PublicKey(pk), SignaturePayload(m), sig) {
		return errors.New("firma de actualización no válida")
	}
	return nil
}
func FileHash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func ReadAppRelease(dir string) (AppRelease, error) {
	var a AppRelease
	b, e := os.ReadFile(filepath.Join(dir, "release.json"))
	if e != nil {
		return a, e
	}
	if e = json.Unmarshal(b, &a); e != nil {
		return a, e
	}
	if a.AppName != "RevVisionApp.exe" || !shaPattern.MatchString(a.AppSHA256) || !versionPattern.MatchString(a.AppVersion) {
		return a, errors.New("release local inválido")
	}
	actual, e := FileHash(filepath.Join(dir, a.AppName))
	if e != nil {
		return a, e
	}
	if actual != a.AppSHA256 {
		return a, errors.New("aplicación local alterada; no se ejecutará")
	}
	return a, nil
}
func client() *http.Client {
	return &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || len(via) > 5 {
			return errors.New("redirección HTTPS inválida")
		}
		return nil
	}}
}
func ReadRemoteManifest(urlStr string) (Manifest, error) {
	var m Manifest
	u, e := url.Parse(urlStr)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return m, errors.New("el canal debe usar HTTPS")
	}
	req, e := http.NewRequest(http.MethodGet, urlStr, nil)
	if e != nil {
		return m, e
	}
	req.Header.Set("Accept", "application/json")
	c := client()
	c.Timeout = 5 * time.Second
	resp, e := c.Do(req)
	if e != nil {
		return m, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return m, fmt.Errorf("servidor de actualizaciones: HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 16*1024+1))
	if e != nil {
		return m, e
	}
	if len(b) > 16*1024 {
		return m, errors.New("manifiesto demasiado grande")
	}
	if e = json.Unmarshal(b, &m); e != nil {
		return m, e
	}
	return m, nil
}
func DownloadPackage(m Manifest, dest string) error {
	req, e := http.NewRequest(http.MethodGet, m.PackageURL, nil)
	if e != nil {
		return e
	}
	resp, e := client().Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("descarga: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxPackageSize {
		return errors.New("paquete demasiado grande")
	}
	f, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, MaxPackageSize+1))
	if e != nil {
		return e
	}
	if n > MaxPackageSize {
		return errors.New("paquete excede tamaño permitido")
	}
	if hex.EncodeToString(h.Sum(nil)) != m.PackageSHA256 {
		return errors.New("SHA-256 del paquete no coincide")
	}
	return f.Sync()
}
func UnpackVerified(zipPath, stage string, m Manifest) error {
	z, e := zip.OpenReader(zipPath)
	if e != nil {
		return e
	}
	defer z.Close()
	needed := 2
	if m.Format == 2 {
		needed = 4
	}
	if len(z.File) != needed {
		return errors.New("contenido del paquete no reconocido")
	}
	allowed := map[string]bool{"RevVisionApp.exe": false, "release.json": false}
	if m.Format == 2 {
		allowed["RevVisionLauncher.exe"] = false
		allowed["LauncherUpdater.exe"] = false
	}
	for _, zf := range z.File {
		if _, ok := allowed[zf.Name]; !ok || zf.FileInfo().IsDir() {
			return fmt.Errorf("archivo inesperado: %s", zf.Name)
		}
		if allowed[zf.Name] {
			return errors.New("archivo duplicado en paquete")
		}
		allowed[zf.Name] = true
		max := MaxAppSize
		if zf.Name == "release.json" {
			max = 4096
		}
		if zf.UncompressedSize64 > uint64(max) {
			return errors.New("archivo interno excesivo")
		}
		r, e := zf.Open()
		if e != nil {
			return e
		}
		out, e := os.OpenFile(filepath.Join(stage, zf.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			r.Close()
			return e
		}
		n, e := io.Copy(out, io.LimitReader(r, max+1))
		closeErr := out.Close()
		r.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if n > max {
			return errors.New("archivo interno demasiado grande")
		}
	}
	for _, v := range allowed {
		if !v {
			return errors.New("falta un archivo")
		}
	}
	rel, e := ReadAppRelease(stage)
	if e != nil {
		return e
	}
	if rel.AppVersion != m.Version || rel.AppSHA256 != m.AppSHA256 {
		return errors.New("release no coincide con manifiesto firmado")
	}
	if m.Format == 2 {
		if rel.LauncherSHA256 != m.LauncherSHA256 || rel.HelperSHA256 != m.HelperSHA256 {
			return errors.New("SHA del launcher o auxiliar no coincide con manifiesto firmado")
		}
		for _, item := range []struct{ name, sha string }{{"RevVisionLauncher.exe", m.LauncherSHA256}, {"LauncherUpdater.exe", m.HelperSHA256}} {
			h, e := FileHash(filepath.Join(stage, item.name))
			if e != nil || h != item.sha {
				return fmt.Errorf("integridad de %s falló", item.name)
			}
		}
	}
	return nil
}

// StageRemoteRelease downloads to a private staging folder; interrupted attempts
// are safe to remove ONLY if their contents match the known updater-owned paths.
func StageRemoteRelease(base string, m Manifest) (string, error) {
	if err := removeKnownStage(base); err != nil {
		return "", err
	}
	staging := filepath.Join(base, ".app-staging")
	if err := os.Mkdir(staging, 0700); err != nil {
		return "", err
	}
	zipFile := filepath.Join(base, ".download-release.zip")
	success := false
	defer func() {
		if !success {
			_ = removeKnownStage(base)
		} else {
			_ = os.Remove(zipFile)
		}
	}()
	if err := DownloadPackage(m, zipFile); err != nil {
		return "", err
	}
	if err := UnpackVerified(zipFile, staging, m); err != nil {
		return "", err
	}
	success = true
	return staging, nil
}

func VerifyLocalPackage(buf []byte, version, appHash string) error { // Used by tests and build tooling.
	if len(buf) > int(MaxPackageSize) {
		return errors.New("demasiado grande")
	}
	d, err := os.MkdirTemp("", "revvision-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(d)
	p := filepath.Join(d, "candidate.zip")
	if err = os.WriteFile(p, buf, 0600); err != nil {
		return err
	}
	m := Manifest{Version: version, AppSHA256: appHash}
	return UnpackVerified(p, d, m)
}
func ParseManifest(b []byte) (Manifest, error) {
	var m Manifest
	err := json.NewDecoder(bytes.NewReader(b)).Decode(&m)
	return m, err
}
