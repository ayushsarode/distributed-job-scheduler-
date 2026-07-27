-- name: InsertWorkflow :one
INSERT INTO workflows (
    id,
    tenant_id,
    name,
    description,
    agent_id,
    status,
    created_by,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: UpdateWorkflow :exec
UPDATE workflows
SET name = $3,
    description = $4,
    agent_id = $5,
    status = $6,
    created_by = $7,
    created_at = $8,
    updated_at = $9
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: GetWorkflow :one
SELECT *
FROM workflows
WHERE id = $1 AND tenant_id = $2;

-- name: INTERNALGetWorkflow :one
SELECT * FROM workflows
WHERE id = $1;

-- name: DeleteWorkflow :exec
DELETE FROM workflows
WHERE id = $1 AND tenant_id = $2;

-- name: ListWorkflows :many
SELECT *
FROM workflows
WHERE tenant_id = $1
ORDER BY id DESC;

-- name: HasPublishedWorkflowsForAgent :one
SELECT EXISTS (
  SELECT 1
  FROM workflows
  WHERE agent_id = $1::text
    AND tenant_id = $2
    AND status = 'published'
) AS has_published_workflow;
