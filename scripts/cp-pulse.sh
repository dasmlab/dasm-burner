#!/usr/bin/env bash
# Control-plane pulse: write one CSV line every 15s. 5s oc timeout.
# Run in a terminal you can see. Ctrl-C to stop. Chat agent is not required.
# Usage: ./scripts/cp-pulse.sh [logfile]
set -u
LOG="${1:-./cp-pulse.csv}"
OC_TO="${OC_REQUEST_TIMEOUT:-5s}"
echo "time,masters_ready,masters_total,master_mem_csv,etcd_ready,kas_ready,oc_err" | tee -a "$LOG"
echo "writing $LOG every 15s (timeout $OC_TO). Ctrl-C to stop." >&2

pulse() {
  local err="" nodes etcd kas mem
  nodes="$(oc --request-timeout="$OC_TO" get nodes -l node-role.kubernetes.io/master --no-headers 2>/tmp/cp-pulse.err || true)"
  if grep -q . /tmp/cp-pulse.err 2>/dev/null; then
    err="$(tr '\n' ' ' </tmp/cp-pulse.err | cut -c1-120)"
  fi
  local total ready
  total="$(printf '%s\n' "$nodes" | grep -c . || true)"
  ready="$(printf '%s\n' "$nodes" | grep -c ' Ready' || true)"
  mem="$(oc --request-timeout="$OC_TO" adm top nodes 2>/dev/null | awk '/master/ {printf "%s:%s ", $1,$5}' || true)"
  etcd="$(oc --request-timeout="$OC_TO" get pods -n openshift-etcd --no-headers 2>/dev/null | awk '/^etcd-/ && $0 !~ /guard/ && $2 ~ /5\/5|4\/4/ {r++} END{print r+0}' || echo 0)"
  kas="$(oc --request-timeout="$OC_TO" get pods -n openshift-kube-apiserver --no-headers 2>/dev/null | awk '/^kube-apiserver-/ && $0 !~ /guard/ && $2 ~ /5\/5/ {r++} END{print r+0}' || echo 0)"
  printf '%s,%s,%s,%s,%s,%s,%s\n' "$(date -u +%H:%M:%S)" "${ready:-0}" "${total:-0}" "${mem:-}" "${etcd:-0}" "${kas:-0}" "${err:-}"
}

while true; do
  pulse | tee -a "$LOG"
  sleep 15
done
