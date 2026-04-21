package grpcserver

import (
	"testing"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
)

func TestConcreteProtoItemType(t *testing.T) {
	t.Parallel()
	_, ok := concreteProtoItemType(gophkeeperv1.ItemType_ITEM_TYPE_UNSPECIFIED)
	if ok {
		t.Fatal("expected unspecified to fail")
	}
	v, ok := concreteProtoItemType(gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL)
	if !ok || v != int16(gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL) {
		t.Fatalf("credential: v=%v ok=%v", v, ok)
	}
	v, ok = concreteProtoItemType(gophkeeperv1.ItemType_ITEM_TYPE_BINARY)
	if !ok || v != int16(gophkeeperv1.ItemType_ITEM_TYPE_BINARY) {
		t.Fatalf("binary: v=%v ok=%v", v, ok)
	}
}
