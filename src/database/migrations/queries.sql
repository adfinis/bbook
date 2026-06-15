-- name: UpsertContact :exec
INSERT INTO contacts (
    zoho_id, first_name, last_name, organization, email, phone, mobile,
    street, city, postal_code, status, modified_time, raw, synced_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
ON CONFLICT (zoho_id) DO UPDATE SET
    (first_name, last_name, organization, email, phone, mobile,
     street, city, postal_code, status, modified_time, raw, synced_at) =
    (EXCLUDED.first_name, EXCLUDED.last_name, EXCLUDED.organization,
     EXCLUDED.email, EXCLUDED.phone, EXCLUDED.mobile, EXCLUDED.street,
     EXCLUDED.city, EXCLUDED.postal_code, EXCLUDED.status,
     EXCLUDED.modified_time, EXCLUDED.raw, EXCLUDED.synced_at);

-- name: AllContacts :many
SELECT * FROM contacts ORDER BY last_name, first_name;

-- name: DeleteContactsSyncedBefore :execrows
DELETE FROM contacts WHERE synced_at < $1;

-- name: TokenExists :one
SELECT EXISTS(SELECT 1 FROM tokens WHERE id = $1);

-- name: ListTokensForUser :many
SELECT id, name, created_at FROM tokens WHERE user_sub = $1 ORDER BY created_at DESC;

-- name: CreateTokenForUser :one
INSERT INTO tokens (user_sub) VALUES ($1) RETURNING id, created_at;

-- name: UpdateTokenName :execrows
UPDATE tokens SET name = $3 WHERE id = $1 AND user_sub = $2;

-- name: DeleteTokenForUser :execrows
DELETE FROM tokens WHERE id = $1 AND user_sub = $2;

-- name: DeleteTokensForUserSubs :execrows
DELETE FROM tokens WHERE user_sub = ANY($1::text[]);

-- name: UpsertOfflineToken :exec
INSERT INTO user_offline_tokens (user_sub, offline_token, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (user_sub) DO UPDATE SET
    offline_token = EXCLUDED.offline_token,
    updated_at    = EXCLUDED.updated_at;

-- name: ListOfflineTokens :many
SELECT user_sub, offline_token FROM user_offline_tokens;

-- name: DeleteOfflineTokensForUserSubs :execrows
DELETE FROM user_offline_tokens WHERE user_sub = ANY($1::text[]);
