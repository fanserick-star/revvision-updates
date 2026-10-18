-- Rev Visión - esquema PostgreSQL 18, migración inicial 001
-- Ya aplicado en el proyecto Neon independiente Rev Vision - Pruebas.
-- NO ejecutar en las bases de Recepción o Historia Clínica del Dr. Revelo.
BEGIN;
CREATE TABLE IF NOT EXISTS patients (
 id uuid PRIMARY KEY,
 full_name text NOT NULL CHECK(length(btrim(full_name)) BETWEEN 2 AND 180),
 document_id text NULL,
 birth_date date NULL,
 age_declared integer NULL CHECK(age_declared BETWEEN 0 AND 125),
 phone text NOT NULL DEFAULT '', email text NOT NULL DEFAULT '',
 address text NOT NULL DEFAULT '',occupation text NOT NULL DEFAULT '',
 notes text NOT NULL DEFAULT '',record_version integer NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),archived_at timestamptz NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS patients_document_unique
 ON patients(upper(btrim(document_id))) WHERE document_id IS NOT NULL AND btrim(document_id)<>'' AND archived_at IS NULL;
CREATE INDEX IF NOT EXISTS patients_name_search
 ON patients(upper(full_name) text_pattern_ops) WHERE archived_at IS NULL;
CREATE TABLE IF NOT EXISTS consultations (
 id uuid PRIMARY KEY,
 patient_id uuid NOT NULL REFERENCES patients(id) ON DELETE RESTRICT,
 seen_at timestamptz NOT NULL DEFAULT now(), motive text NOT NULL DEFAULT '',
 av_uncorrected_od_far text NOT NULL DEFAULT '',av_uncorrected_oi_far text NOT NULL DEFAULT '',
 av_uncorrected_od_near text NOT NULL DEFAULT '',av_uncorrected_oi_near text NOT NULL DEFAULT '',
 av_corrected_od_far text NOT NULL DEFAULT '',av_corrected_oi_far text NOT NULL DEFAULT '',
 av_corrected_od_near text NOT NULL DEFAULT '',av_corrected_oi_near text NOT NULL DEFAULT '',
 sphere_od text NOT NULL DEFAULT '',cylinder_od text NOT NULL DEFAULT '',axis_od text NOT NULL DEFAULT '',
 sphere_oi text NOT NULL DEFAULT '',cylinder_oi text NOT NULL DEFAULT '',axis_oi text NOT NULL DEFAULT '',
 addition text NOT NULL DEFAULT '',dpn_od text NOT NULL DEFAULT '',dpn_oi text NOT NULL DEFAULT '',
 lens_type text NOT NULL DEFAULT '',diagnosis_od text NOT NULL DEFAULT '',
 diagnosis_oi text NOT NULL DEFAULT '',cie10_od text NOT NULL DEFAULT '',cie10_oi text NOT NULL DEFAULT '',
 color_vision text NOT NULL DEFAULT '',observations text NOT NULL DEFAULT '',
 next_control text NOT NULL DEFAULT '',professional text NOT NULL DEFAULT '',
 record_version integer NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS consultations_by_patient ON consultations(patient_id,seen_at DESC);
CREATE TABLE IF NOT EXISTS generated_documents (
 id uuid PRIMARY KEY,consultation_id uuid NOT NULL REFERENCES consultations(id) ON DELETE RESTRICT,
 document_type text NOT NULL CHECK(document_type IN ('historial','certificado','receta')),
 sha256 char(64) NULL,issued_at timestamptz NOT NULL DEFAULT now(),
 issued_by text NOT NULL DEFAULT '',document_number text NULL
);
CREATE TABLE IF NOT EXISTS audit_events (
 id bigserial PRIMARY KEY,occurred_at timestamptz NOT NULL DEFAULT now(),
 device_id text NOT NULL,operation_id uuid NOT NULL UNIQUE,table_name text NOT NULL,
 record_id uuid NOT NULL,action text NOT NULL CHECK(action IN ('create','update','issue')),
 record_version integer NOT NULL
);
CREATE TABLE IF NOT EXISTS schema_migrations (
 version integer PRIMARY KEY,description text NOT NULL,
 installed_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO schema_migrations(version,description) VALUES
 (1,'Rev Vision esquema inicial: pacientes, consultas, documentos y auditoria')
 ON CONFLICT(version) DO NOTHING;
COMMIT;

-- Esquema Rev Visión 2, extensión no destructiva. Aplicado SOLO a su proyecto Neon propio.
BEGIN;
ALTER TABLE patients ADD COLUMN IF NOT EXISTS document_type text NOT NULL DEFAULT 'cedula';
CREATE TABLE IF NOT EXISTS inventory_products (
 id uuid PRIMARY KEY, sku varchar(36) NOT NULL UNIQUE, name varchar(150) NOT NULL,
 category varchar(60) NOT NULL DEFAULT 'OTROS', price_cents bigint NOT NULL CHECK(price_cents>=0 AND price_cents<=100000000),
 stock integer NOT NULL DEFAULT 0 CHECK(stock>=0),track_stock boolean NOT NULL DEFAULT true,
 active boolean NOT NULL DEFAULT true, record_version integer NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS sales_orders (
 id uuid PRIMARY KEY,patient_id uuid NOT NULL REFERENCES patients(id) ON DELETE RESTRICT,
 consultation_id uuid NOT NULL UNIQUE REFERENCES consultations(id) ON DELETE RESTRICT,
 operation_id uuid NOT NULL UNIQUE,total_cents bigint NOT NULL CHECK(total_cents>=0 AND total_cents<=100000000),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS sales_order_items (
 id uuid PRIMARY KEY,order_id uuid NOT NULL REFERENCES sales_orders(id) ON DELETE RESTRICT,
 product_id uuid NOT NULL REFERENCES inventory_products(id) ON DELETE RESTRICT,
 sku text NOT NULL,name text NOT NULL,quantity integer NOT NULL CHECK(quantity>0),
 unit_price_cents bigint NOT NULL CHECK(unit_price_cents>=0),
 subtotal_cents bigint NOT NULL CHECK(subtotal_cents>=0)
);
CREATE TABLE IF NOT EXISTS sales_payments (
 id uuid PRIMARY KEY,order_id uuid NOT NULL REFERENCES sales_orders(id) ON DELETE RESTRICT,
 operation_id uuid NOT NULL UNIQUE,amount_cents bigint NOT NULL CHECK(amount_cents>0),
 method varchar(24) NOT NULL CHECK(method IN ('EFECTIVO','TRANSFERENCIA','TARJETA','OTRO')),
 note text NOT NULL DEFAULT '',receipt_number varchar(50) NOT NULL UNIQUE,
 paid_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS payments_by_order ON sales_payments(order_id,paid_at);
CREATE INDEX IF NOT EXISTS sales_by_patient ON sales_orders(patient_id,created_at DESC);
INSERT INTO schema_migrations(version,description) VALUES
 (2,'Rev Vision: inventario, pedidos, abonos y comprobantes') ON CONFLICT(version) DO NOTHING;
COMMIT;
