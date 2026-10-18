package main

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Cash amounts always use integer US cents, never floating point.
type Product struct {
	ID         string `json:"id"`
	SKU        string `json:"sku"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	PriceCents int64  `json:"price_cents"`
	Stock      int    `json:"stock"`
	TrackStock bool   `json:"track_stock"`
	Active     bool   `json:"active"`
	Version    int    `json:"version"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}
type OrderLine struct {
	ProductID      string `json:"product_id"`
	SKU            string `json:"sku"`
	Name           string `json:"name"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	SubtotalCents  int64  `json:"subtotal_cents"`
}
type Order struct {
	ID             string      `json:"id"`
	PatientID      string      `json:"patient_id"`
	ConsultationID string      `json:"consultation_id"`
	OperationID    string      `json:"operation_id"`
	Lines          []OrderLine `json:"lines"`
	TotalCents     int64       `json:"total_cents"`
	CreatedAt      string      `json:"created_at"`
}
type Payment struct {
	ID            string `json:"id"`
	OrderID       string `json:"order_id"`
	OperationID   string `json:"operation_id"`
	AmountCents   int64  `json:"amount_cents"`
	Method        string `json:"method"`
	Note          string `json:"note"`
	ReceiptNumber string `json:"receipt_number"`
	PaidAt        string `json:"paid_at"`
}
type requestedItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}
type orderRequest struct {
	PatientID      string          `json:"patient_id"`
	ConsultationID string          `json:"consultation_id"`
	OperationID    string          `json:"operation_id"`
	Items          []requestedItem `json:"items"`
}
type paymentRequest struct {
	OrderID     string `json:"order_id"`
	OperationID string `json:"operation_id"`
	AmountCents int64  `json:"amount_cents"`
	Method      string `json:"method"`
	Note        string `json:"note"`
}

func validOperationID(id string) bool {
	return len(id) >= 16 && len(id) <= 80 && !strings.ContainsAny(id, " \r\n\t")
}
func checkProduct(p *Product) error {
	p.SKU = strings.ToUpper(clean(p.SKU, 36))
	p.Name = clean(p.Name, 150)
	p.Category = clean(p.Category, 60)
	if p.SKU == "" || p.Name == "" {
		return errors.New("El código y nombre del producto son obligatorios")
	}
	if p.PriceCents < 0 || p.PriceCents > 100000000 {
		return errors.New("Precio de producto fuera de rango")
	}
	if p.Stock < 0 || p.Stock > 1000000 {
		return errors.New("Existencia fuera de rango")
	}
	if p.Category == "" {
		p.Category = "OTROS"
	}
	return nil
}

// ProductPurchase uses already-persisted orders as the source of truth. No
// second customer list is stored on a product and no stock/payment is changed.
type ProductPurchase struct {
	OrderID         string `json:"order_id"`
	PatientID       string `json:"patient_id"`
	PatientName     string `json:"patient_name"`
	PatientDocument string `json:"patient_document"`
	PurchasedAt     string `json:"purchased_at"`
	Quantity        int    `json:"quantity"`
	UnitPriceCents  int64  `json:"unit_price_cents"`
	SubtotalCents   int64  `json:"subtotal_cents"`
	OrderTotalCents int64  `json:"order_total_cents"`
	PaidCents       int64  `json:"paid_cents"`
	BalanceCents    int64  `json:"balance_cents"`
}

func (s *Store) commercial(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	if path == "/api/products" {
		if r.Method == http.MethodGet {
			s.mu.Lock()
			defer s.mu.Unlock()
			out := make([]Product, 0, len(s.data.Products))
			for _, p := range s.data.Products {
				out = append(out, p)
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
			jsonReply(w, 200, out)
			return true
		}
		if r.Method == http.MethodPost {
			var p Product
			if err := receive(r, &p); err != nil {
				fail(w, 400, err.Error())
				return true
			}
			if err := checkProduct(&p); err != nil {
				fail(w, 400, err.Error())
				return true
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			for _, v := range s.data.Products {
				if v.SKU == p.SKU {
					fail(w, 409, "Ya existe un artículo con ese código")
					return true
				}
			}
			next, err := clone(s.data)
			if err != nil {
				fail(w, 500, "Error interno")
				return true
			}
			p.ID = newID()
			p.CreatedAt = utcNow()
			p.UpdatedAt = p.CreatedAt
			p.Version = 1
			p.Active = true
			next.Products[p.ID] = p
			audit(&next, "create", "product", p.ID, nil)
			if err = s.save(next); err != nil {
				fail(w, 500, "No se pudo guardar producto")
				return true
			}
			jsonReply(w, 201, p)
			return true
		}
	}
	if strings.HasPrefix(path, "/api/products/") && strings.HasSuffix(path, "/buyers") && r.Method == http.MethodGet {
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/products/"), "/buyers")
		if id == "" || strings.Contains(id, "/") {
			fail(w, 404, "Artículo no encontrado")
			return true
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.data.Products[id]; !ok {
			fail(w, 404, "Artículo no encontrado")
			return true
		}
		paidByOrder := make(map[string]int64, len(s.data.Orders))
		for _, payment := range s.data.Payments {
			paidByOrder[payment.OrderID] += payment.AmountCents
		}
		purchases := make([]ProductPurchase, 0)
		for _, o := range s.data.Orders {
			patient, hasPatient := s.data.Patients[o.PatientID]
			patientName := patient.FullName
			if !hasPatient || patientName == "" {
				patientName = "Ficha no disponible"
			}
			for _, line := range o.Lines {
				if line.ProductID != id {
					continue
				}
				paid := paidByOrder[o.ID]
				purchases = append(purchases, ProductPurchase{
					OrderID: o.ID, PatientID: o.PatientID,
					PatientName: patientName, PatientDocument: patient.DocumentID,
					PurchasedAt: o.CreatedAt, Quantity: line.Quantity,
					UnitPriceCents: line.UnitPriceCents, SubtotalCents: line.SubtotalCents,
					OrderTotalCents: o.TotalCents, PaidCents: paid,
					BalanceCents: o.TotalCents - paid,
				})
			}
		}
		sort.Slice(purchases, func(i, j int) bool {
			if purchases[i].PurchasedAt == purchases[j].PurchasedAt {
				return purchases[i].OrderID > purchases[j].OrderID
			}
			return purchases[i].PurchasedAt > purchases[j].PurchasedAt
		})
		jsonReply(w, 200, purchases)
		return true
	}
	if strings.HasPrefix(path, "/api/products/") && r.Method == http.MethodPut {
		id := strings.TrimPrefix(path, "/api/products/")
		var p Product
		if err := receive(r, &p); err != nil {
			fail(w, 400, err.Error())
			return true
		}
		if err := checkProduct(&p); err != nil {
			fail(w, 400, err.Error())
			return true
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		old, ok := s.data.Products[id]
		if !ok {
			fail(w, 404, "Producto no encontrado")
			return true
		}
		if old.Version != p.Version {
			fail(w, 409, "El producto cambió. Actualiza la lista y vuelve a intentar")
			return true
		}
		for _, v := range s.data.Products {
			if v.ID != id && v.SKU == p.SKU {
				fail(w, 409, "Otro artículo utiliza el mismo código")
				return true
			}
		}
		next, err := clone(s.data)
		if err != nil {
			fail(w, 500, "Error interno")
			return true
		}
		p.ID = id
		p.CreatedAt = old.CreatedAt
		p.UpdatedAt = utcNow()
		p.Version = old.Version + 1
		next.Products[id] = p
		audit(&next, "update", "product", id, old)
		if err = s.save(next); err != nil {
			fail(w, 500, "No se pudo guardar producto")
			return true
		}
		jsonReply(w, 200, p)
		return true
	}
	if path == "/api/orders" {
		if r.Method == http.MethodGet {
			patientID := strings.TrimSpace(r.URL.Query().Get("patient_id"))
			s.mu.Lock()
			defer s.mu.Unlock()
			rows := make([]map[string]interface{}, 0)
			for _, o := range s.data.Orders {
				if patientID != "" && o.PatientID != patientID {
					continue
				}
				paid := int64(0)
				pmts := []Payment{}
				for _, p := range s.data.Payments {
					if p.OrderID == o.ID {
						paid += p.AmountCents
						pmts = append(pmts, p)
					}
				}
				sort.Slice(pmts, func(i, j int) bool {
					if pmts[i].PaidAt == pmts[j].PaidAt {
						return pmts[i].ReceiptNumber < pmts[j].ReceiptNumber
					}
					return pmts[i].PaidAt < pmts[j].PaidAt
				})
				patient := s.data.Patients[o.PatientID]
				rows = append(rows, map[string]interface{}{"order": o, "payments": pmts, "paid_cents": paid, "balance_cents": o.TotalCents - paid, "patient_name": patient.FullName, "patient_document": patient.DocumentID})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i]["order"].(Order).CreatedAt > rows[j]["order"].(Order).CreatedAt })
			jsonReply(w, 200, rows)
			return true
		}
		if r.Method == http.MethodPost {
			var inp orderRequest
			if err := receive(r, &inp); err != nil {
				fail(w, 400, err.Error())
				return true
			}
			if !validOperationID(inp.OperationID) {
				fail(w, 400, "Identificador de operación inválido")
				return true
			}
			if len(inp.Items) == 0 || len(inp.Items) > 30 {
				fail(w, 400, "Agrega de 1 a 30 artículos")
				return true
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			for _, v := range s.data.Orders {
				if v.OperationID == inp.OperationID {
					jsonReply(w, 200, v)
					return true
				}
			}
			c, ok := s.data.Consultations[inp.ConsultationID]
			if !ok || c.PatientID != inp.PatientID {
				fail(w, 400, "Selecciona una consulta del mismo paciente")
				return true
			}
			for _, v := range s.data.Orders {
				if v.ConsultationID == inp.ConsultationID {
					fail(w, 409, "Esta atención ya tiene un pedido. Abre su registro para cobrar")
					return true
				}
			}
			next, err := clone(s.data)
			if err != nil {
				fail(w, 500, "Error interno")
				return true
			}
			seen := map[string]bool{}
			lines := []OrderLine{}
			var total int64
			for _, it := range inp.Items {
				if it.Quantity < 1 || it.Quantity > 1000 || seen[it.ProductID] {
					fail(w, 400, "Cantidad inválida o artículo repetido")
					return true
				}
				seen[it.ProductID] = true
				p, found := next.Products[it.ProductID]
				if !found || !p.Active {
					fail(w, 400, "Producto inactivo o inexistente")
					return true
				}
				if p.TrackStock && p.Stock < it.Quantity {
					fail(w, 409, "Stock insuficiente de "+p.Name)
					return true
				}
				subtotal := p.PriceCents * int64(it.Quantity)
				if subtotal > 100000000 || total+subtotal > 100000000 {
					fail(w, 400, "El total excede el límite de esta prueba")
					return true
				}
				if p.TrackStock {
					p.Stock -= it.Quantity
					p.Version++
					p.UpdatedAt = utcNow()
					next.Products[it.ProductID] = p
				}
				lines = append(lines, OrderLine{ProductID: p.ID, SKU: p.SKU, Name: p.Name, Quantity: it.Quantity, UnitPriceCents: p.PriceCents, SubtotalCents: subtotal})
				total += subtotal
			}
			o := Order{ID: newID(), PatientID: inp.PatientID, ConsultationID: inp.ConsultationID, OperationID: inp.OperationID, Lines: lines, TotalCents: total, CreatedAt: utcNow()}
			next.Orders[o.ID] = o
			audit(&next, "create", "order", o.ID, nil)
			if err = s.save(next); err != nil {
				fail(w, 500, "No se pudo guardar pedido ni actualizar inventario")
				return true
			}
			jsonReply(w, 201, o)
			return true
		}
	}
	if path == "/api/payments" && r.Method == http.MethodPost {
		var inp paymentRequest
		if err := receive(r, &inp); err != nil {
			fail(w, 400, err.Error())
			return true
		}
		if !validOperationID(inp.OperationID) {
			fail(w, 400, "Identificador de operación inválido")
			return true
		}
		if inp.AmountCents <= 0 || inp.AmountCents > 100000000 {
			fail(w, 400, "Importe de abono inválido")
			return true
		}
		if inp.Method != "EFECTIVO" && inp.Method != "TRANSFERENCIA" && inp.Method != "TARJETA" && inp.Method != "OTRO" {
			fail(w, 400, "Forma de pago inválida")
			return true
		}
		inp.Note = clean(inp.Note, 280)
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, v := range s.data.Payments {
			if v.OperationID == inp.OperationID {
				jsonReply(w, 200, v)
				return true
			}
		}
		o, ok := s.data.Orders[inp.OrderID]
		if !ok {
			fail(w, 404, "Pedido no encontrado")
			return true
		}
		paid := int64(0)
		for _, v := range s.data.Payments {
			if v.OrderID == o.ID {
				paid += v.AmountCents
			}
		}
		if inp.AmountCents > o.TotalCents-paid {
			fail(w, 409, "El abono supera el saldo pendiente")
			return true
		}
		next, err := clone(s.data)
		if err != nil {
			fail(w, 500, "Error interno")
			return true
		}
		dt := time.Now()
		receipt := fmt.Sprintf("RV-A-%s-%06d", dt.Format("20060102"), len(next.Payments)+1)
		p := Payment{ID: newID(), OrderID: o.ID, OperationID: inp.OperationID, AmountCents: inp.AmountCents, Method: inp.Method, Note: inp.Note, ReceiptNumber: receipt, PaidAt: dt.Format(time.RFC3339)}
		next.Payments[p.ID] = p
		audit(&next, "create", "payment", p.ID, nil)
		if err = s.save(next); err != nil {
			fail(w, 500, "El abono NO se guardó. Comprueba y reintenta con la misma operación")
			return true
		}
		jsonReply(w, 201, p)
		return true
	}
	return false
}
