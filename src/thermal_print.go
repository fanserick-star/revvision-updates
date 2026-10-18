package main

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Impresión térmica RAW ESC/POS, independiente de Edge y su tamaño de papel A4.
// NUNCA cambia importes, pagos ni inventario; solo prepara datos ya guardados.
type thermalPrintRequest struct {
	Printer   string `json:"printer"`
	PaymentID string `json:"payment_id"`
	Copy      bool   `json:"copy"`
}

func printableASCII(s string) string {
	replacements := map[rune]string{'á': "a", 'à': "a", 'ä': "a", 'â': "a", 'Á': "A", 'À': "A", 'Ä': "A", 'Â': "A", 'é': "e", 'è': "e", 'ë': "e", 'ê': "e", 'É': "E", 'È': "E", 'Ë': "E", 'Ê': "E", 'í': "i", 'ì': "i", 'ï': "i", 'Í': "I", 'Ì': "I", 'Ï': "I", 'ó': "o", 'ò': "o", 'ö': "o", 'Ó': "O", 'Ò': "O", 'Ö': "O", 'ú': "u", 'ù': "u", 'ü': "u", 'Ú': "U", 'Ù': "U", 'Ü': "U", 'ñ': "n", 'Ñ': "N", 'ç': "c", 'Ç': "C"}
	var b strings.Builder
	for _, r := range s {
		if replacement, ok := replacements[r]; ok {
			b.WriteString(replacement)
			continue
		}
		if r >= 32 && r <= 126 {
			b.WriteRune(r)
			continue
		}
		if unicode.IsSpace(r) {
			b.WriteByte(' ')
			continue
		}
		// Evita códigos de control ESC/POS inyectados desde datos de pacientes.
		b.WriteByte('?')
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
func receiptCents(v int64) string { return fmt.Sprintf("$%d.%02d", v/100, v%100) }

const paperColumns = 40 // 80 mm: margen de seguridad para no cortar caracteres a la derecha.
func receiptLine(label, value string) string {
	label = printableASCII(label)
	value = printableASCII(value)
	chunks := []string{}
	remaining := strings.TrimSpace(label + ": " + value)
	for len(remaining) > paperColumns {
		k := strings.LastIndex(remaining[:paperColumns+1], " ")
		if k < 8 {
			k = paperColumns
		}
		chunks = append(chunks, strings.TrimSpace(remaining[:k]))
		remaining = strings.TrimSpace(remaining[k:])
	}
	if remaining != "" {
		chunks = append(chunks, remaining)
	}
	return strings.Join(chunks, "\n") + "\n"
}
func receiptAligned(label, amount string) string {
	label = printableASCII(label)
	amount = printableASCII(amount)
	if len(label)+len(amount)+1 > paperColumns {
		return receiptLine(label, amount)
	}
	return label + strings.Repeat(" ", paperColumns-len(label)-len(amount)) + amount + "\n"
}
func centerTicketLine(s string) string {
	s = printableASCII(s)
	if len(s) > paperColumns {
		s = s[:paperColumns]
	}
	return strings.Repeat(" ", (paperColumns-len(s))/2) + s + "\n"
}
func receiptDate(value string) string {
	// Un pago se registra en UTC: mostrarlo en hora oficial de Ecuador continental (UTC-5).
	instant, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return instant.In(time.FixedZone("EC", -5*3600)).Format("02/01/2006 15:04")
	}
	if len(value) >= 19 {
		return strings.ReplaceAll(value[:19], "T", " ")
	}
	return value
}
func receiptBytes(order Order, patient Patient, payment Payment, previous int64, isCopy bool) []byte {
	var b strings.Builder
	b.WriteString("\x1b@")              // Inicializar ESC/POS
	b.WriteString("\x1b3\x18")          // Interlineado cómodo
	b.WriteString("\x1ba\x01\x1bE\x01") // Centro, negrita
	b.WriteString("REV VISION\n")
	b.WriteString("\x1bE\x00")
	b.WriteString("OPTICA Y SALUD VISUAL\n")
	b.WriteString("COMPROBANTE DE ABONO\n")
	b.WriteString("NO ES FACTURA TRIBUTARIA\n")
	if isCopy {
		b.WriteString("REIMPRESION DEL MISMO ABONO\n")
	}
	b.WriteString("\x1ba\x00")
	b.WriteString(strings.Repeat("-", paperColumns) + "\n")
	b.WriteString(receiptLine("Nro", payment.ReceiptNumber))
	b.WriteString(receiptLine("Pedido", strings.ToUpper(order.ID[:min(8, len(order.ID))])))
	b.WriteString(receiptLine("Paciente", patient.FullName))
	if patient.DocumentID != "" {
		b.WriteString(receiptLine("C.I.", patient.DocumentID))
	}
	b.WriteString(receiptLine("Fecha", receiptDate(payment.PaidAt)))
	b.WriteString(strings.Repeat("-", paperColumns) + "\n")
	for _, line := range order.Lines {
		b.WriteString(receiptLine("Articulo", fmt.Sprintf("%s x%d", line.Name, line.Quantity)))
		b.WriteString(receiptAligned("Subtotal", receiptCents(line.SubtotalCents)))
	}
	b.WriteString(strings.Repeat("-", paperColumns) + "\n")
	b.WriteString(receiptAligned("Total pedido", receiptCents(order.TotalCents)))
	b.WriteString(receiptAligned("Abonado antes", receiptCents(previous)))
	b.WriteString("\x1bE\x01")
	b.WriteString(receiptAligned("ABONO RECIBIDO", receiptCents(payment.AmountCents)))
	b.WriteString("\x1bE\x00")
	b.WriteString(receiptAligned("SALDO", receiptCents(order.TotalCents-previous-payment.AmountCents)))
	b.WriteString(receiptLine("Pago", payment.Method))
	if strings.TrimSpace(payment.Note) != "" {
		b.WriteString(receiptLine("Referencia", payment.Note))
	}
	b.WriteString("\x1ba\x01")
	b.WriteString("\nGRACIAS POR SU VISITA\n")
	b.WriteString("\n\n")
	b.WriteString("\x1dVB\x00") // GS V B 0: corte automático si firmware lo admite
	return []byte(b.String())
}
func testReceiptBytes() []byte {
	return []byte("\x1b@\x1ba\x01\x1bE\x01REV VISION\n\x1bE\x00PRUEBA TERMICA DIRECTA\nPAPEL 80 MM\nNO REGISTRA PAGOS\n\nFIN DE PRUEBA\n\n\x1dVB\x00")
}
func allowedPrintOrigin(r *http.Request) bool {
	// No autorizar solicitudes entre sitios que intenten usar la impresora local.
	if r.Host != "127.0.0.1:"+port && r.Host != "localhost:"+port {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://127.0.0.1:"+port && origin != "http://localhost:"+port {
		return false
	}
	return true
}
func (s *Store) thermalHandler(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/printers" && r.URL.Path != "/api/print/thermal/test" && r.URL.Path != "/api/print/thermal/receipt" {
		return false
	}
	if !allowedPrintOrigin(r) {
		fail(w, 403, "Origen de impresión no autorizado")
		return true
	}
	if r.URL.Path == "/api/printers" {
		if r.Method != http.MethodGet {
			fail(w, 405, "Método no permitido")
			return true
		}
		printers, e := getInstalledPrinters()
		if e != nil {
			fail(w, 503, "No se pudieron consultar impresoras de Windows: "+e.Error())
			return true
		}
		if printers == nil {
			printers = []string{}
		}
		sort.Strings(printers)
		jsonReply(w, 200, map[string]interface{}{"printers": printers, "mode": "windows-raw-escpos", "hint": "Selecciona POS-80C o Crome CRM-308"})
		return true
	}
	if r.Method != http.MethodPost {
		fail(w, 405, "Método no permitido")
		return true
	}
	var req thermalPrintRequest
	if err := receive(r, &req); err != nil {
		fail(w, 400, err.Error())
		return true
	}
	req.Printer = strings.TrimSpace(req.Printer)
	if len(req.Printer) < 2 || len(req.Printer) > 200 {
		fail(w, 400, "Selecciona una impresora térmica instalada")
		return true
	}
	printers, e := getInstalledPrinters()
	if e != nil {
		fail(w, 503, "No se pudieron consultar las impresoras")
		return true
	}
	match := false
	for _, p := range printers {
		if strings.EqualFold(req.Printer, p) {
			req.Printer = p
			match = true
			break
		}
	}
	if !match {
		fail(w, 400, "Impresora no instalada o no disponible. Selecciónala en la lista.")
		return true
	}
	var payload []byte
	if r.URL.Path == "/api/print/thermal/test" {
		payload = testReceiptBytes()
	} else {
		if req.PaymentID == "" {
			fail(w, 400, "Selecciona un abono guardado")
			return true
		}
		s.mu.Lock()
		payment, ok := s.data.Payments[req.PaymentID]
		if !ok {
			s.mu.Unlock()
			fail(w, 404, "Abono no encontrado")
			return true
		}
		order, ok := s.data.Orders[payment.OrderID]
		if !ok {
			s.mu.Unlock()
			fail(w, 404, "Pedido no encontrado")
			return true
		}
		patient, ok := s.data.Patients[order.PatientID]
		if !ok {
			s.mu.Unlock()
			fail(w, 404, "Paciente no encontrado")
			return true
		}
		previous := int64(0)
		for _, other := range s.data.Payments {
			if other.OrderID == order.ID && (other.PaidAt < payment.PaidAt || (other.PaidAt == payment.PaidAt && other.ID < payment.ID)) {
				previous += other.AmountCents
			}
		}
		if previous+payment.AmountCents > order.TotalCents {
			s.mu.Unlock()
			fail(w, 409, "Montos inconsistentes. No se imprimió")
			return true
		}
		payload = receiptBytes(order, patient, payment, previous, req.Copy)
		s.mu.Unlock()
	}
	if len(payload) > 16384 {
		fail(w, 413, "Ticket demasiado largo. Revisa los artículos")
		return true
	}
	if e := sendRawToPrinter(req.Printer, payload); e != nil {
		fail(w, 503, "No se pudo enviar el trabajo a la impresora: "+e.Error())
		return true
	}
	jsonReply(w, 200, map[string]string{"status": "spooled", "message": "Trabajo enviado a la cola de la impresora. Comprueba la salida física."})
	return true
}

var errPrinterUnavailable = errors.New("la impresión directa requiere Windows")
