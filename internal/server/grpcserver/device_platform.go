package grpcserver

import gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"

func protoDevicePlatform(p int16) gophkeeperv1.DevicePlatform {
	switch p {
	case 1:
		return gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_LINUX
	case 2:
		return gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_WINDOWS
	case 3:
		return gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_MACOS
	default:
		return gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_UNSPECIFIED
	}
}
