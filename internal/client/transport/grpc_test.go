package transport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_Success_Insecure(t *testing.T) {
	// Arrange
	addr := "localhost:50051"
	token := "test-token"
	opts := DialOptions{
		Insecure: true,
	}

	// Act
	client, err := New(addr, token, opts)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.Conn)
	assert.NotNil(t, client.AuthClient())
	assert.NotNil(t, client.VaultClient())
	assert.NotNil(t, client.SyncClient())
	assert.NotNil(t, client.BlobClient())

	// Cleanup
	_ = client.Close()
}

func TestNew_Fail_Secure_InvalidCAPath(t *testing.T) {
	// Arrange
	addr := "localhost:50051"
	token := "test-token"
	opts := DialOptions{
		Insecure: false,
	}

	// Act
	client, err := New(addr, token, opts)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestNew_Fail_Secure_InvalidCA(t *testing.T) {
	// Arrange
	addr := "localhost:50051"
	token := "test-token"
	tempDir := t.TempDir()
	invalidCA := filepath.Join(tempDir, "invalid-ca.pem")
	// Создаём файл с невалидным сертификатом
	err := os.WriteFile(invalidCA, []byte("invalid certificate content"), 0644)
	assert.NoError(t, err)
	opts := DialOptions{
		Insecure: false,
		CAPath:   invalidCA,
	}

	// Act
	client, err := New(addr, token, opts)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "invalid CA cert")
}

func TestNew_Secure_ValidCA(t *testing.T) {
	// Arrange
	addr := "localhost:50051"
	token := "test-token"
	tempDir := t.TempDir()
	caPath := filepath.Join(tempDir, "ca.pem")
	// Создаём файл с валидным сертификатом
	err := os.WriteFile(caPath, []byte(`-----BEGIN CERTIFICATE-----
MIIFHzCCAwegAwIBAgIUbCRytwmwIiUhUGtdLP6YADdFDJUwDQYJKoZIhvcNAQEL
BQAwFDESMBAGA1UEAwwJbG9jYWxob3N0MB4XDTI2MDMyNTEzMTEyNFoXDTI3MDMy
NTEzMTEyNFowFDESMBAGA1UEAwwJbG9jYWxob3N0MIICIjANBgkqhkiG9w0BAQEF
AAOCAg8AMIICCgKCAgEA0TVjbS2r6Ln3tIvQaw/DECWI9Td3+Io7zIcPS06UJvHr
iDZphFhFLkZfNxnJuwILchtT2RgEEd4qoKv/bHfFisIbJKPmvVpTpaRvN1gRd8aj
RJKG8gsHWdExOlcWdr6KlVPeeGC/x7HDJCjoWpUu/Lm5GOV9mq7KwKo8UN/18Ba8
Yq+5ExgPF+eplsPaIlGTvkd4yNVUTwDz3SCyypD+TlZ2WgcGv/acsCTFDdrday5W
XnRRC14zFlGB+ppynRrQVnftRdYzTMFvY0u6NVtgS/ZqwCLzfZDHBUtCAnVi2VZE
PolRNMS9KipgpaVrTcxXhS02YfI/q/iSo9cY5W0YTQI7AFitS7mNfXK4HinFDIZv
yEqGBwikuIUhj4mPSoLOm+Rj42SZ7WSZ66MynX8Yq5FAYtF6UITNcFoWejupBP24
a6c2VdWP0dP0nJZfwkJm15kBFIXQfzXsw941fNsbtZo1IsdZ3PL9FY42UzporJG8
lkcE6ZtgBTPfK/OTYXluhD48PaGVdUL8eop5GvWdBlTqL82ENTyWh1MszchwHx8M
3LLAV06CK3b/Q4D1B3Ahy3WD2mbl9R/ZCq9KkzqhuuaUW0lYhoqIYIrwmVlRRnKT
mFzdxkQLISiQnQcRJ82y4G9Jx6q/tK1gb2VkiFEReOZCjTPoeEuwGYdQoLdTg9sC
AwEAAaNpMGcwHQYDVR0OBBYEFNcAZY7xA/P6UPq5DwL5VjtRjL3bMB8GA1UdIwQY
MBaAFNcAZY7xA/P6UPq5DwL5VjtRjL3bMA8GA1UdEwEB/wQFMAMBAf8wFAYDVR0R
BA0wC4IJbG9jYWxob3N0MA0GCSqGSIb3DQEBCwUAA4ICAQBrJneIOH5FL17Mr1qx
sTHL4G5537T99AGC2sovcip+bqiPqvYtx3uGxpZi1JSXe8gaECVRoZbHkHJs3q2a
AW9j9MMw9RK41X8L9ATXVaXIJwuJ9nkJPMELFd3lriPKwXWAC4dj76ZV90oCH7ij
V7RXBcbOyRKWIC2FUhxeihOyjjoWAcoQM14xrRpIrlRHK7OFmZDodD3xa1upYqjT
euh8GMCvfoIoM824Tn9SZsABVIjxU3/NEjuPIvY0Y3pNrvOrV07PNwd5fPYSdett
5Q3bpHLQ2OKu9/+mOeb5lQLfYmUUyGXnxs3+/AEL6FGNRN6qyMQkyLts9p26lbJX
RsctP3nQuhxaeriEB60pkm4E6EO8nrjQjrXGqKT/ZtrUp5hKkBUZRcJgVR8vQn52
7HJx3Usbcl8vGI4tnoe8ff7UJ/2k+P1XoCRaOAvrVNG1d6BRFuzGWcVuNwVjU2sX
Fr/UvtfZWpave1x713t1MS3fa3E0kqMzG9TgtspaotEjS1pAtHE8ig0ZX2S1Bw75
oZlTUZLWTsLH3R/cZI2HIOf3Li83RMDRKvXblUTIA6VPfjcJEvdi7Msz/byNQoGZ
6nChw4yuvjlYjuuy5/YT784/j1a2JI0sj/q+6kFCqvGNYeKqqFgerppe49yc7Ue0
NVo/UOXaWYfyT4bHVLBz82KhYQ==
-----END CERTIFICATE-----
`), 0644)
	assert.NoError(t, err)
	opts := DialOptions{
		Insecure: false,
		CAPath:   caPath,
	}

	// Act
	client, err := New(addr, token, opts)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.Conn)
	assert.NotNil(t, client.AuthClient())
	assert.NotNil(t, client.VaultClient())
	assert.NotNil(t, client.SyncClient())
	assert.NotNil(t, client.BlobClient())

	// Cleanup
	_ = client.Close()
}
