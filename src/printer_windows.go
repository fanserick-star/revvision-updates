//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var winspool = syscall.NewLazyDLL("winspool.drv")
var enumPrinters = winspool.NewProc("EnumPrintersW")
var openPrinter = winspool.NewProc("OpenPrinterW")
var closePrinter = winspool.NewProc("ClosePrinter")
var startDoc = winspool.NewProc("StartDocPrinterW")
var endDoc = winspool.NewProc("EndDocPrinter")
var startPage = winspool.NewProc("StartPagePrinter")
var endPage = winspool.NewProc("EndPagePrinter")
var writePrinter = winspool.NewProc("WritePrinter")

type winPrinterInfo4 struct {
	Name       *uint16
	Server     *uint16
	Attributes uint32
}
type winDocInfo1 struct {
	Name   *uint16
	Output *uint16
	Type   *uint16
}

func fromUTF16Ptr(p *uint16) string {
	if p == nil {
		return ""
	}
	out := make([]uint16, 0, 150)
	for i := 0; i < 512; i++ {
		c := *(*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + uintptr(i*2)))
		if c == 0 {
			break
		}
		out = append(out, c)
	}
	return syscall.UTF16ToString(out)
}
func getInstalledPrinters() ([]string, error) {
	const flags = 2 | 4 // PRINTER_ENUM_LOCAL | PRINTER_ENUM_CONNECTIONS
	var size, count uint32
	_, _, _ = enumPrinters.Call(flags, 0, 4, 0, 0, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)))
	if size == 0 {
		return []string{}, nil
	}
	buf := make([]byte, size)
	ok, _, err := enumPrinters.Call(flags, 0, 4, uintptr(unsafe.Pointer(&buf[0])), uintptr(size), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)))
	if ok == 0 {
		return nil, fmt.Errorf("EnumPrintersW: %v", err)
	}
	out := make([]string, 0, count)
	step := unsafe.Sizeof(winPrinterInfo4{})
	for i := uint32(0); i < count; i++ {
		obj := (*winPrinterInfo4)(unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0])) + uintptr(i)*step))
		if name := fromUTF16Ptr(obj.Name); name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}
func checkedResult(n uintptr, e error, method string) error {
	if n != 0 {
		return nil
	}
	return fmt.Errorf("%s: %v", method, e)
}
func sendRawToPrinter(name string, payload []byte) error {
	if len(payload) == 0 {
		return fmt.Errorf("ticket vacío")
	}
	ptr, e := syscall.UTF16PtrFromString(name)
	if e != nil {
		return e
	}
	var handle syscall.Handle
	n, _, callErr := openPrinter.Call(uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(&handle)), 0)
	if err := checkedResult(n, callErr, "OpenPrinter"); err != nil {
		return err
	}
	defer closePrinter.Call(uintptr(handle))
	title, _ := syscall.UTF16PtrFromString("Rev Vision - Ticket termico 80mm")
	raw, _ := syscall.UTF16PtrFromString("RAW")
	doc := winDocInfo1{Name: title, Type: raw}
	id, _, docErr := startDoc.Call(uintptr(handle), 1, uintptr(unsafe.Pointer(&doc)))
	if err := checkedResult(id, docErr, "StartDocPrinter"); err != nil {
		return err
	}
	defer endDoc.Call(uintptr(handle))
	n, _, pageErr := startPage.Call(uintptr(handle))
	if err := checkedResult(n, pageErr, "StartPagePrinter"); err != nil {
		return err
	}
	defer endPage.Call(uintptr(handle))
	var written uint32
	n, _, writeErr := writePrinter.Call(uintptr(handle), uintptr(unsafe.Pointer(&payload[0])), uintptr(len(payload)), uintptr(unsafe.Pointer(&written)))
	if err := checkedResult(n, writeErr, "WritePrinter"); err != nil {
		return err
	}
	if written != uint32(len(payload)) {
		return fmt.Errorf("trabajo incompleto: %d/%d bytes", written, len(payload))
	}
	return nil
}
