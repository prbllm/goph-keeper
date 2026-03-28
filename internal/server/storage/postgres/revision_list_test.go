package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListRevisionEventsAfter_orphanRevisionNoVaultRow(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	userID := "user-1"
	after := uint64(0)
	limit := 5
	changed := time.Unix(1700000000, 0)

	rows := sqlmock.NewRows([]string{
		"revision_id", "r_item_id", "change_type", "item_version", "changed_at",
		"v_item_id", "v_user_id", "v_item_type",
		"title_ciphertext", "title_nonce", "metadata_ciphertext", "metadata_nonce",
		"payload_ciphertext", "payload_nonce",
		"blob_id", "version", "checksum", "created_at", "updated_at", "deleted_at",
	}).AddRow(uint64(1), "orphan-item", int16(1), uint64(1), changed,
		nil, nil, nil,
		nil, nil, nil, nil,
		nil, nil,
		nil, nil, nil, nil, nil, nil)

	mock.ExpectQuery(`SELECT r.revision_id`).
		WithArgs(userID, after, limit).
		WillReturnRows(rows)

	out, err := ListRevisionEventsAfter(context.Background(), db, userID, after, limit)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].RevisionID != 1 || out[0].ItemID != "orphan-item" || out[0].Item != nil {
		t.Fatalf("unexpected row: %+v", out[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
