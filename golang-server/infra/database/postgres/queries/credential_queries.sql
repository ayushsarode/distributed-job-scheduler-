-- name: InsertCredential :one
INSERT INTO credentials (
    id,
    tenant_id,
    credential_name,
    credential_type,
    credential_metadata,
    encrypted_payload,
    encrypted_data_key,
    nonce,
    created_by,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: UpdateCredentials :one
UPDATE credentials
SET credential_name = $3,
    credential_type = $4,
    credential_metadata = $5,
    encrypted_payload = $6,
    encrypted_data_key = $7,
    nonce = $8,
    created_by = $9,
    created_at = $10,
    updated_at = $11
WHERE id = $1 AND tenant_id = $2
RETURNING *;


-- name: GetCredentialEncrypted :one
SELECT
    id,
    tenant_id,
    credential_name,
    credential_type,
    credential_metadata,
    encrypted_payload,
    encrypted_data_key,
    nonce,
    created_by,
    created_at,
    updated_at
FROM credentials
WHERE id = $1 AND tenant_id = $2;

-- name: INTERNALgetCredential :one
SELECT
    id,
    tenant_id,
    credential_name,
    credential_type,
    credential_metadata,
    encrypted_payload,
    encrypted_data_key,
    nonce,
    created_by,
    created_at,
    updated_at
FROM credentials
WHERE id = $1;

-- name: DeleteCredential :exec
DELETE FROM credentials
WHERE id = $1 AND tenant_id = $2;

-- name: ListCredentials :many
SELECT
    id,
    tenant_id,
    credential_name,
    credential_type,
    credential_metadata,
    encrypted_payload,
    encrypted_data_key,
    nonce,
    created_by,
    created_at,
    updated_at
FROM credentials
WHERE tenant_id = $1
ORDER BY id DESC;