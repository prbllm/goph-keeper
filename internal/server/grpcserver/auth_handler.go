package grpcserver

import (
	"context"
	"errors"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type authHandler struct {
	gophkeeperv1.UnimplementedAuthServiceServer
	svc *auth.Service
}

func (h authHandler) Register(ctx context.Context, req *gophkeeperv1.RegisterRequest) (*gophkeeperv1.RegisterResponse, error) {
	out, err := h.svc.Register(ctx, auth.RegisterInput{
		Login:                  req.GetLogin(),
		Password:               req.GetPassword(),
		PasswordSalt:           req.GetPasswordSalt(),
		KDFAlgorithm:           req.GetKdfParams().GetAlgorithm(),
		KDFMemoryKiB:           int32(req.GetKdfParams().GetMemoryKib()),
		KDFIterations:          int32(req.GetKdfParams().GetIterations()),
		KDFParallelism:         int32(req.GetKdfParams().GetParallelism()),
		KDFKeyLength:           int32(req.GetKdfParams().GetKeyLength()),
		EncryptedVaultKey:      req.GetEncryptedVaultKey(),
		EncryptedVaultKeyNonce: req.GetEncryptedVaultKeyNonce(),
		DeviceID:               req.GetDevice().GetDeviceId(),
		DeviceName:             req.GetDevice().GetDeviceName(),
		Platform:               int16(req.GetDevice().GetPlatform()),
		ClientVersion:          req.GetDevice().GetClientVersion(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &gophkeeperv1.RegisterResponse{
		User: &gophkeeperv1.UserInfo{
			UserId: out.UserID,
			Login:  out.Login,
		},
		DeviceId:     out.DeviceID,
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		Limits:       h.svc.Limits(),
	}, nil
}

func (h authHandler) Login(ctx context.Context, req *gophkeeperv1.LoginRequest) (*gophkeeperv1.LoginResponse, error) {
	out, err := h.svc.Login(ctx, auth.LoginInput{
		Login:         req.GetLogin(),
		Password:      req.GetPassword(),
		DeviceID:      req.GetDevice().GetDeviceId(),
		DeviceName:    req.GetDevice().GetDeviceName(),
		Platform:      int16(req.GetDevice().GetPlatform()),
		ClientVersion: req.GetDevice().GetClientVersion(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &gophkeeperv1.LoginResponse{
		User: &gophkeeperv1.UserInfo{
			UserId: out.UserID,
			Login:  out.Login,
		},
		DeviceId:     out.DeviceID,
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		PasswordSalt: out.PasswordSalt,
		KdfParams: &gophkeeperv1.KdfParams{
			Algorithm:   out.KDFAlgorithm,
			MemoryKib:   uint32(out.KDFMemoryKiB),
			Iterations:  uint32(out.KDFIterations),
			Parallelism: uint32(out.KDFParallelism),
			KeyLength:   uint32(out.KDFKeyLength),
		},
		EncryptedVaultKey:      out.EncryptedVaultKey,
		EncryptedVaultKeyNonce: out.EncryptedVaultKeyNonce,
		Limits:                 h.svc.Limits(),
	}, nil
}

func (h authHandler) Refresh(ctx context.Context, req *gophkeeperv1.RefreshRequest) (*gophkeeperv1.RefreshResponse, error) {
	out, err := h.svc.Refresh(ctx, auth.RefreshInput{
		RefreshToken: req.GetRefreshToken(),
		DeviceID:     req.GetDeviceId(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &gophkeeperv1.RefreshResponse{
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
	}, nil
}

func (h authHandler) Logout(ctx context.Context, req *gophkeeperv1.LogoutRequest) (*gophkeeperv1.LogoutResponse, error) {
	if err := h.svc.Logout(ctx, req.GetRefreshToken()); err != nil {
		return nil, toStatusError(err)
	}
	return &gophkeeperv1.LogoutResponse{Success: true}, nil
}

func toStatusError(err error) error {
	switch {
	case errors.Is(err, auth.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, auth.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, auth.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
