package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/vault"
)

// RevisionEventRow is a revision_log row joined with the current vault_items row.
type RevisionEventRow struct {
	RevisionID  uint64
	ItemID      string
	ChangeType  vault.ChangeType
	ItemVersion uint64
	ChangedAt   time.Time
	Item        *vault.Item
}

// UserMaxRevision returns the highest revision_id for the user, or 0 if none.
func UserMaxRevision(ctx context.Context, exec Executor, userID string) (uint64, error) {
	const q = `SELECT COALESCE(MAX(revision_id), 0) FROM revision_log WHERE user_id = $1`
	var max uint64
	if err := exec.QueryRowContext(ctx, q, userID).Scan(&max); err != nil {
		return 0, err
	}
	return max, nil
}

// ListRevisionEventsAfter returns revision events with revision_id strictly greater than afterRevision.
// vault_items is LEFT JOINed so revision rows remain visible if the item row is missing (orphan log).
func ListRevisionEventsAfter(ctx context.Context, exec Executor, userID string, afterRevision uint64, limit int) ([]RevisionEventRow, error) {
	if limit <= 0 {
		return nil, nil
	}
	const q = `
SELECT r.revision_id, r.item_id, r.change_type, r.item_version, r.changed_at,
       v.item_id, v.user_id, v.item_type,
       v.title_ciphertext, v.title_nonce,
       v.metadata_ciphertext, v.metadata_nonce,
       v.payload_ciphertext, v.payload_nonce,
       v.blob_id, v.version, v.checksum, v.created_at, v.updated_at, v.deleted_at
FROM revision_log r
LEFT JOIN vault_items v ON v.item_id = r.item_id AND v.user_id = r.user_id
WHERE r.user_id = $1 AND r.revision_id > $2
ORDER BY r.revision_id ASC
LIMIT $3`
	rows, err := exec.QueryContext(ctx, q, userID, afterRevision, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RevisionEventRow
	for rows.Next() {
		var (
			revID       uint64
			rItemID     string
			changeType  int16
			itemVersion uint64
			changedAt   time.Time
			vItemID     sql.NullString
			vUserID     sql.NullString
			vItemType   sql.NullInt16
			titleCT     []byte
			titleNonce  []byte
			metaCT      []byte
			metaNonce   []byte
			payloadCT   []byte
			payloadN    []byte
			blobID      sql.NullString
			vVersion    sql.NullInt64
			checksum    []byte
			createdAt   sql.NullTime
			updatedAt   sql.NullTime
			deletedAt   sql.NullTime
		)
		if err := rows.Scan(
			&revID, &rItemID, &changeType, &itemVersion, &changedAt,
			&vItemID, &vUserID, &vItemType,
			&titleCT, &titleNonce,
			&metaCT, &metaNonce,
			&payloadCT, &payloadN,
			&blobID, &vVersion, &checksum, &createdAt, &updatedAt, &deletedAt,
		); err != nil {
			return nil, err
		}
		row := RevisionEventRow{
			RevisionID:  revID,
			ItemID:      rItemID,
			ChangeType:  vault.ChangeType(changeType),
			ItemVersion: itemVersion,
			ChangedAt:   changedAt,
		}
		if vItemID.Valid && vItemType.Valid && vVersion.Valid {
			it := vault.Item{
				ItemID:             vItemID.String,
				UserID:             vUserID.String,
				ItemType:           vItemType.Int16,
				TitleCiphertext:    titleCT,
				TitleNonce:         titleNonce,
				MetadataCiphertext: metaCT,
				MetadataNonce:      metaNonce,
				PayloadCiphertext:  payloadCT,
				PayloadNonce:       payloadN,
				Version:            uint64(vVersion.Int64),
				Checksum:           checksum,
			}
			if createdAt.Valid {
				it.CreatedAt = createdAt.Time
			}
			if updatedAt.Valid {
				it.UpdatedAt = updatedAt.Time
			}
			if blobID.Valid {
				s := blobID.String
				it.BlobID = &s
			}
			if deletedAt.Valid {
				t := deletedAt.Time
				it.DeletedAt = &t
			}
			row.Item = &it
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
