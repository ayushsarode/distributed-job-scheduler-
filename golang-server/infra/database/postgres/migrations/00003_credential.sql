-- +goose Up
CREATE TABLE IF NOT EXISTS credentials (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    credential_name TEXT NOT NULL,
    credential_type TEXT NOT NULL,
    credential_metadata BYTEA NOT NULL,
    encrypted_payload BYTEA NOT NULL,
    encrypted_data_key BYTEA NOT NULL,
    nonce BYTEA NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE (tenant_id, credential_name)
);

CREATE INDEX IF NOT EXISTS idx_credentials_tenant_id ON credentials(tenant_id);
CREATE INDEX IF NOT EXISTS idx_credentials_tenant_type ON credentials(tenant_id, credential_type);

-- +goose Down
DROP INDEX IF EXISTS idx_credentials_tenant_type;
DROP INDEX IF EXISTS idx_credentials_tenant_id;
DROP TABLE IF EXISTS credentials;
