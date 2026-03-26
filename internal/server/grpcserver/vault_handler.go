package grpcserver

import (
	"context"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/vault"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type vaultHandler struct {
	gophkeeperv1.UnimplementedVaultServiceServer
	engine vault.Engine
}

func (h vaultHandler) CreateItem(ctx context.Context, req *gophkeeperv1.CreateItemRequest) (*gophkeeperv1.CreateItemResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid CreateItem request: request is required")
	}
	if req.GetTitle() == nil || req.GetMetadata() == nil || req.GetPayload() == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid CreateItem request: title, metadata and payload are required")
	}

	out, err := h.engine.Create(ctx, userID, req.GetOperationId(), vault.CreateInput{
		ItemType:           int16(req.GetItemType()),
		TitleCiphertext:    req.GetTitle().GetCiphertext(),
		TitleNonce:         req.GetTitle().GetNonce(),
		MetadataCiphertext: req.GetMetadata().GetCiphertext(),
		MetadataNonce:      req.GetMetadata().GetNonce(),
		PayloadCiphertext:  req.GetPayload().GetCiphertext(),
		PayloadNonce:       req.GetPayload().GetNonce(),
		BlobID:             stringPtrOrNil(req.GetBlobId()),
		Checksum:           nil,
	})
	if err != nil {
		return nil, toVaultStatusError(err)
	}
	return &gophkeeperv1.CreateItemResponse{
		ItemId:        out.ItemID,
		Version:       out.Version,
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
	if req.GetTitle() == nil || req.GetMetadata() == nil || req.GetPayload() == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid UpdateItem request: title, metadata and payload are required")
	}
	if req.GetItemId() == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid UpdateItem request: item_id is required")
	}

	out, err := h.engine.Update(ctx, userID, req.GetOperationId(), req.GetItemId(), req.GetExpectedVersion(), vault.UpdateInput{
		TitleCiphertext:    req.GetTitle().GetCiphertext(),
		TitleNonce:         req.GetTitle().GetNonce(),
		MetadataCiphertext: req.GetMetadata().GetCiphertext(),
		MetadataNonce:      req.GetMetadata().GetNonce(),
		PayloadCiphertext:  req.GetPayload().GetCiphertext(),
		PayloadNonce:       req.GetPayload().GetNonce(),
		BlobID:             stringPtrOrNil(req.GetBlobId()),
		Checksum:           nil,
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
		return status.Error(codes.Internal, "internal error")
	}
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toProtoItem(it *vault.Item) *gophkeeperv1.VaultItem {
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
		Payload: &gophkeeperv1.EncryptedField{
			Ciphertext: it.PayloadCiphertext,
			Nonce:      it.PayloadNonce,
		},
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


