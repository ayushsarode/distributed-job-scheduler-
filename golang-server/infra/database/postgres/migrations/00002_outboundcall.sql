-- +goose Up
CREATE TABLE IF NOT EXISTS outbound_call_document (
    id UUID PRIMARY KEY,
    document_name TEXT NOT NULL,
    document_url TEXT NOT NULL,
    document_type TEXT NOT NULL,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    created_by TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS call_jobs (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    document_id TEXT NOT NULL,
    document_type TEXT NOT NULL,
    preffered_language TEXT,
    max_retries INTEGER,
    retry_delay INTEGER,
    outbound_call_provider_id TEXT,  -- Maps to credential(id)
    workflow_id TEXT NOT NULL,
    created_by TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'ready', 'running', 'completed', 'failed', 'unknown')),
    is_materialised BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS job_item (
    id UUID PRIMARY KEY,
    phone_no TEXT NOT NULL,
    agent_context TEXT NOT NULL,
    job_id UUID NOT NULL,
    job_data TEXT,
    external_call_id TEXT,
    call_status TEXT DEFAULT 'unknown'
     CHECK (call_status IN (
        'pending', 'initiated', 'queued', 'ringing', 'answered',
        'completed', 'busy', 'failed', 'no-answer', 'canceled', 'unknown')),
    call_started_at TIMESTAMP,
    call_ended_at TIMESTAMP,
    call_duration INTEGER,
    call_error_message TEXT,
    created_by TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_job_id FOREIGN KEY (job_id) REFERENCES call_jobs (id) ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_call_jobs_tenant_id ON call_jobs(tenant_id, id);
CREATE INDEX IF NOT EXISTS idx_call_jobs_workflow ON call_jobs(workflow_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_call_jobs_workflow_status ON call_jobs(workflow_id, tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_job_item_job_tenant ON job_item(job_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_job_item_external_call_id ON job_item(external_call_id);

-- +goose Down
DROP INDEX IF EXISTS idx_job_item_external_call_id;
DROP INDEX IF EXISTS idx_job_item_job_tenant;
DROP INDEX IF EXISTS idx_call_jobs_workflow_status;
DROP INDEX IF EXISTS idx_call_jobs_workflow;
DROP INDEX IF EXISTS idx_call_jobs_tenant_id;
DROP TABLE IF EXISTS job_item;
DROP TABLE IF EXISTS call_jobs;
DROP TABLE IF EXISTS outbound_call_document;
