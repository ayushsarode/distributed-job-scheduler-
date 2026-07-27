-- name: InsertAuditLog :one
INSERT INTO audit_logs (
    id,
    tenant_id,
    user_id,
    action,
    resource_type,
    resource_id,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;