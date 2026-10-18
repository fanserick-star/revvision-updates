package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func postTest(t *testing.T, s *Store, url string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-RevVision-Client", "revvision-local")
	w := httptest.NewRecorder()
	s.handler(w, req)
	return w
}
func TestPatientConsultationAndRestore(t *testing.T) {
	dir := t.TempDir()
	store := &Store{file: filepath.Join(dir, "patients.json"), data: freshDatabase()}
	p := Patient{FullName: "PACIENTE DE PRUEBA", DocumentID: "0912345675", Notes: "Observaciones de prueba"}
	w := postTest(t, store, "/api/patients", p)
	if w.Code != 201 {
		t.Fatalf("alta = %d: %s", w.Code, w.Body.String())
	}
	var saved Patient
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" || saved.Version != 1 {
		t.Fatalf("ficha inválida: %+v", saved)
	}
	dup := postTest(t, store, "/api/patients", p)
	if dup.Code != 409 {
		t.Fatalf("duplicado permitió %d", dup.Code)
	}
	c := Consultation{PatientID: saved.ID, Professional: "PROFESIONAL PRUEBA", Motive: "CONSULTA DE PRUEBA", SphereOD: "+0.50", SphereOI: "-0.25"}
	wc := postTest(t, store, "/api/consultations", c)
	if wc.Code != 201 {
		t.Fatalf("consulta=%d: %s", wc.Code, wc.Body.String())
	}
	file, err := os.ReadFile(store.file)
	if err != nil {
		t.Fatal(err)
	}
	var restored Database
	if err := json.Unmarshal(file, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Schema != 2 || len(restored.Patients) != 1 || len(restored.Consultations) != 1 {
		t.Fatalf("reapertura inválida: %+v", restored)
	}
	for _, visit := range restored.Consultations {
		if visit.PatientID != saved.ID || visit.SphereOD != "+0.50" {
			t.Errorf("historial dañado: %+v", visit)
		}
	}
}
func TestClientHeaderRequiredForWrite(t *testing.T) {
	s := &Store{file: filepath.Join(t.TempDir(), "patients.json"), data: freshDatabase()}
	req := httptest.NewRequest(http.MethodPost, "/api/patients", bytes.NewBufferString(`{"full_name":"PRUEBA TEST"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handler(w, req)
	if w.Code != 403 {
		t.Fatalf("se esperaba 403: %d", w.Code)
	}
}

func TestPatientValidationRules(t *testing.T) {
	p := Patient{FullName: "María ñúñez", DocumentID: "0912345675", Phone: "0991234567", Email: "PRUEBA@EJEMPLO.COM"}
	if e := validatePatient(&p); e != nil {
		t.Fatal(e)
	}
	if p.FullName != "MARÍA ÑÚÑEZ" || p.Phone != "099 123 4567" || p.Email != "prueba@ejemplo.com" {
		t.Fatalf("normalización: %+v", p)
	}
	for _, id := range []string{"0912345676", "12345", "2512345671", "091234567A"} {
		a := Patient{FullName: "PRUEBA", DocumentID: id}
		if validatePatient(&a) == nil {
			t.Errorf("cédula inválida admitida: %s", id)
		}
	}
	a := Patient{FullName: "PRUEBA", Phone: "0991234"}
	if validatePatient(&a) == nil {
		t.Fatal("teléfono incorrecto admitido")
	}
	a = Patient{FullName: "PRUEBA", Email: "abc@.com"}
	if validatePatient(&a) == nil {
		t.Fatal("correo incorrecto admitido")
	}
	a = Patient{FullName: "Juan Prueba", DocumentType: "pasaporte", DocumentID: "XJ-1234567"}
	if e := validatePatient(&a); e != nil {
		t.Fatal(e)
	}
}
func TestInventorySaleAndPaymentsIdempotent(t *testing.T) {
	s := &Store{file: filepath.Join(t.TempDir(), "data.json"), data: freshDatabase()}
	w := postTest(t, s, "/api/patients", Patient{FullName: "cliente prueba", DocumentID: "0912345675"})
	if w.Code != 201 {
		t.Fatalf("alta %d %s", w.Code, w.Body)
	}
	var patient Patient
	_ = json.Unmarshal(w.Body.Bytes(), &patient)
	w = postTest(t, s, "/api/consultations", Consultation{PatientID: patient.ID, Professional: "OPTOMETRISTA PRUEBA", Motive: "EXAMEN"})
	if w.Code != 201 {
		t.Fatalf("consulta %d %s", w.Code, w.Body)
	}
	var c Consultation
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	prod := Product{SKU: "M-001", Name: "MONTURA PRUEBA", Category: "MONTURAS", PriceCents: 12550, Stock: 3, TrackStock: true}
	w = postTest(t, s, "/api/products", prod)
	if w.Code != 201 {
		t.Fatalf("producto %d %s", w.Code, w.Body)
	}
	var saved Product
	_ = json.Unmarshal(w.Body.Bytes(), &saved)
	if postTest(t, s, "/api/products", prod).Code != 409 {
		t.Fatal("duplicado de SKU permitido")
	}
	op := "op-venta-prueba-00001"
	orderBody := orderRequest{PatientID: patient.ID, ConsultationID: c.ID, OperationID: op, Items: []requestedItem{{ProductID: saved.ID, Quantity: 2}}}
	w = postTest(t, s, "/api/orders", orderBody)
	if w.Code != 201 {
		t.Fatalf("venta %d %s", w.Code, w.Body)
	}
	var o Order
	_ = json.Unmarshal(w.Body.Bytes(), &o)
	if o.TotalCents != 25100 || s.data.Products[saved.ID].Stock != 1 {
		t.Fatalf("stock o total incorrecto: %+v %+v", o, s.data.Products[saved.ID])
	}
	if postTest(t, s, "/api/orders", orderBody).Code != 200 || len(s.data.Orders) != 1 || s.data.Products[saved.ID].Stock != 1 {
		t.Fatal("reintento duplicó pedido o descontó stock")
	}
	if postTest(t, s, "/api/orders", orderRequest{PatientID: patient.ID, ConsultationID: c.ID, OperationID: "op-venta-distinto-01", Items: orderBody.Items}).Code != 409 {
		t.Fatal("segunda venta para consulta admitida")
	}
	ab := paymentRequest{OrderID: o.ID, OperationID: "op-abono-prueba-0001", AmountCents: 10000, Method: "EFECTIVO"}
	w = postTest(t, s, "/api/payments", ab)
	if w.Code != 201 {
		t.Fatalf("abono %d %s", w.Code, w.Body)
	}
	var payment Payment
	_ = json.Unmarshal(w.Body.Bytes(), &payment)
	if payment.ReceiptNumber == "" || payment.PaidAt == "" {
		t.Fatalf("comprobante incompleto: %+v", payment)
	}
	if postTest(t, s, "/api/payments", ab).Code != 200 || len(s.data.Payments) != 1 {
		t.Fatal("reintento duplicó abono")
	}
	if postTest(t, s, "/api/payments", paymentRequest{OrderID: o.ID, OperationID: "op-abono-prueba-0002", AmountCents: 25100, Method: "EFECTIVO"}).Code != 409 {
		t.Fatal("sobrecobro permitido")
	}
	w = postTest(t, s, "/api/payments", paymentRequest{OrderID: o.ID, OperationID: "op-abono-prueba-0003", AmountCents: 15100, Method: "TRANSFERENCIA"})
	if w.Code != 201 {
		t.Fatalf("abono final %d %s", w.Code, w.Body)
	}
	if len(s.data.Payments) != 2 {
		t.Fatal("faltan abonos")
	}
	// Check that all state was persisted, not just updated in memory.
	data, err := os.ReadFile(s.file)
	if err != nil {
		t.Fatal(err)
	}
	var d Database
	if err = json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Payments) != 2 || len(d.Orders) != 1 || d.Products[saved.ID].Stock != 1 || d.Schema != 2 {
		t.Fatalf("pérdida al guardar: %+v", d)
	}
}
func TestMigrateLocalSchemaOneWithoutLosingPatients(t *testing.T) {
	path := filepath.Join(t.TempDir(), "patients.json")
	s := &Store{file: path, data: freshDatabase()}
	s.data.Schema = 1
	s.data.Products = nil
	s.data.Orders = nil
	s.data.Payments = nil
	s.data.Patients["preserved"] = Patient{ID: "preserved", FullName: "NO PERDER", DocumentID: "0912345675"}
	b, _ := json.Marshal(s.data)
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	// The test store is in a custom folder; emulate the migration's actual save path.
	existing := &Store{file: path, data: freshDatabase()}
	if e := json.Unmarshal(b, &existing.data); e != nil {
		t.Fatal(e)
	}
	next, e := clone(existing.data)
	if e != nil {
		t.Fatal(e)
	}
	next.Schema = 2
	next.Products = map[string]Product{}
	next.Orders = map[string]Order{}
	next.Payments = map[string]Payment{}
	if e = existing.save(next); e != nil {
		t.Fatal(e)
	}
	final, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var migrated Database
	if e = json.Unmarshal(final, &migrated); e != nil {
		t.Fatal(e)
	}
	if migrated.Schema != 2 || len(migrated.Patients) != 1 || migrated.Patients["preserved"].FullName != "NO PERDER" {
		t.Fatal("la migración perdió la ficha")
	}
	backup, e := os.ReadFile(path + ".last-good")
	if e != nil {
		t.Fatalf("sin respaldo del esquema antiguo: %v", e)
	}
	var previous Database
	if e = json.Unmarshal(backup, &previous); e != nil {
		t.Fatal(e)
	}
	if previous.Schema != 1 || len(previous.Patients) != 1 {
		t.Fatal("respaldo original inválido")
	}
}

func TestConsultationIdempotency(t *testing.T) {
	s := &Store{file: filepath.Join(t.TempDir(), "db.json"), data: freshDatabase()}
	w := postTest(t, s, "/api/patients", Patient{FullName: "PRUEBA ID", DocumentID: "1710034065"})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var p Patient
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	c := Consultation{PatientID: p.ID, Professional: "PRUEBA", Motive: "TEST", OperationID: "unique-operation-2026-100"}
	first := postTest(t, s, "/api/consultations", c)
	second := postTest(t, s, "/api/consultations", c)
	if first.Code != 201 || second.Code != 200 || len(s.data.Consultations) != 1 {
		t.Fatalf("consulta duplicada %d %d registros:%d", first.Code, second.Code, len(s.data.Consultations))
	}
}

// No se aceptan letras en agudeza visual ni aunque alguien evite las restricciones del navegador.
func TestAcuityValidationAllEightFields(t *testing.T) {
	good := Consultation{Professional: "OPTOMETRISTA PRUEBA", Motive: "CONTROL",
		AVUnODFar: "20/20", AVUnOIFar: "20/25", AVUnODNear: ".50", AVUnOINear: "0,50",
		AVCODFar: "20/15", AVCOIFar: "+20/20", AVCODNear: "-0.5", AVCOINear: "100%"}
	if err := validateConsultation(&good); err != nil {
		t.Fatalf("agudezas válidas rechazadas: %v", err)
	}
	badValues := []string{"20/20M", "N/A", "LETRAS", "0.5 M", "20_20", "2O/2O", "1234567890123456789"}
	for _, invalid := range badValues {
		c := good
		c.AVUnODFar = invalid
		if err := validateConsultation(&c); err == nil {
			t.Errorf("valor inválido aceptado: %q", invalid)
		}
	}
	for _, setter := range []func(*Consultation){
		func(c *Consultation) { c.AVUnODFar = "ABC" },
		func(c *Consultation) { c.AVUnOIFar = "ABC" },
		func(c *Consultation) { c.AVUnODNear = "ABC" },
		func(c *Consultation) { c.AVUnOINear = "ABC" },
		func(c *Consultation) { c.AVCODFar = "ABC" },
		func(c *Consultation) { c.AVCOIFar = "ABC" },
		func(c *Consultation) { c.AVCODNear = "ABC" },
		func(c *Consultation) { c.AVCOINear = "ABC" },
	} {
		c := good
		setter(&c)
		if err := validateConsultation(&c); err == nil {
			t.Fatal("uno de los ocho campos admitió letras")
		}
	}
}

func TestOrdersGlobalListWithBalances(t *testing.T) {
	s := &Store{file: filepath.Join(t.TempDir(), "orders.json"), data: freshDatabase()}
	p1 := postTest(t, s, "/api/patients", Patient{FullName: "PACIENTE UNO", DocumentID: "0912345675"})
	p2 := postTest(t, s, "/api/patients", Patient{FullName: "PACIENTE DOS", DocumentID: "1710034065"})
	if p1.Code != 201 || p2.Code != 201 {
		t.Fatalf("pacientes: %s / %s", p1.Body, p2.Body)
	}
	var a, b Patient
	_ = json.Unmarshal(p1.Body.Bytes(), &a)
	_ = json.Unmarshal(p2.Body.Bytes(), &b)
	pa := postTest(t, s, "/api/consultations", Consultation{PatientID: a.ID, Professional: "PROFESIONAL", Motive: "CONTROL"})
	pb := postTest(t, s, "/api/consultations", Consultation{PatientID: b.ID, Professional: "PROFESIONAL", Motive: "CONTROL"})
	if pa.Code != 201 || pb.Code != 201 {
		t.Fatal("consultas no creadas")
	}
	var ca, cb Consultation
	_ = json.Unmarshal(pa.Body.Bytes(), &ca)
	_ = json.Unmarshal(pb.Body.Bytes(), &cb)
	prodResponse := postTest(t, s, "/api/products", Product{SKU: "L-001", Name: "LUNA DE PRUEBA", PriceCents: 1500, Stock: 10, TrackStock: true})
	if prodResponse.Code != 201 {
		t.Fatal(prodResponse.Body.String())
	}
	var prod Product
	_ = json.Unmarshal(prodResponse.Body.Bytes(), &prod)
	for i, pc := range []struct {
		p Patient
		c Consultation
	}{{a, ca}, {b, cb}} {
		order := postTest(t, s, "/api/orders", orderRequest{PatientID: pc.p.ID, ConsultationID: pc.c.ID, OperationID: "test-order-global-0" + string(rune('0'+i)), Items: []requestedItem{{ProductID: prod.ID, Quantity: 1}}})
		if order.Code != 201 {
			t.Fatalf("pedido: %s", order.Body.String())
		}
	}
	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		s.handler(w, r)
		return w
	}
	global := request("/api/orders")
	if global.Code != 200 {
		t.Fatalf("listado general: %d %s", global.Code, global.Body.String())
	}
	var rows []struct {
		PatientName     string `json:"patient_name"`
		PatientDocument string `json:"patient_document"`
		BalanceCents    int64  `json:"balance_cents"`
	}
	if err := json.Unmarshal(global.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].BalanceCents != 1500 || rows[1].BalanceCents != 1500 {
		t.Fatalf("pedidos generales: %+v", rows)
	}
	for _, row := range rows {
		if row.PatientName == "" || row.PatientDocument == "" {
			t.Errorf("pedido sin paciente asociado: %+v", row)
		}
	}
	filtered := request("/api/orders?patient_id=" + a.ID)
	var filteredRows []any
	if err := json.Unmarshal(filtered.Body.Bytes(), &filteredRows); err != nil {
		t.Fatal(err)
	}
	if filtered.Code != 200 || len(filteredRows) != 1 {
		t.Fatalf("filtro alterado: %d %s", filtered.Code, filtered.Body.String())
	}
}
