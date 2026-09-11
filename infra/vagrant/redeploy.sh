#!/usr/bin/env bash
# Copia el binario recien compilado a los nodos y reinicia el servicio, sin re-aprovisionar.
# Uso (desde infra/vagrant, en Git Bash):  bash redeploy.sh [node2|node3|all]
set -euo pipefail
target="${1:-all}"
nodes=(node2 node3)
[[ "$target" != "all" ]] && nodes=("$target")
for n in "${nodes[@]}"; do
  echo "→ $n"
  vagrant ssh "$n" -c "sudo install -m 0755 /vagrant/bin/worker-linux-amd64 /usr/local/bin/mediacase-worker && sudo systemctl restart mediacase-worker && sleep 1 && systemctl is-active mediacase-worker"
done
