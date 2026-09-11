#!/usr/bin/env bash
# Aprovisiona un nodo worker: ffmpeg + binario + servicio systemd. Idempotente.
# Lo ejecuta Vagrant como root con WORKER_ID y WORKER_ROLE en el entorno.
set -euo pipefail

COORDINATOR_HOST="${COORDINATOR_HOST:-192.168.56.1}"   # el host de VirtualBox

echo "→ instalando ffmpeg"
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ffmpeg > /dev/null

echo "→ instalando el worker"
install -d /etc/mediacase /var/lib/mediacase
install -m 0755 /vagrant/bin/worker-linux-amd64 /usr/local/bin/mediacase-worker

cat > /etc/mediacase/worker.env <<ENV
WORKER_ID=${WORKER_ID}
WORKER_ROLE=${WORKER_ROLE}
WORKER_POOL_SIZE=2
COORDINATOR_URL=http://${COORDINATOR_HOST}:8080
MINIO_ENDPOINT=${COORDINATOR_HOST}:9000
MINIO_PUBLIC_ENDPOINT=${COORDINATOR_HOST}:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=results
ENV

# Restart=always: si el worker muere o el coordinador no esta, systemd lo relanza cada 3 s.
cat > /etc/systemd/system/mediacase-worker.service <<'UNIT'
[Unit]
Description=MediaCase worker
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/mediacase/worker.env
ExecStart=/usr/local/bin/mediacase-worker
Restart=always
RestartSec=3
WorkingDirectory=/var/lib/mediacase

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now mediacase-worker
sleep 2
systemctl --no-pager --lines=5 status mediacase-worker || true
echo "provision OK: ${WORKER_ID} (${WORKER_ROLE}) → ${COORDINATOR_HOST}"
