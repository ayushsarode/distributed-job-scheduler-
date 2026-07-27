-- name: InsertDocument :one
INSERT INTO
document(id, document_name, document_url, is_indexed, is_active, created_by, tenant_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListDocuments :many
SELECT * FROM document
WHERE tenant_id = $1 AND
is_deleted = false
ORDER BY id DESC
LIMIT $2
OFFSET $3;

-- name: InsertAgent :one
INSERT INTO
agent(id, create_agent_request_json, name, created_at, updated_at, created_by, tenant_id, deployment_status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;


-- name: GetAgent :one
SELECT * FROM agent
WHERE id = $1 AND
tenant_id = $2;

-- name: INTERNALGetAgent :one
SELECT * FROM agent
WHERE id = $1;

-- name: ListAgents :many
SELECT * FROM agent
WHERE tenant_id = $1
ORDER BY id DESC
LIMIT $2
OFFSET $3;

-- name: UpdateAgentDeploymentStatus :exec
UPDATE agent
SET deployment_status = $2,
    updated_at = $3
WHERE id = $1;

-- name: UpdateAgent :exec
UPDATE agent
SET name = $1,
    create_agent_request_json = $2,
    deployment_status = $3,
    updated_at = $4
WHERE id = $5 AND tenant_id = $6
RETURNING *;

-- name: RegisterUser :one
INSERT INTO users (user_id, first_name, last_name, tenant_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: InsertTenant :one
INSERT INTO tenants (id, name, created_at, updated_at, is_active)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserTenantID :one
SELECT tenant_id FROM users WHERE user_id = $1;


-- name: UpdateDocument :one
UPDATE document
SET
    is_active = $1,
    is_indexed = $2,
    updated_at = $3,
    is_deleted = $4
WHERE
    id = $5
    AND tenant_id = $6
    AND is_deleted = false
RETURNING *;


-- name: GetDocumentById :one
SELECT * FROM document
WHERE id = $1 AND
tenant_id = $2 AND
is_deleted = false;



-- name: UploadOutboundCallDocument :one
INSERT INTO outbound_call_document(id, document_name, document_url, document_type, created_by, tenant_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListOutboundCallDocuments :many
SELECT * FROM outbound_call_document
WHERE tenant_id = $1
AND is_deleted = false
ORDER BY id DESC
LIMIT $2
OFFSET $3;

-- name: GetOutboundCallDocument :one
SELECT * FROM outbound_call_document
WHERE id = $1 AND
tenant_id = $2
AND is_deleted = false;

-- name: DeleteOutboundCallDocument :one
UPDATE outbound_call_document
SET is_deleted = TRUE
WHERE id = $1 AND
tenant_id = $2
RETURNING *;

-- name: InsertCallJob :one
INSERT INTO call_jobs (
    id,
    name,
    workflow_id,
    document_id,
    document_type,
    preffered_language,
    max_retries,
    retry_delay,
    outbound_call_provider_id,
    is_materialised,
    created_by,
    tenant_id,
    status
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
RETURNING *;

-- name: GetCallJob :one
SELECT *
FROM call_jobs
WHERE id = $1 AND tenant_id = $2;

-- name: DeleteCallJob :exec
DELETE FROM call_jobs
WHERE id = $1 AND tenant_id = $2;

-- name: UpdateCallJob :exec
UPDATE call_jobs
SET
    name = $3,
    workflow_id = $4,
    document_id = $5,
    document_type = $6,
    preffered_language = $7,
    max_retries = $8,
    retry_delay = $9,
    outbound_call_provider_id = $10,
    is_materialised = $11,
    status = $12,
    updated_at = $13
WHERE id = $1 AND tenant_id = $2;

-- name: GetWorkflowCallJobCount :one
SELECT CAST(COUNT(*) AS INTEGER) AS count
FROM call_jobs
WHERE workflow_id = $1 AND tenant_id = $2;

-- name: GetWorkflowCallJobCountByStatuses :one
SELECT CAST(COUNT(*) AS INTEGER) AS count
FROM call_jobs
WHERE workflow_id = $1
  AND tenant_id = $2
  AND status = ANY(sqlc.slice('statuses')::text[]);

-- name: UpdateCallJobStatus :exec
UPDATE call_jobs
SET
    status = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1 AND tenant_id = $3;


-- name: InsertJobItem :exec
INSERT INTO job_item (
    id,
    phone_no,
    agent_context,
    call_status,
    job_id,
    job_data,
    created_by,
    tenant_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
);

-- name: INTERNALGetJobItemById :one
SELECT *
FROM job_item
WHERE id = $1;

-- name: UpdateJobItem :exec
UPDATE job_item
SET call_status = $3,
    updated_at = $4,
    call_started_at = $5,
    call_ended_at = $6,
    call_duration = $7,
    call_error_message = $8,
    job_data = $9,
    external_call_id = $10
WHERE id = $1 AND tenant_id = $2;

-- name: GetJobItemByExternalCallId :one
SELECT *
FROM job_item
WHERE external_call_id = $1;

-- name: UpdateJobItemCallStatus :exec
UPDATE job_item
SET
    call_status = $1,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $2;

-- name: GetJobItemTenantById :one
SELECT cj.tenant_id
FROM job_item ji
JOIN call_jobs cj ON ji.job_id = cj.id
WHERE ji.id = $1;

-- name: GetJobItemsByIDs :many
SELECT *
FROM job_item
WHERE job_id = sqlc.arg(source_job_id)
  AND id = ANY(sqlc.arg(job_item_ids)::uuid[])
  AND tenant_id = sqlc.arg(tenant_id);

-- name: DeleteJobItem :exec
DELETE FROM job_item
WHERE id = $1 AND job_id = $2 AND tenant_id = $3;
