#!/usr/bin/env bash
#
# deploy-pi.sh — Install & jalankan Sistem Monitoring COD di Raspberry Pi (64-bit).
# Cara pakai:  bash deploy-pi.sh
# DIJALANKAN DI RASPBERRY PI (bukan di laptop). Bisa dijalankan berulang (idempotent).
#
set -euo pipefail

APP_DIR="$HOME/cod-monitor"
REPO_URL="https://github.com/abidahh/sistem-monitoring.git"
SERVICE_NAME="cod-monitor"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
ENV_FILE="$HOME/cod-monitor.env"
PI_USER="$(whoami)"

info() { echo "[INFO]  $*"; }
ok()   { echo "[OK]    $*"; }
warn() { echo "[WARN]  $*"; }
die()  { echo "[ERROR] $*"; exit 1; }

# ---------------------------------------------------------------------------
# 0. Cek arsitektur (wajib 64-bit)
# ---------------------------------------------------------------------------
ARCH="$(uname -m)"
if [ "$ARCH" != "aarch64" ]; then
  die "Arsitektur '$ARCH' bukan aarch64. Wajib install Raspberry Pi OS (64-bit) dulu."
fi
ok "Arsitektur: $ARCH"

# ---------------------------------------------------------------------------
# 1. Update sistem + install git & curl
# ---------------------------------------------------------------------------
info "Install git & curl (butuh internet) ..."
sudo apt-get update -y
sudo apt-get install -y git curl

# ---------------------------------------------------------------------------
# 2. Install Go >= 1.27 (tarball resmi; jangan pakai 'apt install golang' yang lama)
# ---------------------------------------------------------------------------
if command -v go >/dev/null 2>&1; then
  MAJOR="$(go version | awk '{print $3}' | cut -c3- | cut -d. -f1)"
  MINOR="$(go version | awk '{print $3}' | cut -c3- | cut -d. -f2)"
  if [ "$MAJOR" -gt 1 ] || { [ "$MAJOR" -eq 1 ] && [ "$MINOR" -ge 27 ]; }; then
    ok "Go sudah terpasang: $(go version)"
  else
    warn "Go terpasang terlalu lama ($(go version)); pasang ulang Go 1.27.1..."
    wget -q https://go.dev/dl/go1.27.1.linux-arm64.tar.gz
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf go1.27.1.linux-arm64.tar.gz
    rm -f go1.27.1.linux-arm64.tar.gz
    export PATH="$PATH:/usr/local/go/bin"
  fi
elif [ -x /usr/local/go/bin/go ]; then
  ok "Go sudah ada di /usr/local/go: $(/usr/local/go/bin/go version)"
  export PATH="$PATH:/usr/local/go/bin"
else
  info "Unduh & pasang Go 1.27.1 ..."
  wget -q https://go.dev/dl/go1.27.1.linux-arm64.tar.gz
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf go1.27.1.linux-arm64.tar.gz
  rm -f go1.27.1.linux-arm64.tar.gz
  export PATH="$PATH:/usr/local/go/bin"
fi
if ! grep -q '/usr/local/go/bin' "$HOME/.bashrc" 2>/dev/null; then
  echo 'export PATH="$PATH:/usr/local/go/bin"' >> "$HOME/.bashrc"
fi
go version

# ---------------------------------------------------------------------------
# 3. Clone / update repo
# ---------------------------------------------------------------------------
if [ ! -d "$APP_DIR/.git" ]; then
  info "Clone repo (Username=abidahh, Password=PAT token GitHub) ..."
  git config --global credential.helper store
  git clone "$REPO_URL" "$APP_DIR"
else
  info "Update repo (git pull) ..."
  git -C "$APP_DIR" pull
fi
cd "$APP_DIR"

# ---------------------------------------------------------------------------
# 4. Build binary
# ---------------------------------------------------------------------------
info "Build cod-server ..."
go build -o cod-server .

# ---------------------------------------------------------------------------
# 5. Buat file env bila belum ada (terletak di luar repo: ~/cod-monitor.env)
# ---------------------------------------------------------------------------
if [ ! -f "$ENV_FILE" ]; then
  RANDOM_SECRET="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  umask 077
  cat > "$ENV_FILE" <<EOF
SERVER_PORT=:8080
SESSION_SECRET=$RANDOM_SECRET
ADMIN_USERNAME=admin
ADMIN_PASSWORD=GANTI-PASSWORD-ADMIN-AMAN
DEFAULT_API_KEY=ganti-api-key-random
EOF
  warn "File $ENV_FILE baru saja dibuat."
  echo ""
  echo "=============================================================================="
  echo " LANJUTKAN DI SINI:"
  echo " 1) EDIT file konfigurasi:     nano $ENV_FILE"
  echo " 2) GANTI nilai ADMIN_PASSWORD dan DEFAULT_API_KEY dengan milik Anda."
  echo " 3) JALANKAN ULANG skrip:      bash deploy-pi.sh"
  echo "=============================================================================="
  echo ""
  exit 0
fi
ok "Env file sudah ada: $ENV_FILE"

# ---------------------------------------------------------------------------
# 6. Izin akses USB/serial
# ---------------------------------------------------------------------------
info "Tambah user '$PI_USER' ke grup 'dialout' ..."
sudo usermod -aG dialout "$PI_USER"

# ---------------------------------------------------------------------------
# 7. Pasang systemd service (auto-start saat Pi reboot)
# ---------------------------------------------------------------------------
info "Pasang systemd service '$SERVICE_NAME' ..."
sudo tee "$SERVICE_FILE" > /dev/null <<EOF
[Unit]
Description=Sistem Monitoring COD (Go + Modbus RTU)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$PI_USER
Group=dialout
WorkingDirectory=$APP_DIR
EnvironmentFile=$ENV_FILE
ExecStart=$APP_DIR/cod-server
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now "$SERVICE_NAME"

# ---------------------------------------------------------------------------
# 8. Status & info akses
# ---------------------------------------------------------------------------
sleep 2
echo ""
ok "Status service:"
systemctl --no-pager --pretty=off status "$SERVICE_NAME" || true

IP="$(hostname -I | awk '{print $1}')"
echo ""
ok "Akses dari HP/laptop (satu wifi):  http://$IP:8080"
ok "Akses dari Pi sendiri:             http://localhost:8080"
echo ""
warn "Lihat log:   journalctl -u cod-monitor -f"
warn "Grup dialout aktif setelah logout/login (atau reboot Pi)."
warn "Update versi baru nanti: cd ~/cod-monitor && git pull && go build -o cod-server . && sudo systemctl restart cod-monitor"
echo "Selesai."