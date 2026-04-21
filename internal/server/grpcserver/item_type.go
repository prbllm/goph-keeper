package grpcserver

import gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"

// concreteProtoItemType maps a protobuf ItemType to the int16 stored for vault_items.item_type.
// Unspecified and unknown values return ok == false.
func concreteProtoItemType(t gophkeeperv1.ItemType) (v int16, ok bool) {
	switch t {
	case gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
		gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
		gophkeeperv1.ItemType_ITEM_TYPE_CARD,
		gophkeeperv1.ItemType_ITEM_TYPE_OTP,
		gophkeeperv1.ItemType_ITEM_TYPE_BINARY:
		return int16(t), true
	default:
		return 0, false
	}
}
