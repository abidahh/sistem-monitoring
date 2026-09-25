#!/bin/sh
# Membuat sertifikat self-signed untuk HTTPS (akses LAN via browser).
# Hasil: cert/server.crt dan cert/server.key.
# Lalu aktifkan HTTPS di .env:
#   SERVER_CERT=cert/server.crt
#   SERVER_KEY=cert/server.key
set -e

DIR="$(cd "$(dirname "$0")/.." && pwd)/cert"
mkdir -p "$DIR"

openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$DIR/server.key" \
  -out "$DIR/server.crt" \
  -days 825 \
  -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:0.0.0.0" \
  >/dev/null 2>&1

echo "Selesai! Sertifikat dibuat di:"
echo "  $DIR/server.crt"
echo "  $DIR/server.key"
echo "Perbarui .env lalu jalankan ulang server."