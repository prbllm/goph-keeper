package vault

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Clock func() time.Time

type EngineService struct {
	repo         Repository
	revisions    RevisionLogRepository
	processedOps ProcessedOperationsRepository
	now          Clock
	log          *zap.Logger
}

func NewEngine(
	repo Repository,
	revisions RevisionLogRepository,
	processedOps ProcessedOperationsRepository,
	now Clock,
	log *zap.Logger,
) *EngineService {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &EngineService{
		repo:         repo,
		revisions:    revisions,
		processedOps: processedOps,
		now:          now,
		log:          log,
	}
}

func (e *EngineService) Create(ctx context.Context, userID, operationID string, in CreateInput) (*CreateOutput, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(operationID) == "" {
		return nil, ErrInvalidArgument
	}
	if op, err := e.processedOps.Lookup(ctx, userID, operationID); err != nil {
		return nil, err
	} else if op != nil {
		item, err := e.repo.GetItem(ctx, userID, op.ItemID)
		if err != nil {
			if err == ErrNotFound {
				e.log.Warn("vault: Create idempotent lookup found operation but item is missing",
					zap.String("user_id", userID),
					zap.String("operation_id", operationID),
					zap.String("item_id", op.ItemID))
				return nil, ErrNotFound
			}
			return nil, err
		}
		return &CreateOutput{
			ItemID:         op.ItemID,
			Version:        item.Version,
			ServerRevision: op.RevisionID,
		}, nil
	}
	now := e.now()
	itemID := uuid.NewString()

	item := &Item{
		ItemID:             itemID,
		UserID:             userID,
		ItemType:           in.ItemType,
		TitleCiphertext:    in.TitleCiphertext,
		TitleNonce:         in.TitleNonce,
		MetadataCiphertext: in.MetadataCiphertext,
		MetadataNonce:      in.MetadataNonce,
		PayloadCiphertext:  in.PayloadCiphertext,
		PayloadNonce:       in.PayloadNonce,
		BlobID:             in.BlobID,
		Version:            1,
		Checksum:           in.Checksum,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := e.repo.CreateItem(ctx, item); err != nil {
		return nil, err
	}

	entry := &RevisionLogEntry{
		UserID:      userID,
		OperationID: &operationID,
		ItemID:      itemID,
		ChangeType:  ChangeTypeCreated,
		ItemVersion: item.Version,
		ChangedAt:   now,
	}
	if err := e.revisions.AppendRevision(ctx, entry); err != nil {
		return nil, err
	}

	if err := e.processedOps.MarkProcessed(ctx, userID, operationID, itemID, entry.RevisionID); err != nil {
		return nil, err
	}

	return &CreateOutput{
		ItemID:         itemID,
		Version:        item.Version,
		ServerRevision: entry.RevisionID,
	}, nil
}

func (e *EngineService) Update(ctx context.Context, userID, operationID, itemID string, expectedVersion uint64, in UpdateInput) (*UpdateOutput, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(operationID) == "" || strings.TrimSpace(itemID) == "" {
		return nil, ErrInvalidArgument
	}

	if op, err := e.processedOps.Lookup(ctx, userID, operationID); err != nil {
		return nil, err
	} else if op != nil {
		if op.ItemID != "" && op.ItemID != itemID {
			e.log.Warn("vault: Update called with mismatched itemID for processed operation",
				zap.String("user_id", userID),
				zap.String("operation_id", operationID),
				zap.String("stored_item_id", op.ItemID),
				zap.String("request_item_id", itemID))
			return nil, ErrInvalidArgument
		}
		item, err := e.repo.GetItem(ctx, userID, op.ItemID)
		if err != nil {
			return nil, err
		}
		return &UpdateOutput{
			NewVersion:     item.Version,
			ServerRevision: op.RevisionID,
		}, nil
	}

	current, err := e.repo.GetItem(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}
	if current.DeletedAt != nil {
		return nil, ErrNotFound
	}
	if current.Version != expectedVersion {
		return nil, ErrConflict
	}

	now := e.now()
	current.TitleCiphertext = in.TitleCiphertext
	current.TitleNonce = in.TitleNonce
	current.MetadataCiphertext = in.MetadataCiphertext
	current.MetadataNonce = in.MetadataNonce
	current.PayloadCiphertext = in.PayloadCiphertext
	current.PayloadNonce = in.PayloadNonce
	current.BlobID = in.BlobID
	current.Checksum = in.Checksum
	current.Version++
	current.UpdatedAt = now

	if err := e.repo.UpdateItem(ctx, current, expectedVersion); err != nil {
		return nil, err
	}

	entry := &RevisionLogEntry{
		UserID:      userID,
		OperationID: &operationID,
		ItemID:      itemID,
		ChangeType:  ChangeTypeUpdated,
		ItemVersion: current.Version,
		ChangedAt:   now,
	}
	if err := e.revisions.AppendRevision(ctx, entry); err != nil {
		return nil, err
	}

	if err := e.processedOps.MarkProcessed(ctx, userID, operationID, itemID, entry.RevisionID); err != nil {
		return nil, err
	}

	return &UpdateOutput{
		NewVersion:     current.Version,
		ServerRevision: entry.RevisionID,
	}, nil
}

func (e *EngineService) Delete(ctx context.Context, userID, operationID, itemID string, expectedVersion uint64) (*DeleteOutput, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(operationID) == "" || strings.TrimSpace(itemID) == "" {
		return nil, ErrInvalidArgument
	}

	if op, err := e.processedOps.Lookup(ctx, userID, operationID); err != nil {
		return nil, err
	} else if op != nil {
		if op.ItemID != "" && op.ItemID != itemID {
			e.log.Warn("vault: Delete called with mismatched itemID for processed operation",
				zap.String("user_id", userID),
				zap.String("operation_id", operationID),
				zap.String("stored_item_id", op.ItemID),
				zap.String("request_item_id", itemID))
			return nil, ErrInvalidArgument
		}
		item, err := e.repo.GetItem(ctx, userID, op.ItemID)
		if err != nil {
			if err == ErrNotFound {
				e.log.Warn("vault: Delete idempotent lookup found operation but item is missing",
					zap.String("user_id", userID),
					zap.String("operation_id", operationID),
					zap.String("item_id", op.ItemID))
				return nil, ErrNotFound
			}
			return nil, err
		}
		return &DeleteOutput{
			NewVersion:     item.Version,
			ServerRevision: op.RevisionID,
		}, nil
	}

	now := e.now()
	newVersion, err := e.repo.DeleteItem(ctx, userID, itemID, expectedVersion, now)
	if err != nil {
		return nil, err
	}

	entry := &RevisionLogEntry{
		UserID:      userID,
		OperationID: &operationID,
		ItemID:      itemID,
		ChangeType:  ChangeTypeDeleted,
		ItemVersion: newVersion,
		ChangedAt:   now,
	}
	if err := e.revisions.AppendRevision(ctx, entry); err != nil {
		return nil, err
	}
	if err := e.processedOps.MarkProcessed(ctx, userID, operationID, itemID, entry.RevisionID); err != nil {
		return nil, err
	}

	return &DeleteOutput{
		NewVersion:     newVersion,
		ServerRevision: entry.RevisionID,
	}, nil
}

func (e *EngineService) Get(ctx context.Context, userID, itemID string) (*Item, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(itemID) == "" {
		return nil, ErrInvalidArgument
	}
	return e.repo.GetItem(ctx, userID, itemID)
}

func (e *EngineService) List(ctx context.Context, userID string, includeDeleted bool, pageSize int32, pageToken string) ([]*Item, string, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, "", ErrInvalidArgument
	}
	return e.repo.ListItems(ctx, userID, includeDeleted, pageSize, pageToken)
}
