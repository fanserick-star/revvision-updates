package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Regression test: when the local server is already running, pressing the
// desktop shortcut must reopen the dedicated app window instead of exiting.
func TestReopenWindowUsesDedicatedEdgeApp(t *testing.T) {
	root := t.TempDir()
	edge := filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe")
	if err := os.MkdirAll(filepath.Dir(edge), 0755); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(root, "edge_stub.go")
	source := `package main
import("os";"strings")
func main(){ _ = os.WriteFile(os.Getenv("REVVISION_EDGE_TEST_MARKER"),[]byte(strings.Join(os.Args[1:],"\\n")),0600) }
`
	if err := os.WriteFile(stub, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", edge, stub)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("stub Edge: %s: %v", output, err)
	}
	appdata := filepath.Join(root, "local")
	marker := filepath.Join(root, "edge-args.txt")
	t.Setenv("REVVISION_EDGE_TEST_MARKER", marker)
	t.Setenv("LOCALAPPDATA", appdata)
	t.Setenv("PROGRAMFILES(X86)", root)
	t.Setenv("PROGRAMFILES", filepath.Join(root, "other-program-files"))
	if err := reopenWindow(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(marker)
		if err == nil {
			args := string(b)
			for _, expected := range []string{"--app=http://127.0.0.1:58437", "--user-data-dir=" + filepath.Join(appdata, "RevVision", "Pruebas", "edge-window-profile"), "--disable-background-mode"} {
				if !strings.Contains(args, expected) {
					t.Fatalf("Edge missing %q: %s", expected, args)
				}
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Edge app mode was never launched")
}
