//go:build windows
package main
import("errors";"syscall";"unsafe")
type rvDPBlob struct{ N uint32; P *byte }
func rvBlob(b []byte) rvDPBlob {if len(b)==0{return rvDPBlob{}};return rvDPBlob{uint32(len(b)),&b[0]}}
func rvCrypt(b []byte, protect bool)([]byte,error){
 if len(b)<1||len(b)>4096{return nil,errors.New("Tamaño de credencial inválido")}
 in:=rvBlob(b);var out rvDPBlob
 name:="CryptProtectData";if !protect{name="CryptUnprotectData"}
 var ok uintptr;var err error
 proc:=syscall.NewLazyDLL("crypt32.dll").NewProc(name)
 if protect{ok,_,err=proc.Call(uintptr(unsafe.Pointer(&in)),0,0,0,0,1,uintptr(unsafe.Pointer(&out)))}else{ok,_,err=proc.Call(uintptr(unsafe.Pointer(&in)),0,0,0,0,0,0,uintptr(unsafe.Pointer(&out)))}
 if ok==0{return nil,err}
 defer syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree").Call(uintptr(unsafe.Pointer(out.P)))
 if out.N==0||out.N>8192{return nil,errors.New("Credencial protegida inválida")}
 res:=make([]byte,int(out.N));copy(res,unsafe.Slice(out.P,int(out.N)));return res,nil
}
func rvSecure(b []byte)([]byte,error){return rvCrypt(b,true)}
func rvReveal(b []byte)([]byte,error){return rvCrypt(b,false)}
func rvReplace(src,dst string)error{
 a,e:=syscall.UTF16PtrFromString(src);if e!=nil{return e}
 b,e:=syscall.UTF16PtrFromString(dst);if e!=nil{return e}
 ok,_,e:=syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW").Call(uintptr(unsafe.Pointer(a)),uintptr(unsafe.Pointer(b)),uintptr(1|8))
 if ok==0{return e};return nil
}