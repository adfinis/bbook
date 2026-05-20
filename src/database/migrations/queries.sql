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
