package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// The inventory acquisition view is a read-only projection of real saved orders.
func TestInventoryBuyersWithHistoricalPricesAndPayments(t *testing.T) {
	s := &Store{file: filepath.Join(t.TempDir(), "patients.json"), data: freshDatabase()}
	s.data.Products["frame-1"] = Product{ID: "frame-1", SKU: "M-01-EDITADO", Name: "MONTURA LILA", PriceCents: 8000, Active: false, TrackStock: true, Stock: 7}
	s.data.Products["lens-2"] = Product{ID: "lens-2", SKU: "L-02", Name: "LUNA", PriceCents: 1000, Active: true}
	s.data.Patients["p1"] = Patient{ID: "p1", FullName: "ANA PRUEBA", DocumentID: "0912345675"}
	s.data.Patients["p2"] = Patient{ID: "p2", FullName: "LUIS PRUEBA", DocumentID: "1710034065"}
	s.data.Orders["order-old"] = Order{ID: "order-old", PatientID: "p1", CreatedAt: "2026-10-15T10:00:00Z", TotalCents: 5000, Lines: []OrderLine{{ProductID: "frame-1", SKU: "M-01", Name: "MONTURA ROSA", Quantity: 2, UnitPriceCents: 2500, SubtotalCents: 5000}}}
	s.data.Orders["order-new"] = Order{ID: "order-new", PatientID: "p2", CreatedAt: "2026-10-17T10:00:00Z", TotalCents: 3800, Lines: []OrderLine{{ProductID: "frame-1", SKU: "M-01", Name: "MONTURA ROSA", Quantity: 1, UnitPriceCents: 2800, SubtotalCents: 2800}, {ProductID: "lens-2", SKU: "L-02", Name: "LUNA", Quantity: 1, UnitPriceCents: 1000, SubtotalCents: 1000}}}
	s.data.Payments["pay1"] = Payment{ID: "pay1", OrderID: "order-new", AmountCents: 1000}
	s.data.Payments["pay2"] = Payment{ID: "pay2", OrderID: "order-new", AmountCents: 500}
	before, _ := json.Marshal(s.data)
	req := httptest.NewRequest(http.MethodGet, "/api/products/frame-1/buyers", nil)
	w := httptest.NewRecorder()
	s.handler(w, req)
	if w.Code != 200 {
		t.Fatalf("GET buyers %d: %s", w.Code, w.Body.String())
	}
	var result []ProductPurchase
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected two purchases, got %+v", result)
	}
	if result[0].OrderID != "order-new" || result[0].PatientName != "LUIS PRUEBA" || result[0].Quantity != 1 || result[0].UnitPriceCents != 2800 || result[0].PaidCents != 1500 || result[0].BalanceCents != 2300 {
		t.Fatalf("recent purchase incorrect: %+v", result[0])
	}
	if result[1].OrderID != "order-old" || result[1].PatientName != "ANA PRUEBA" || result[1].Quantity != 2 || result[1].UnitPriceCents != 2500 || result[1].BalanceCents != 5000 {
		t.Fatalf("historical purchase incorrect: %+v", result[1])
	}
	after, _ := json.Marshal(s.data)
	if !bytes.Equal(before, after) {
		t.Fatal("read-only buyers view modified database")
	}
	w = httptest.NewRecorder()
	s.handler(w, httptest.NewRequest(http.MethodGet, "/api/products/lens-2/buyers", nil))
	if w.Code != 200 {
		t.Fatalf("other product GET: %d", w.Code)
	}
	result = nil
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 1 || result[0].Quantity != 1 || result[0].OrderID != "order-new" {
		t.Fatalf("other product mixed with frame sales: %+v", result)
	}
	w = httptest.NewRecorder()
	s.handler(w, httptest.NewRequest(http.MethodGet, "/api/products/unknown/buyers", nil))
	if w.Code != 404 {
		t.Fatalf("missing product status: %d", w.Code)
	}
	s.data.Products["unsold"] = Product{ID: "unsold", SKU: "UN", Name: "SIN VENTAS"}
	w = httptest.NewRecorder()
	s.handler(w, httptest.NewRequest(http.MethodGet, "/api/products/unsold/buyers", nil))
	if w.Code != 200 || w.Body.String() == "null\n" || w.Body.String() != "[]\n" {
		t.Fatalf("unsold product response: %d %q", w.Code, w.Body.String())
	}
}
