package grpcserver

import (
	"context"
	"errors"
	"testing"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/auth"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeDeviceAdmin struct {
	list    []auth.DeviceRecord
	listErr error
	revoke  error
}

func (f *fakeDeviceAdmin) ListDevicesByUser(context.Context, string) ([]auth.DeviceRecord, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.list, nil
}

func (f *fakeDeviceAdmin) RevokeDeviceForUser(context.Context, string, string) error {
	return f.revoke
}

var _ auth.DeviceAdmin = (*fakeDeviceAdmin)(nil)

func testDeviceHandler(devices auth.DeviceAdmin) deviceHandler {
	return deviceHandler{logger: zap.NewNop(), devices: devices}
}

func TestDeviceHandler_ListDevices_unauthenticated(t *testing.T) {
	t.Parallel()
	h := testDeviceHandler(&fakeDeviceAdmin{})
	_, err := h.ListDevices(context.Background(), &gophkeeperv1.ListDevicesRequest{})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		t.Fatalf("ListDevices: %v, want Unauthenticated", err)
	}
}

func TestDeviceHandler_ListDevices_internal(t *testing.T) {
	t.Parallel()
	h := testDeviceHandler(&fakeDeviceAdmin{listErr: errors.New("db connection refused")})
	ctx := WithAuthContext(context.Background(), "u1", "d0")
	_, err := h.ListDevices(ctx, &gophkeeperv1.ListDevicesRequest{})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Fatalf("ListDevices: %v, want Internal", err)
	}
	if st.Message() != "devices: internal error" {
		t.Fatalf("message: %q", st.Message())
	}
}

func TestDeviceHandler_ListDevices_success(t *testing.T) {
	t.Parallel()
	created := time.Unix(100, 0).UTC()
	lastSeen := time.Unix(200, 0).UTC()
	h := testDeviceHandler(&fakeDeviceAdmin{
		list: []auth.DeviceRecord{
			{
				DeviceID: "d1", UserID: "u1", DeviceName: "phone", Platform: 1, ClientVersion: "1",
				CreatedAt: created, LastSeenAt: lastSeen,
			},
		},
	})
	ctx := WithAuthContext(context.Background(), "u1", "d0")
	resp, err := h.ListDevices(ctx, &gophkeeperv1.ListDevicesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetDevices()) != 1 {
		t.Fatalf("devices len: %d", len(resp.GetDevices()))
	}
	d := resp.GetDevices()[0]
	if d.GetDeviceId() != "d1" || d.GetUserId() != "u1" || d.GetDeviceName() != "phone" {
		t.Fatalf("device: %+v", d)
	}
	if d.GetPlatform() != gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_LINUX {
		t.Fatalf("platform: %v", d.GetPlatform())
	}
	if d.GetRevokedAt() != nil {
		t.Fatal("expected no revoked_at")
	}
}

func TestDeviceHandler_ListDevices_revokedAt(t *testing.T) {
	t.Parallel()
	created := time.Unix(100, 0).UTC()
	lastSeen := time.Unix(200, 0).UTC()
	revoked := time.Unix(300, 0).UTC()
	h := testDeviceHandler(&fakeDeviceAdmin{
		list: []auth.DeviceRecord{
			{
				DeviceID: "d1", UserID: "u1", DeviceName: "phone", Platform: 1, ClientVersion: "1",
				CreatedAt: created, LastSeenAt: lastSeen, RevokedAt: &revoked,
			},
		},
	})
	ctx := WithAuthContext(context.Background(), "u1", "d0")
	resp, err := h.ListDevices(ctx, &gophkeeperv1.ListDevicesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	d := resp.GetDevices()[0]
	ra := d.GetRevokedAt()
	if ra == nil {
		t.Fatal("expected revoked_at")
	}
	if !ra.AsTime().Equal(revoked) {
		t.Fatalf("revoked_at: got %v want %v", ra.AsTime(), revoked)
	}
}

func TestDeviceHandler_RevokeDevice_requiresDeviceID(t *testing.T) {
	t.Parallel()
	h := testDeviceHandler(&fakeDeviceAdmin{})
	ctx := WithAuthContext(context.Background(), "u1", "d0")
	_, err := h.RevokeDevice(ctx, &gophkeeperv1.RevokeDeviceRequest{})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("RevokeDevice: %v, want InvalidArgument", err)
	}
}

func TestDeviceHandler_RevokeDevice_whitespaceDeviceID(t *testing.T) {
	t.Parallel()
	h := testDeviceHandler(&fakeDeviceAdmin{})
	ctx := WithAuthContext(context.Background(), "u1", "d0")
	_, err := h.RevokeDevice(ctx, &gophkeeperv1.RevokeDeviceRequest{DeviceId: "  \t  "})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("RevokeDevice: %v, want InvalidArgument", err)
	}
}

func TestDeviceHandler_RevokeDevice_notFound(t *testing.T) {
	t.Parallel()
	h := testDeviceHandler(&fakeDeviceAdmin{revoke: auth.ErrDeviceNotFound})
	ctx := WithAuthContext(context.Background(), "u1", "d0")
	_, err := h.RevokeDevice(ctx, &gophkeeperv1.RevokeDeviceRequest{DeviceId: "missing"})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Fatalf("RevokeDevice: %v, want NotFound", err)
	}
}

func TestDeviceHandler_RevokeDevice_internal(t *testing.T) {
	t.Parallel()
	h := testDeviceHandler(&fakeDeviceAdmin{revoke: errors.New("db connection refused")})
	ctx := WithAuthContext(context.Background(), "u1", "d0")
	_, err := h.RevokeDevice(ctx, &gophkeeperv1.RevokeDeviceRequest{DeviceId: "d1"})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Fatalf("RevokeDevice: %v, want Internal", err)
	}
	if st.Message() != "devices: internal error" {
		t.Fatalf("message: %q", st.Message())
	}
}

func TestDeviceHandler_RevokeDevice_success(t *testing.T) {
	t.Parallel()
	h := testDeviceHandler(&fakeDeviceAdmin{})
	ctx := WithAuthContext(context.Background(), "u1", "d0")
	resp, err := h.RevokeDevice(ctx, &gophkeeperv1.RevokeDeviceRequest{DeviceId: "d1"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetSuccess() {
		t.Fatal("expected success")
	}
}
