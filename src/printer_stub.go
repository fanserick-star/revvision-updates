//go:build !windows

package main

func getInstalledPrinters() ([]string, error)   { return nil, errPrinterUnavailable }
func sendRawToPrinter(_ string, _ []byte) error { return errPrinterUnavailable }
