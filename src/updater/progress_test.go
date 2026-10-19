package updater

import (
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "testing"
)
func TestDownloadProgressRealBytes(t *testing.T) {
    payload:=bytes.Repeat([]byte("RevVisionSignedUpdate"),48000)
    srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        w.Header().Set("Content-Length","1008000")
        _,_=w.Write(payload)
    }))
    defer srv.Close()
    sum:=sha256.Sum256(payload)
    m:=Manifest{PackageURL:srv.URL,PackageSHA256:hex.EncodeToString(sum[:])}
    path:=filepath.Join(t.TempDir(),"package.zip")
    var calls int
    var lastDone,lastTotal int64
    err:=DownloadPackageWithProgress(m,path,func(done,total int64){
        calls++
        if done<lastDone {t.Errorf("progress moved backward")}
        lastDone,lastTotal=done,total
    })
    if err!=nil {t.Fatal(err)}
    if calls<2 {t.Fatalf("expected multiple progress calls, got %d",calls)}
    if lastDone!=int64(len(payload))||lastTotal!=int64(len(payload)) {
        t.Fatalf("not true byte progress: %d/%d actual %d",lastDone,lastTotal,len(payload))
    }
    b,err:=os.ReadFile(path)
    if err!=nil||!bytes.Equal(b,payload){t.Fatal("download differs from source")}
}
func TestProgressRejectsBadSHA(t *testing.T) {
    srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){_,_=w.Write([]byte("bad bytes"))}))
    defer srv.Close()
    path:=filepath.Join(t.TempDir(),"invalid.zip")
    m:=Manifest{PackageURL:srv.URL,PackageSHA256:hex.EncodeToString(make([]byte,32))}
    if err:=DownloadPackageWithProgress(m,path,nil);err==nil {t.Fatal("download was accepted despite bad SHA")}
}
