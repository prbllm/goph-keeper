package grpcserver

import (
	"context"
	"errors"
	"strings"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/auth"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type deviceHandler struct {
	gophkeeperv1.UnimplementedDeviceServiceServer
	logger  *zap.Logger
	devices auth.DeviceAdmin
}

func (h deviceHandler) ListDevices(ctx context.Context, _ *gophkeeperv1.ListDevicesRequest) (*gophkeeperv1.ListDevicesResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}
	rows, err := h.devices.ListDevicesByUser(ctx, userID)
	if err != nil {
		h.logger.Error("devices: list failed", zap.String("user_id", userID), zap.Error(err))
		return nil, status.Error(codes.Internal, "devices: internal error")
	}
	out := make([]*gophkeeperv1.Device, 0, len(rows))
	for i := range rows {
		out = append(out, deviceRecordToProto(&rows[i]))
	}
	return &gophkeeperv1.ListDevicesResponse{Devices: out}, nil
}

func (h deviceHandler) RevokeDevice(ctx context.Context, req *gophkeeperv1.RevokeDeviceRequest) (*gophkeeperv1.RevokeDeviceResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}
	if req == nil || strings.TrimSpace(req.GetDeviceId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "device_id is required")
	}
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if err := h.devices.RevokeDeviceForUser(ctx, userID, deviceID); err != nil {
		if errors.Is(err, auth.ErrDeviceNotFound) {
			return nil, status.Error(codes.NotFound, "device not found")
		}
		h.logger.Error("devices: revoke failed",
			zap.String("user_id", userID),
			zap.String("device_id", deviceID),
			zap.Error(err))
		return nil, status.Error(codes.Internal, "devices: internal error")
	}
	return &gophkeeperv1.RevokeDeviceResponse{Success: true}, nil
}

func deviceRecordToProto(r *auth.DeviceRecord) *gophkeeperv1.Device {
	if r == nil {
		return nil
	}
	d := &gophkeeperv1.Device{
		DeviceId:      r.DeviceID,
		UserId:        r.UserID,
		DeviceName:    r.DeviceName,
		Platform:      protoDevicePlatform(r.Platform),
		ClientVersion: r.ClientVersion,
		CreatedAt:     timestamppb.New(r.CreatedAt),
		LastSeenAt:    timestamppb.New(r.LastSeenAt),
	}
	if r.RevokedAt != nil {
		d.RevokedAt = timestamppb.New(*r.RevokedAt)
	}
	return d
}
