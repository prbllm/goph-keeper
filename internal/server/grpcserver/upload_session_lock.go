package grpcserver

import "sync"

// uploadSessionLocks serializes BlobService.UploadBlob per (userID, uploadSessionID).
// One process instance only; multi-instance deployments need a distributed lock or DB-side claim.
type uploadSessionLocks struct {
	m sync.Map // string -> *sync.Mutex
}

func uploadLockKey(userID, sessionID string) string {
	return userID + "\x00" + sessionID
}

// acquire blocks until the per-session lock is held. The returned function releases it.
func (l *uploadSessionLocks) acquire(key string) (release func()) {
	v, _ := l.m.LoadOrStore(key, new(sync.Mutex))
	mu := v.(*sync.Mutex)
	mu.Lock()
	return func() { mu.Unlock() }
}
