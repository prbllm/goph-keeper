package grpcserver

import (
	"context"
	"strings"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/blob"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/vault"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// vaultHandler serves VaultService RPCs.
type vaultHandler struct {
	gophkeeperv1.UnimplementedVaultServiceServer
	logger      *zap.Logger
	engine      vault.Engine
	cfg         *config.Config
	now         func() time.Time
	blobRepo    blob.Repository
	blobStorage blob.ObjectStorage
}

func (h vaultHandler) CreateItem(ctx context.Context, req *gophkeeperv1.CreateItemRequest) (*gophkeeperv1.CreateItemResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid CreateItem request: request is required")
	}
	if req.GetTitle() == nil || req.GetMetadata() == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid CreateItem request: title and metadata are required")
	}
	if strings.TrimSpace(req.GetBlobId()) == "" && req.GetPayload() == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid CreateItem request: payload or blob_id is required")
	}

	itemType, ok := concreteProtoItemType(req.GetItemType())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid CreateItem request: item_type must be a concrete type")
	}

	pc, pn, bid, sum, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, h.now, userID, req.GetPayload(), req.GetBlobId())
	if err != nil {
		return nil, err
	}

	out, err := h.engine.Create(ctx, userID, req.GetOperationId(), vault.CreateInput{
		ItemType:           itemType,
		TitleCiphertext:    req.GetTitle().GetCiphertext(),
		TitleNonce:         req.GetTitle().GetNonce(),
		MetadataCiphertext: req.GetMetadata().GetCiphertext(),
		MetadataNonce:      req.GetMetadata().GetNonce(),
		PayloadCiphertext:  pc,
		PayloadNonce:       pn,
		BlobID:             bid,
		Checksum:           sum,
	})
	if err != nil {
		return nil, toVaultStatusError(err)
	}
	return &gophkeeperv1.CreateItemResponse{
		ItemId:         out.ItemID,
		Version:        out.Version,
		ServerRevision: out.ServerRevision,
	}, nil
}

func (h vaultHandler) GetItem(ctx context.Context, req *gophkeeperv1.GetItemRequest) (*gophkeeperv1.GetItemResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid GetItem request: request is required")
	}
	if req.GetItemId() == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid GetItem request: item_id is required")
	}
	item, err := h.engine.Get(ctx, userID, req.GetItemId())
	if err != nil {
		return nil, toVaultStatusError(err)
	}
	if item == nil {
		log := h.logger
		if log == nil {
			log = zap.NewNop()
		}
		log.Error("vault: Get returned nil item without error",
			zap.String("user_id", userID),
			zap.String("item_id", req.GetItemId()))
		return nil, status.Error(codes.Internal, "vault: internal error")
	}
	return &gophkeeperv1.GetItemResponse{
		Item: toProtoItem(item),
	}, nil
}

func (h vaultHandler) ListItems(ctx context.Context, req *gophkeeperv1.ListItemsRequest) (*gophkeeperv1.ListItemsResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid ListItems request: request is required")
	}
	items, nextToken, err := h.engine.List(ctx, userID, req.GetIncludeDeleted(), int32(req.GetPageSize()), req.GetPageToken())
	if err != nil {
		return nil, toVaultStatusError(err)
	}
	resp := &gophkeeperv1.ListItemsResponse{
		Items:         make([]*gophkeeperv1.ItemSummary, 0, len(items)),
		NextPageToken: nextToken,
	}
	for _, it := range items {
		if it == nil {
			log := h.logger
			if log == nil {
				log = zap.NewNop()
			}
			log.Error("vault: list returned nil item", zap.String("user_id", userID))
			return nil, status.Error(codes.Internal, "vault: internal error")
		}
		resp.Items = append(resp.Items, toProtoSummary(it))
	}
	return resp, nil
}

func (h vaultHandler) UpdateItem(ctx context.Context, req *gophkeeperv1.UpdateItemRequest) (*gophkeeperv1.UpdateItemResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid UpdateItem request: request is required")
	}
	if req.GetTitle() == nil || req.GetMetadata() == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid UpdateItem request: title and metadata are required")
	}
	if strings.TrimSpace(req.GetBlobId()) == "" && req.GetPayload() == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid UpdateItem request: payload or blob_id is required")
	}
	if req.GetItemId() == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid UpdateItem request: item_id is required")
	}

	pc, pn, bid, sum, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, h.now, userID, req.GetPayload(), req.GetBlobId())
	if err != nil {
		return nil, err
	}

	out, err := h.engine.Update(ctx, userID, req.GetOperationId(), req.GetItemId(), req.GetExpectedVersion(), vault.UpdateInput{
		TitleCiphertext:    req.GetTitle().GetCiphertext(),
		TitleNonce:         req.GetTitle().GetNonce(),
		MetadataCiphertext: req.GetMetadata().GetCiphertext(),
		MetadataNonce:      req.GetMetadata().GetNonce(),
		PayloadCiphertext:  pc,
		PayloadNonce:       pn,
		BlobID:             bid,
		Checksum:           sum,
	})
	if err != nil {
		return nil, toVaultStatusError(err)
	}
	return &gophkeeperv1.UpdateItemResponse{
		NewVersion:     out.NewVersion,
		ServerRevision: out.ServerRevision,
	}, nil
}

func (h vaultHandler) DeleteItem(ctx context.Context, req *gophkeeperv1.DeleteItemRequest) (*gophkeeperv1.DeleteItemResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid DeleteItem request: request is required")
	}
	if req.GetItemId() == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid DeleteItem request: item_id is required")
	}
	if req.GetOperationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid DeleteItem request: operation_id is required")
	}
	out, err := h.engine.Delete(ctx, userID, req.GetOperationId(), req.GetItemId(), req.GetExpectedVersion())
	if err != nil {
		return nil, toVaultStatusError(err)
	}
	return &gophkeeperv1.DeleteItemResponse{
		NewVersion:     out.NewVersion,
		ServerRevision: out.ServerRevision,
	}, nil
}

func toVaultStatusError(err error) error {
	switch {
	case err == nil:
		return nil
	case err == vault.ErrInvalidArgument:
		return status.Error(codes.InvalidArgument, err.Error())
	case err == vault.ErrNotFound:
		return status.Error(codes.NotFound, err.Error())
	case err == vault.ErrConflict:
		return status.Error(codes.Aborted, err.Error())
	default:
		return status.Error(codes.Internal, "vault: internal error")
	}
}

func toProtoItem(it *vault.Item) *gophkeeperv1.VaultItem {
	if it == nil {
		return nil
	}
	res := &gophkeeperv1.VaultItem{
		ItemId:   it.ItemID,
		UserId:   it.UserID,
		ItemType: gophkeeperv1.ItemType(it.ItemType),
		Title: &gophkeeperv1.EncryptedField{
			Ciphertext: it.TitleCiphertext,
			Nonce:      it.TitleNonce,
		},
		Metadata: &gophkeeperv1.EncryptedField{
			Ciphertext: it.MetadataCiphertext,
			Nonce:      it.MetadataNonce,
		},
		Payload:   vaultPayloadToProto(it),
		Version:   it.Version,
		Checksum:  it.Checksum,
		CreatedAt: timestamppb.New(it.CreatedAt),
		UpdatedAt: timestamppb.New(it.UpdatedAt),
	}
	if it.BlobID != nil {
		res.BlobId = *it.BlobID
	}
	if it.DeletedAt != nil {
		res.DeletedAt = timestamppb.New(*it.DeletedAt)
	}
	return res
}

func toProtoSummary(it *vault.Item) *gophkeeperv1.ItemSummary {
	if it == nil {
		return nil
	}
	summary := &gophkeeperv1.ItemSummary{
		ItemId:   it.ItemID,
		ItemType: gophkeeperv1.ItemType(it.ItemType),
		Title: &gophkeeperv1.EncryptedField{
			Ciphertext: it.TitleCiphertext,
			Nonce:      it.TitleNonce,
		},
		Metadata: &gophkeeperv1.EncryptedField{
			Ciphertext: it.MetadataCiphertext,
			Nonce:      it.MetadataNonce,
		},
		Version:   it.Version,
		UpdatedAt: timestamppb.New(it.UpdatedAt),
	}
	if it.BlobID != nil {
		summary.BlobId = *it.BlobID
	}
	if it.DeletedAt != nil {
		summary.DeletedAt = timestamppb.New(*it.DeletedAt)
	}
	return summary
}
