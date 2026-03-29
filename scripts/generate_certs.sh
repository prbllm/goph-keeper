#!/usr/bin/env bash

set -euo pipefail

command -v openssl >/dev/null 2>&1 || {
  echo "openssl is required"
  exit 1
}

CERTS_DIRECTORY="certs"
COUNTRY="RU"
STATE="Moscow"
LOCALITY="Moscow"
ORGANIZATION="GophKeeperIndustry"
CA_COMMON_NAME="GophKeeperIndustryCA"
SERVER_COMMON_NAME="localhost"

CA_VALIDITY_DAYS=3650
SERVER_VALIDITY_DAYS=365

mkdir -p "${CERTS_DIRECTORY}"
cd "${CERTS_DIRECTORY}"

echo "==> Generating CA private key"
openssl genrsa -out ca.key 4096

echo "==> Generating CA certificate"
openssl req -x509 -new -nodes \
  -key ca.key \
  -sha256 \
  -days "${CA_VALIDITY_DAYS}" \
  -out ca.crt \
  -subj "/C=${COUNTRY}/ST=${STATE}/L=${LOCALITY}/O=${ORGANIZATION}/CN=${CA_COMMON_NAME}"

echo "==> Generating server private key"
openssl genrsa -out server.key 2048

echo "==> Generating server CSR"
openssl req -new \
  -key server.key \
  -out server.csr \
  -subj "/C=${COUNTRY}/ST=${STATE}/L=${LOCALITY}/O=${ORGANIZATION}/CN=${SERVER_COMMON_NAME}"

echo "==> Writing SAN config"
cat > san.cnf <<EOF
subjectAltName=DNS:localhost,IP:127.0.0.1
EOF

echo "==> Signing server certificate with CA"
openssl x509 -req \
  -in server.csr \
  -CA ca.crt \
  -CAkey ca.key \
  -CAcreateserial \
  -out server.crt \
  -days "${SERVER_VALIDITY_DAYS}" \
  -sha256 \
  -extfile san.cnf

echo "==> Removing temporary files"
rm -f server.csr san.cnf ca.srl

echo "==> Done"
echo
echo "Public files (can be stored in git):"
echo "  certs/ca.crt"
echo "  certs/server.crt"
echo
echo "Private files (must NOT be stored in git):"
echo "  certs/ca.key"
echo "  certs/server.key"