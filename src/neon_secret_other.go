//go:build !windows
package main
import("errors";"os")
func rvSecure([]byte)([]byte,error){return nil,errors.New("Credenciales cifradas requieren Windows")}
func rvReveal([]byte)([]byte,error){return nil,errors.New("Credenciales cifradas requieren Windows")}
func rvReplace(src,dst string)error{return os.Rename(src,dst)}
