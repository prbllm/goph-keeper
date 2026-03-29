package grpcserver

import (
	"testing"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
)

func TestProtoDevicePlatform(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   int16
		want gophkeeperv1.DevicePlatform
	}{
		{1, gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_LINUX},
		{2, gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_WINDOWS},
		{3, gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_MACOS},
		{0, gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_UNSPECIFIED},
		{99, gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_UNSPECIFIED},
	}
	for _, tt := range tests {
		if got := protoDevicePlatform(tt.in); got != tt.want {
			t.Fatalf("protoDevicePlatform(%d) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
