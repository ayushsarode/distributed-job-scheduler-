-- +goose Up
CREATE TABLE workflows (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    agent_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'published', 'inactive')) DEFAULT 'draft',
    created_by TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_workflow_name_per_tenant UNIQUE(tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_workflows_tenant_id ON workflows(tenant_id, id);
CREATE INDEX IF NOT EXISTS idx_workflows_agent_tenant_status ON workflows(agent_id, tenant_id, status);

-- +goose Down
DROP INDEX IF EXISTS idx_workflows_agent_tenant_status;
DROP INDEX IF EXISTS idx_workflows_tenant_id;
DROP TABLE IF EXISTS workflows;
