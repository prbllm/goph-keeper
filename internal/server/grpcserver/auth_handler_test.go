package grpcserver

import (
	"context"
	"errors"
	"testing"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAuthHandlerSvc struct {
	regErr    error
	loginErr  error
	refreshErr error
	logoutErr error
	limits    *gophkeeperv1.Limits
}

func (f fakeAuthHandlerSvc) Register(context.Context, auth.RegisterInput) (*auth.RegisterOutput, error) {
	if f.regErr != nil {
		return nil, f.regErr
	}
	return &auth.RegisterOutput{
		UserID: "u1", Login: "log", DeviceID: "d1",
		AccessToken: "a", RefreshToken: "r",
	}, nil
}

func (f fakeAuthHandlerSvc) Login(context.Context, auth.LoginInput) (*auth.LoginOutput, error) {
	if f.loginErr != nil {
		return nil, f.loginErr
	}
	return &auth.LoginOutput{
		UserID: "u1", Login: "log", DeviceID: "d1",
		AccessToken: "a", RefreshToken: "r",
	}, nil
}

func (f fakeAuthHandlerSvc) Refresh(context.Context, auth.RefreshInput) (*auth.RefreshOutput, error) {
	if f.refreshErr != nil {
		return nil, f.refreshErr
	}
	return &auth.RefreshOutput{AccessToken: "a2", RefreshToken: "r2"}, nil
}

func (f fakeAuthHandlerSvc) Logout(context.Context, string) error {
	return f.logoutErr
}

func (f fakeAuthHandlerSvc) Limits() *gophkeeperv1.Limits {
	if f.limits != nil {
		return f.limits
	}
	return &gophkeeperv1.Limits{}
}

func TestAuthHandler_Register_mapsDomainErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	req := &gophkeeperv1.RegisterRequest{
		Login: "x", Password: "y",
		Device: &gophkeeperv1.DeviceInfo{DeviceId: "d", DeviceName: "n", Platform: 1, ClientVersion: "1"},
	}

	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{"invalid_argument", auth.ErrInvalidArgument, codes.InvalidArgument},
		{"already_exists", auth.ErrAlreadyExists, codes.AlreadyExists},
		{"unauthorized", auth.ErrUnauthorized, codes.Unauthenticated},
		{"device_not_found_maps_internal", auth.ErrDeviceNotFound, codes.Internal},
		{"unknown_maps_internal", errors.New("boom"), codes.Internal},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := authHandler{svc: fakeAuthHandlerSvc{regErr: tc.err}}
			_, err := h.Register(ctx, req)
			if err == nil {
				t.Fatal("expected error")
			}
			st, ok := status.FromError(err)
			if !ok || st.Code() != tc.code {
				t.Fatalf("got %v code=%v want %v", err, st.Code(), tc.code)
			}
		})
	}
}

func TestAuthHandler_Login_mapsUnauthorized(t *testing.T) {
	t.Parallel()
	h := authHandler{svc: fakeAuthHandlerSvc{loginErr: auth.ErrUnauthorized}}
	_, err := h.Login(context.Background(), &gophkeeperv1.LoginRequest{
		Login: "a", Password: "b",
		Device: &gophkeeperv1.DeviceInfo{DeviceId: "d"},
	})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		t.Fatalf("got %v", err)
	}
}

func TestAuthHandler_Refresh_mapsInvalidArgument(t *testing.T) {
	t.Parallel()
	h := authHandler{svc: fakeAuthHandlerSvc{refreshErr: auth.ErrInvalidArgument}}
	_, err := h.Refresh(context.Background(), &gophkeeperv1.RefreshRequest{})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
}

func TestAuthHandler_Logout_mapsAlreadyExists(t *testing.T) {
	t.Parallel()
	h := authHandler{svc: fakeAuthHandlerSvc{logoutErr: auth.ErrAlreadyExists}}
	_, err := h.Logout(context.Background(), &gophkeeperv1.LogoutRequest{})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.AlreadyExists {
		t.Fatalf("got %v", err)
	}
}
