package grpcserver

import "context"

type authContextKey string

const (
	authContextKeyUserID   authContextKey = "auth_user_id"
	authContextKeyDeviceID authContextKey = "auth_device_id"
)

// WithAuthContext stores authenticated subject data in context.
func WithAuthContext(ctx context.Context, userID, deviceID string) context.Context {
	ctx = context.WithValue(ctx, authContextKeyUserID, userID)
	return context.WithValue(ctx, authContextKeyDeviceID, deviceID)
}

// UserIDFromContext extracts authenticated user ID from context.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(authContextKeyUserID).(string)
	return v, ok && v != ""
}

// DeviceIDFromContext extracts authenticated device ID from context.
func DeviceIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(authContextKeyDeviceID).(string)
	return v, ok && v != ""
}
