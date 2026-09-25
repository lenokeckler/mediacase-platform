#!/usr/bin/env bash
set -euo pipefail
SRC="${1:?uso: upload_input.sh <ruta-local> [clave]}"
KEY="${2:-$(basename "$SRC")}"
INFRA="docker compose -f docker-compose.infra.yml"
$INFRA cp "$SRC" "minio:/tmp/$KEY" >/dev/null 2>&1
$INFRA exec -T minio sh -c "mc alias set local http://localhost:9000 minioadmin minioadmin >/dev/null && mc mb --ignore-existing local/dataset >/dev/null && mc cp /tmp/$KEY local/dataset/$KEY >/dev/null && rm /tmp/$KEY"
echo "dataset/$KEY listo"
