package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"revvision/windowcontrol"
)

//go:embed ui/*
var ui embed.FS

var appVersion = "dev"

const port = "58437"

type Patient struct {
	ID           string `json:"id"`
	FullName     string `json:"full_name"`
	DocumentType string `json:"document_type"`
	DocumentID   string `json:"document_id"`
	BirthDate    string `json:"birth_date"`
	AgeDeclared  string `json:"age_declared"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	Address      string `json:"address"`
	Occupation   string `json:"occupation"`
	Notes        string `json:"notes"`
	Version      int    `json:"version"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type Consultation struct {
	OperationID  string `json:"operation_id"`
	ID           string `json:"id"`
	PatientID    string `json:"patient_id"`
	Date         string `json:"date"`
	Motive       string `json:"motive"`
	AVUnODFar    string `json:"av_uncorrected_od_far"`
	AVUnOIFar    string `json:"av_uncorrected_oi_far"`
	AVUnODNear   string `json:"av_uncorrected_od_near"`
	AVUnOINear   string `json:"av_uncorrected_oi_near"`
	AVCODFar     string `json:"av_corrected_od_far"`
	AVCOIFar     string `json:"av_corrected_oi_far"`
	AVCODNear    string `json:"av_corrected_od_near"`
	AVCOINear    string `json:"av_corrected_oi_near"`
	SphereOD     string `json:"sphere_od"`
	CylinderOD   string `json:"cylinder_od"`
	AxisOD       string `json:"axis_od"`
	SphereOI     string `json:"sphere_oi"`
	CylinderOI   string `json:"cylinder_oi"`
	AxisOI       string `json:"axis_oi"`
	Addition     string `json:"addition"`
	DPNOD        string `json:"dpn_od"`
	DPNOI        string `json:"dpn_oi"`
	LensType     string `json:"lens_type"`
	DiagnosisOD  string `json:"diagnosis_od"`
	DiagnosisOI  string `json:"diagnosis_oi"`
	CIE10OD      string `json:"cie10_od"`
	CIE10OI      string `json:"cie10_oi"`
	ColorVision  string `json:"color_vision"`
	Observations string `json:"observations"`
	NextControl  string `json:"next_control"`
	Professional string `json:"professional"`
	Version      int    `json:"version"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type Audit struct {
	ID       string          `json:"id"`
	At       string          `json:"at"`
	Action   string          `json:"action"`
	Kind     string          `json:"kind"`
	RecordID string          `json:"record_id"`
	Snapshot json.RawMessage `json:"snapshot,omitempty"`
}

type Database struct {
	Schema        int                     `json:"schema"`
	Patients      map[string]Patient      `json:"patients"`
	Consultations map[string]Consultation `json:"consultations"`
	Products      map[string]Product      `json:"products"`
	Orders        map[string]Order        `json:"orders"`
	Payments      map[string]Payment      `json:"payments"`
	Audit         []Audit                 `json:"audit"`
}

type Store struct {
	mu   sync.Mutex
	file string
	data Database
}

func freshDatabase() Database {
	return Database{Schema: 2, Patients: map[string]Patient{}, Consultations: map[string]Consultation{}, Products: map[string]Product{}, Orders: map[string]Order{}, Payments: map[string]Payment{}, Audit: []Audit{}}
}
func storeDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "RevVision", "Pruebas", "data")
	return dir, os.MkdirAll(dir, 0700)
}
func loadStore() (*Store, error) {
	dir, err := storeDir()
	if err != nil {
		return nil, err
	}
	s := &Store{file: filepath.Join(dir, "revvision-local.json"), data: freshDatabase()}
	b, err := os.ReadFile(s.file)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("archivo de datos inválido: %w. No se modificó", err)
	}
	if s.data.Schema != 1 && s.data.Schema != 2 {
		return nil, fmt.Errorf("esquema desconocido %d; se evita abrir para proteger datos", s.data.Schema)
	}
	if s.data.Patients == nil || s.data.Consultations == nil {
		return nil, errors.New("base local incompleta; no se modifica")
	}
	if s.data.Schema == 1 {
		// One single forward migration. The old file is preserved as .last-good on disk.
		next, e := clone(s.data)
		if e != nil {
			return nil, e
		}
		next.Schema = 2
		next.Products = map[string]Product{}
		next.Orders = map[string]Order{}
		next.Payments = map[string]Payment{}
		if e = s.save(next); e != nil {
			return nil, fmt.Errorf("migración segura fallida: %w", e)
		}
	}
	if s.data.Products == nil || s.data.Orders == nil || s.data.Payments == nil {
		return nil, errors.New("base de inventario incompleta; no se modifica")
	}
	return s, nil
}
func newID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	v := hex.EncodeToString(b)
	return v[:8] + "-" + v[8:12] + "-" + v[12:16] + "-" + v[16:20] + "-" + v[20:]
}
func utcNow() string { return time.Now().UTC().Format(time.RFC3339) }
func (s *Store) save(next Database) error {
	// Put full audit trail in data, stage+sync+atomic replace. Previous committed snapshot retained.
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.file)
	f, err := os.CreateTemp(dir, ".revvision-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if _, err = os.Stat(s.file); err == nil {
		// Windows MoveFileEx may not replace an existing file; keep last-good, then perform replace with recovery.
		backup := s.file + ".last-good"
		old, err := os.ReadFile(s.file)
		if err != nil {return err}
		// Additional immutable daily recovery point. Failure is logged but
		// never interrupts the primary atomic save operation.
		if e:=rvDailyCheckpoint(s.file,old);e!=nil{log.Printf("Respaldo local diario no completado: %v",e)}
		bf, err := os.CreateTemp(dir, ".last-good-*.tmp")
		if err != nil {
			return err
		}
		bn := bf.Name()
		defer os.Remove(bn)
		if _, err = bf.Write(old); err != nil {
			bf.Close()
			return err
		}
		if err = bf.Sync(); err != nil {
			bf.Close()
			return err
		}
		if err = bf.Close(); err != nil {
			return err
		}
		// Renaming backup replaces existing on supported platforms; a previous backup stays usable if rename fails.
		if err = os.Rename(bn, backup); err != nil {
			return err
		}
	}
	if err = os.Rename(tmp, s.file); err != nil {
		// No borrar la versión anterior: si el reemplazo falla, se conserva intacta.
		return fmt.Errorf("no se pudo realizar reemplazo seguro: %w", err)
	}

	s.data = next
	return nil
}
func clone(d Database) (Database, error) {
	b, err := json.Marshal(d)
	if err != nil {
		return d, err
	}
	var cp Database
	err = json.Unmarshal(b, &cp)
	return cp, err
}
func audit(d *Database, action, kind, id string, snapshot interface{}) {
	b, _ := json.Marshal(snapshot)
	d.Audit = append(d.Audit, Audit{ID: newID(), At: utcNow(), Action: action, Kind: kind, RecordID: id, Snapshot: b})
}
func clean(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > max {
		r := []rune(s)
		s = string(r[:max])
	}
	return s
}

var emailBasic = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func validEcuadorCedula(v string) bool {
	if len(v) != 10 {
		return false
	}
	d := make([]int, 10)
	for i, c := range v {
		if c < '0' || c > '9' {
			return false
		}
		d[i] = int(c - '0')
	}
	province := d[0]*10 + d[1]
	if province < 1 || province > 24 || d[2] >= 6 {
		return false
	}
	sum := 0
	for i := 0; i < 9; i++ {
		m := 1
		if i%2 == 0 {
			m = 2
		}
		x := d[i] * m
		if x >= 10 {
			x -= 9
		}
		sum += x
	}
	return (10-sum%10)%10 == d[9]
}
func validatePatient(p *Patient) error {
	p.FullName = strings.ToUpper(clean(p.FullName, 180))
	p.DocumentType = strings.ToLower(clean(p.DocumentType, 16))
	if p.DocumentType == "" {
		p.DocumentType = "cedula"
	}
	p.DocumentID = clean(p.DocumentID, 40)
	p.Phone = clean(p.Phone, 40)
	p.Email = strings.ToLower(clean(p.Email, 120))
	p.Address = clean(p.Address, 300)
	p.Occupation = clean(p.Occupation, 180)
	p.Notes = clean(p.Notes, 1500)
	if len([]rune(p.FullName)) < 2 {
		return errors.New("Ingresa los nombres y apellidos del paciente")
	}
	for _, c := range p.FullName {
		if unicode.IsDigit(c) {
			return errors.New("El nombre no debe contener números")
		}
	}
	switch p.DocumentType {
	case "cedula":
		if p.DocumentID != "" && !validEcuadorCedula(p.DocumentID) {
			return errors.New("Cédula ecuatoriana no válida: debe tener 10 dígitos y un dígito verificador correcto")
		}
	case "pasaporte":
		if p.DocumentID != "" && (len(p.DocumentID) < 4 || len(p.DocumentID) > 25) {
			return errors.New("Pasaporte o identificación extranjera inválida")
		}
	default:
		return errors.New("Tipo de identificación desconocido")
	}
	if p.Phone != "" {
		digits := ""
		for _, ch := range p.Phone {
			if ch >= '0' && ch <= '9' {
				digits += string(ch)
			} else if ch != ' ' && ch != '-' {
				return errors.New("Teléfono inválido: usa 10 dígitos")
			}
		}
		if len(digits) != 10 {
			return errors.New("El teléfono debe tener 10 dígitos (XXX XXX XXXX)")
		}
		p.Phone = digits[:3] + " " + digits[3:6] + " " + digits[6:]
	}
	if p.Email != "" {
		if !emailBasic.MatchString(p.Email) {
			return errors.New("Correo electrónico inválido")
		}
		if a, e := mail.ParseAddress(p.Email); e != nil || a.Address != p.Email {
			return errors.New("Correo electrónico inválido")
		}
	}
	if p.BirthDate != "" {
		d, err := time.Parse("2006-01-02", p.BirthDate)
		if err != nil || d.After(time.Now()) {
			return errors.New("Fecha de nacimiento inválida")
		}
	}
	if p.AgeDeclared != "" {
		age, e := strconv.Atoi(p.AgeDeclared)
		if e != nil || age < 0 || age > 125 {
			return errors.New("Edad inválida")
		}
	}
	return nil
}

// Comprobación en servidor para evitar ingresar letras incluso saltándose el formulario.
// Los registros antiguos se respetan; solo se validan nuevas consultas.
var acuityPattern = regexp.MustCompile(`^[0-9+.,:/%\-]*$`)

func validateConsultation(c *Consultation) error {
	c.Motive = clean(c.Motive, 2000)
	c.Observations = clean(c.Observations, 5000)
	c.DiagnosisOD = clean(c.DiagnosisOD, 450)
	c.DiagnosisOI = clean(c.DiagnosisOI, 450)
	c.Professional = clean(c.Professional, 180)
	if c.Date == "" {
		c.Date = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", c.Date); err != nil {
		return errors.New("Fecha de consulta inválida")
	}
	if c.Professional == "" {
		return errors.New("Especifica el profesional que atendió")
	}
	acuityValues := []string{
		c.AVUnODFar, c.AVUnOIFar, c.AVUnODNear, c.AVUnOINear,
		c.AVCODFar, c.AVCOIFar, c.AVCODNear, c.AVCOINear,
	}
	for _, value := range acuityValues {
		if len(value) > 18 || !acuityPattern.MatchString(value) {
			return errors.New("Agudeza visual: ingresa solo números y signos (ej. 20/20 o 0.50), sin letras")
		}
	}
	for _, x := range []string{c.SphereOD, c.SphereOI, c.CylinderOD, c.CylinderOI, c.AxisOD, c.AxisOI, c.DPNOD, c.DPNOI, c.Addition} {
		if len(x) > 30 {
			return errors.New("Un valor de refracción es demasiado largo")
		}
	}
	if c.Motive == "" {
		return errors.New("Especifica el motivo de consulta")
	}
	return nil
}
func jsonReply(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	jsonReply(w, status, map[string]string{"error": msg})
}
func receive(r *http.Request, target interface{}) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("Se requiere application/json")
	}
	if r.ContentLength > 256000 {
		return errors.New("Datos demasiado grandes")
	}
	d := json.NewDecoder(io.LimitReader(r.Body, 256001))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return fmt.Errorf("Contenido inválido: %w", err)
	}
	var extra interface{}
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("Contenido JSON adicional")
	}
	return nil
}
func (s *Store) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	if r.Method != "GET" && r.Header.Get("X-RevVision-Client") != "revvision-local" {
		fail(w, 403, "Petición bloqueada")
		return
	}
	if s.rvBackupHandler(w,r) {return}
	if s.neonHandler(w,r) {return}
	if s.thermalHandler(w, r) {
		return
	}
	if s.commercial(w, r) {
		return
	}
	if r.URL.Path == "/api/health" {
		jsonReply(w, 200, map[string]string{"app": "RevVision", "version": appVersion, "storage": "LOCAL-OFFLINE"})
		return
	}
	if r.URL.Path == "/api/patients" {
		if r.Method == "GET" {
			s.mu.Lock()
			defer s.mu.Unlock()
			q := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("q")))
			if len(q) > 120 {
				fail(w, 400, "Búsqueda demasiado larga")
				return
			}
			all := make([]Patient, 0)
			for _, p := range s.data.Patients {
				if q == "" || strings.Contains(strings.ToUpper(p.FullName), q) || strings.Contains(strings.ToUpper(p.DocumentID), q) {
					all = append(all, p)
				}
			}
			sort.Slice(all, func(i, j int) bool { return all[i].UpdatedAt > all[j].UpdatedAt })
			if len(all) > 150 {
				all = all[:150]
			}
			jsonReply(w, 200, all)
			return
		}
		if r.Method == "POST" {
			var p Patient
			if err := receive(r, &p); err != nil {
				fail(w, 400, err.Error())
				return
			}
			if err := validatePatient(&p); err != nil {
				fail(w, 400, err.Error())
				return
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			for _, v := range s.data.Patients {
				if p.DocumentID != "" && strings.EqualFold(strings.TrimSpace(v.DocumentID), p.DocumentID) {
					fail(w, 409, "Ya existe un paciente con esta identificación: "+v.FullName)
					return
				}
			}
			next, err := clone(s.data)
			if err != nil {
				fail(w, 500, "Error de memoria")
				return
			}
			p.ID = newID()
			p.Version = 1
			p.CreatedAt = utcNow()
			p.UpdatedAt = p.CreatedAt
			next.Patients[p.ID] = p
			audit(&next, "create", "patient", p.ID, nil)
			if err := s.save(next); err != nil {
				log.Printf("save failure: %v", err)
				fail(w, 500, "No se pudo guardar; comprueba permisos y espacio")
				return
			}
			jsonReply(w, 201, p)
			return
		}
	}
	if strings.HasPrefix(r.URL.Path, "/api/patients/") {
		id := strings.TrimPrefix(r.URL.Path, "/api/patients/")
		s.mu.Lock()
		defer s.mu.Unlock()
		p, ok := s.data.Patients[id]
		if !ok {
			fail(w, 404, "No existe el paciente")
			return
		}
		if r.Method == "GET" {
			visits := []Consultation{}
			for _, c := range s.data.Consultations {
				if c.PatientID == id {
					visits = append(visits, c)
				}
			}
			sort.Slice(visits, func(i, j int) bool {
				if visits[i].Date == visits[j].Date {
					return visits[i].CreatedAt > visits[j].CreatedAt
				}
				return visits[i].Date > visits[j].Date
			})
			jsonReply(w, 200, map[string]interface{}{"patient": p, "consultations": visits})
			return
		}
		if r.Method == "PUT" {
			var inp Patient
			if err := receive(r, &inp); err != nil {
				fail(w, 400, err.Error())
				return
			}
			if inp.Version != p.Version {
				fail(w, 409, "Este paciente cambió desde que abriste la ficha. Actualiza la pantalla antes de editar.")
				return
			}
			if err := validatePatient(&inp); err != nil {
				fail(w, 400, err.Error())
				return
			}
			for _, v := range s.data.Patients {
				if v.ID != id && inp.DocumentID != "" && strings.EqualFold(strings.TrimSpace(v.DocumentID), inp.DocumentID) {
					fail(w, 409, "Otra ficha ya utiliza esta identificación")
					return
				}
			}
			inp.ID = id
			inp.CreatedAt = p.CreatedAt
			inp.UpdatedAt = utcNow()
			inp.Version = p.Version + 1
			next, _ := clone(s.data)
			next.Patients[id] = inp
			audit(&next, "update", "patient", id, p)
			if err := s.save(next); err != nil {
				log.Printf("save failure: %v", err)
				fail(w, 500, "No se pudo guardar")
				return
			}
			jsonReply(w, 200, inp)
			return
		}
	}
	if r.URL.Path == "/api/consultations" && r.Method == "POST" {
		var c Consultation
		if err := receive(r, &c); err != nil {
			fail(w, 400, err.Error())
			return
		}
		if err := validateConsultation(&c); err != nil {
			fail(w, 400, err.Error())
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if c.OperationID != "" {
			for _, v := range s.data.Consultations {
				if v.OperationID == c.OperationID {
					jsonReply(w, 200, v)
					return
				}
			}
		}
		if _, ok := s.data.Patients[c.PatientID]; !ok {
			fail(w, 404, "Paciente inexistente")
			return
		}
		next, _ := clone(s.data)
		c.ID = newID()
		c.Version = 1
		c.CreatedAt = utcNow()
		c.UpdatedAt = c.CreatedAt
		next.Consultations[c.ID] = c
		audit(&next, "create", "consultation", c.ID, nil)
		if err := s.save(next); err != nil {
			log.Printf("save failure: %v", err)
			fail(w, 500, "No se pudo guardar la consulta")
			return
		}
		jsonReply(w, 201, c)
		return
	}
	fail(w, 404, "Ruta desconocida")
}
func edgeExecutable() (string, error) {
	if os.Getenv("REVVISION_NO_BROWSER") == "1" {
		return "", errors.New("modo de pruebas sin ventana")
	}
	candidates := []string{}
	for _, v := range []string{"PROGRAMFILES(X86)", "PROGRAMFILES", "LOCALAPPDATA"} {
		if root := os.Getenv(v); root != "" {
			candidates = append(candidates, filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
	}
	for _, p := range candidates {
		if st, e := os.Stat(p); e == nil && !st.IsDir() {
			return p, nil
		}
	}
	if p, e := exec.LookPath("msedge.exe"); e == nil {
		return p, nil
	}
	return "", errors.New("No se encontró Microsoft Edge, necesario para la ventana independiente. Instala Edge o el motor WebView2.")
}

// Open a dedicated Chromium application window (no tabs or address bar).
// Uses the Windows Edge app-host instead of exposing the local server in a browser tab.
func openURL(url string) {
	if os.Getenv("REVVISION_NO_BROWSER") == "1" {
		return
	}
	if runtime.GOOS == "windows" {
		// A second launch should reuse, not duplicate, the independent Edge window.
		if windowcontrol.ActivateExisting() {return}
		edge, e := edgeExecutable()
		if e != nil {
			log.Printf("No se pudo mostrar la ventana de Rev Vision: %v", e)
			return
		}
		root := os.Getenv("LOCALAPPDATA")
		if root == "" {
			log.Printf("LOCALAPPDATA no está disponible; no se abrió navegador")
			return
		}
		profile := filepath.Join(root, "RevVision", "Pruebas", "edge-window-profile")
		if e = os.MkdirAll(profile, 0700); e != nil {
			log.Printf("perfil de ventana: %v", e)
			return
		}
		args := []string{"--app=" + url, "--user-data-dir=" + profile, "--no-first-run", "--no-default-browser-check", "--disable-background-mode"}
		cmd := exec.Command(edge, args...)
		if e = cmd.Start(); e != nil {
			log.Printf("Ventana Rev Vision: %v", e)
			return
		}
		// A dedicated profile lets the process remain associated with its app window.
		// If Edge delegates to an existing process, don't shut down the server immediately.
		started := time.Now()
		e = cmd.Wait()
		if time.Since(started) > 4*time.Second && e == nil {
			// Signal the hosting process to close when the app window is closed.
			requestOwnShutdown()
		}
		return
	}
	// Non-Windows is for development/testing only.
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", url)
	} else {
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// No clinical data is changed on window close. The regular server shutdown path
// flushes and closes existing handles; the next launch reopens the same database.
var onWindowClosed = make(chan struct{}, 1)

func requestOwnShutdown() {
	select {
	case onWindowClosed <- struct{}{}:
	default:
	}
}

type installControl struct {
	PID   int    `json:"pid"`
	Token string `json:"token"`
}

func makeInstallControl() (installControl, string, error) {
	dir, err := storeDir()
	if err != nil {
		return installControl{}, "", err
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return installControl{}, "", err
	}
	ctrl := installControl{PID: os.Getpid(), Token: hex.EncodeToString(b)}
	path := filepath.Join(dir, "installer-control.json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return installControl{}, "", err
	}
	if err = json.NewEncoder(f).Encode(ctrl); err != nil {
		f.Close()
		return installControl{}, "", err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return installControl{}, "", err
	}
	if err = f.Close(); err != nil {
		return installControl{}, "", err
	}
	return ctrl, path, nil
}
func cleanupControlFile(path, token string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var ctrl installControl
	if json.Unmarshal(b, &ctrl) == nil && ctrl.Token == token {
		_ = os.Remove(path)
	}
}
func main() {
	s, err := loadStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Rev Vision no se inició:", err)
		os.Exit(1)
	}
	address := "127.0.0.1:" + port
	listener, err := net.Listen("tcp", address)
	if err != nil { // no second instance: only open if existing server identifies itself.
		client := &http.Client{Timeout: 800 * time.Millisecond}
		resp, e := client.Get("http://" + address + "/api/health")
		if e == nil {
			defer resp.Body.Close()
			var info map[string]string
			_ = json.NewDecoder(resp.Body).Decode(&info)
			if info["app"] == "RevVision" {
				if !windowcontrol.ActivateExisting() {
					openURL("http://" + address)
				}
				return
			}
		}
		fmt.Fprintln(os.Stderr, "No se puede iniciar: puerto local ocupado. No se abrieron datos de otra aplicación.")
		os.Exit(1)
	}
	defer listener.Close()
	uiFS, _ := fs.Sub(ui, "ui")
	mux := http.NewServeMux()
	control, controlPath, controlErr := makeInstallControl()
	if controlErr != nil {
		log.Printf("No se pudo activar cierre controlado: %v", controlErr)
	} else {
		defer cleanupControlFile(controlPath, control.Token)
	}
	var shutdownRequested atomic.Bool
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 8192}
	mux.HandleFunc("/api/internal/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if controlErr != nil || r.Method != http.MethodPost || !strings.HasPrefix(r.RemoteAddr, "127.0.0.1:") || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-RevVision-Control")), []byte(control.Token)) != 1 {
			http.Error(w, "No autorizado", 403)
			return
		}
		if !shutdownRequested.CompareAndSwap(false, true) {
			http.Error(w, "Ya se está cerrando", 409)
			return
		}
		jsonReply(w, 200, map[string]string{"status": "closing", "pid": strconv.Itoa(os.Getpid())})
		go func() {
			time.Sleep(100 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
		}()
	})
	mux.HandleFunc("/api/", s.handler)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.FileServer(http.FS(uiFS)).ServeHTTP(w, r)
	}))
	server.Handler = mux
	// Background service only uploads ciphertext when owner explicitly enables
	// encrypted backups and activity is recent; AFK performs no Neon calls.
	cloudCtx, cloudCancel := context.WithCancel(context.Background())
	defer cloudCancel()
	go rvBackgroundBackup(cloudCtx,s)
	go func() {
		<-onWindowClosed
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	log.Printf("REV VISION %s | almacenamiento local y Neon opcional | %s", appVersion, address)
	go func() { time.Sleep(300 * time.Millisecond); openURL("http://" + address) }()
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
