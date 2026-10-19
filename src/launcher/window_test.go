package main

import (
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"
    "syscall"
    "testing"
    "time"
    "unsafe"
)

func buildEdgeStub(t *testing.T, root string) string {
    t.Helper()
    edge:=filepath.Join(root,"Microsoft","Edge","Application","msedge.exe")
    if err:=os.MkdirAll(filepath.Dir(edge),0755);err!=nil{t.Fatal(err)}
    source:=`package main
import("os";"strings";"syscall";"time";"unsafe")
func main(){
    _=os.WriteFile(os.Getenv("REVVISION_EDGE_TEST_MARKER"),[]byte(strings.Join(os.Args[1:],"\\n")),0600)
    usr:=syscall.NewLazyDLL("user32.dll")
    klass,_:=syscall.UTF16PtrFromString("STATIC")
    title,_:=syscall.UTF16PtrFromString("Rev Visión | Historia Clínica")
    hwnd,_,_:=usr.NewProc("CreateWindowExW").Call(0,uintptr(unsafe.Pointer(klass)),uintptr(unsafe.Pointer(title)),0x10CF0000,10,10,300,150,0,0,0,0)
    stop:=os.Getenv("REVVISION_EDGE_TEST_EXIT")
    until:=time.Now().Add(7*time.Second)
    for time.Now().Before(until) {
        if _,e:=os.Stat(stop);e==nil {break}
        time.Sleep(50*time.Millisecond)
    }
    if hwnd!=0 {usr.NewProc("DestroyWindow").Call(hwnd)}
}
`
    path:=filepath.Join(root,"edge_stub.go")
    if err:=os.WriteFile(path,[]byte(source),0600);err!=nil{t.Fatal(err)}
    command:=exec.Command("go","build","-o",edge,path)
    if out,err:=command.CombinedOutput();err!=nil{t.Fatalf("stub Edge build: %s: %v",out,err)}
    return edge
}
func TestReopenWindowUsesDedicatedEdgeApp(t *testing.T) {
    root:=t.TempDir()
    edge:=buildEdgeStub(t,root)
    stop:=filepath.Join(root,"edge-exit.txt")
    t.Setenv("REVVISION_EDGE_TEST_EXIT",stop)
    t.Cleanup(func(){
        _=os.WriteFile(stop,[]byte("exit"),0600)
        // Give the simulated Edge process time to exit before TempDir deletes
        // its executable (Windows locks running .exe files).
        for i:=0;i<100;i++ {
            err:=os.Remove(edge)
            if err==nil||os.IsNotExist(err){return}
            time.Sleep(80*time.Millisecond)
        }
        t.Error("The fake Edge process did not close after test")
    })
    marker:=filepath.Join(root,"edge-args.txt")
    appdata:=filepath.Join(root,"local")
    t.Setenv("REVVISION_EDGE_TEST_MARKER",marker)
    t.Setenv("LOCALAPPDATA",appdata)
    t.Setenv("PROGRAMFILES(X86)",root)
    t.Setenv("PROGRAMFILES",filepath.Join(root,"other-program-files"))
    if err:=reopenWindow();err!=nil{t.Fatal(err)}
    args,err:=os.ReadFile(marker)
    if err!=nil{t.Fatal(err)}
    for _,want:=range []string{"--app=http://127.0.0.1:58437","--user-data-dir="+filepath.Join(appdata,"RevVision","Pruebas","edge-window-profile"),"--disable-background-mode"}{
        if !strings.Contains(string(args),want){t.Errorf("Edge launch missing %q in %q",want,string(args))}
    }
}
func TestReopenWindowFocusesAlreadyVisibleInsteadOfSpawning(t *testing.T) {
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()
    user:=syscall.NewLazyDLL("user32.dll")
    klass,_:=syscall.UTF16PtrFromString("STATIC")
    title,_:=syscall.UTF16PtrFromString("Rev Visión | Historia Clínica")
    hwnd,_,err:=user.NewProc("CreateWindowExW").Call(0,uintptr(unsafe.Pointer(klass)),uintptr(unsafe.Pointer(title)),0x10CF0000,10,10,300,150,0,0,0,0)
    if hwnd==0 {t.Skipf("No test desktop: %v",err)}
    defer user.NewProc("DestroyWindow").Call(hwnd)
    root:=t.TempDir()
    marker:=filepath.Join(root,"edge-args.txt")
    t.Setenv("REVVISION_EDGE_TEST_MARKER",marker)
    // Edge is deliberately NOT installed in this fake environment: reopening
    // an already visible window must succeed without starting any executable.
    t.Setenv("PROGRAMFILES(X86)",filepath.Join(root,"none"))
    t.Setenv("PROGRAMFILES",filepath.Join(root,"none2"))
    if e:=reopenWindow();e!=nil{t.Fatal(e)}
    time.Sleep(250*time.Millisecond)
    if _,e:=os.Stat(marker);e==nil {t.Fatal("Unnecessary second Edge instance spawned")}
}
