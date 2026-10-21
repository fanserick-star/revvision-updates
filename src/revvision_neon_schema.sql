-- Esquema exclusivo de Revvision. No mezclar con Mi Optica ni el consultorio.
CREATE SCHEMA IF NOT EXISTS revvision;
CREATE TABLE IF NOT EXISTS revvision.ownership (id INTEGER PRIMARY KEY CHECK(id=1), owner_id TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE TABLE IF NOT EXISTS revvision.encrypted_backups (id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, owner_id TEXT NOT NULL, fingerprint TEXT NOT NULL, ciphertext TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), UNIQUE(owner_id,fingerprint));
