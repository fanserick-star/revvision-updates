package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestThermalReceiptContents(t *testing.T) {
	order := Order{ID: "aabbccddeeff", TotalCents: 5700, Lines: []OrderLine{{Name: "Montura prueba", Quantity: 1, SubtotalCents: 5700}}}
	patient := Patient{FullName: "PEÑA ÁLVAREZ MARÍA", DocumentID: "1234567890"}
	payment := Payment{ReceiptNumber: "RV-A-00001", PaidAt: "2026-10-17T11:30:00Z", Method: "EFECTIVO", AmountCents: 2000}
	b := receiptBytes(order, patient, payment, 1000, true)
	for _, v := range []string{"PENA ALVAREZ MARIA", "RV-A-00001", "ABONO RECIBIDO", "$20.00", "$27.00", "REIMPRESION DEL MISMO ABONO", "NO ES FACTURA"} {
		if !bytes.Contains(b, []byte(v)) {
			t.Fatalf("no contiene %s", v)
		}
	}
	if !bytes.HasSuffix(b, []byte("\x1dVB\x00")) {
		t.Fatal("faltó el comando de corte")
	}
	if bytes.Contains(b, []byte("\x1b[31m")) {
		t.Fatal("código inesperado")
	}
}
func TestThermalSanitizeUnsafeInput(t *testing.T) {
	txt := printableASCII("ANA\x1b@\nPALACIOS")
	if strings.Contains(txt, "\x1b") || strings.Contains(txt, "\n") {
		t.Fatal("control code injection")
	}
	if len(receiptLine("Paciente", strings.Repeat("X", 180))) < 180 {
		t.Fatal("texto desaparecido")
	}
}
func TestThermalTestOnly(t *testing.T) {
	b := testReceiptBytes()
	if !bytes.Contains(b, []byte("NO REGISTRA PAGOS")) || !bytes.HasSuffix(b, []byte("\x1dVB\x00")) {
		t.Fatal("prueba incompleta")
	}
}
