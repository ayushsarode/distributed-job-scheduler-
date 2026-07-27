-- +goose Up
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS users (
    user_id TEXT PRIMARY KEY,
    first_name TEXT,
    last_name TEXT,
    tenant_id UUID NOT NULL REFERENCES tenants(id)
);

CREATE TABLE IF NOT EXISTS document (
    id UUID PRIMARY KEY,
    document_name TEXT NOT NULL,
    document_url TEXT NOT NULL,
    is_indexed BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    created_by TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, document_name)
);

CREATE TABLE IF NOT EXISTS agent (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    create_agent_request_json BYTEA NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    deployment_status TEXT NOT NULL DEFAULT 'not_deployed'
);

CREATE INDEX IF NOT EXISTS idx_agent_tenant_id ON agent (tenant_id, id);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users (tenant_id);

-- +goose Down
DROP INDEX IF EXISTS idx_agent_tenant_id;
DROP INDEX IF EXISTS idx_users_tenant_id;
DROP TABLE IF EXISTS agent;
DROP TABLE IF EXISTS document;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
