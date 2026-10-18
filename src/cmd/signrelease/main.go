// Offline signing utility. The private key must NEVER be stored with application code.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"revvision/updater"
	"strings"
)

func run() error {
	if len(os.Args) < 2 {
		return errors.New("use genkey <private-path> | pubkey <private-path> | sign <private-path> <version> <https-url> <zip> <app> <launcher> <helper> <out-manifest>")
	}
	switch os.Args[1] {
	case "genkey":
		if len(os.Args) != 3 {
			return errors.New("falta ruta de clave")
		}
		_, private, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return e
		}
		f, e := os.OpenFile(os.Args[2], os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = f.WriteString(base64.StdEncoding.EncodeToString(private) + "\n")
		return e
	case "pubkey":
		if len(os.Args) != 3 {
			return errors.New("falta clave privada")
		}
		private, e := loadPrivate(os.Args[2])
		if e != nil {
			return e
		}
		fmt.Println(base64.StdEncoding.EncodeToString(private.Public().(ed25519.PublicKey)))
		return nil
	case "sign":
		if len(os.Args) != 10 {
			return errors.New("sign <private-path> <version> <https-url> <zip> <app> <launcher> <helper> <out-manifest>")
		}
		private, e := loadPrivate(os.Args[2])
		if e != nil {
			return e
		}
		z, e := os.ReadFile(os.Args[5])
		if e != nil {
			return e
		}
		if len(z) > int(updater.MaxPackageSize) {
			return errors.New("ZIP demasiado grande")
		}
		appHash, e := updater.FileHash(os.Args[6])
		if e != nil {
			return e
		}
		zipSum := sha256.Sum256(z)
		launcherHash, e := updater.FileHash(os.Args[7])
		if e != nil {
			return e
		}
		helperHash, e := updater.FileHash(os.Args[8])
		if e != nil {
			return e
		}
		m := updater.Manifest{Format: 2, Version: os.Args[3], PackageURL: os.Args[4], PackageSHA256: hex.EncodeToString(zipSum[:]), AppSHA256: appHash, LauncherSHA256: launcherHash, HelperSHA256: helperHash}
		m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, updater.SignaturePayload(m)))
		pub := base64.StdEncoding.EncodeToString(private.Public().(ed25519.PublicKey))
		if e = updater.Verify(m, pub); e != nil {
			return e
		}
		b, e := json.MarshalIndent(m, "", "  ")
		if e != nil {
			return e
		}
		return os.WriteFile(os.Args[9], append(b, '\n'), 0644)
	}
	return errors.New("comando desconocido")
}
func loadPrivate(path string) (ed25519.PrivateKey, error) {
	var k ed25519.PrivateKey
	b, e := os.ReadFile(filepath.Clean(path))
	if e != nil {
		return nil, e
	}
	data, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if e != nil {
		return nil, e
	}
	if len(data) != ed25519.PrivateKeySize {
		return nil, errors.New("longitud de clave inválida")
	}
	return append(k, data...), nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
