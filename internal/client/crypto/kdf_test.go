package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeriveKey(t *testing.T) {
	salt, err := GenerateSalt()
	require.NoError(t, err)

	key1 := DeriveKey([]byte("pass"), salt)
	key2 := DeriveKey([]byte("pass"), salt)

	require.Equal(t, key1, key2)
}
