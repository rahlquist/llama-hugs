#!/usr/bin/env bash
set -euo pipefail
# Measure one configured model load on its applicable GPU and persist history.
# Usage: backfill-smoke-vram.sh MODEL_ID [DEVICE]
MODEL_ID="${1:?model id required}"
DEVICE="${2:-ROCm0}"
API="${HUGS_API_URL:-http://127.0.0.1:8080}"
CONFIG="${HUGS_CONFIG:-/opt/llama-hugs/config.yaml}"
PORT="${SMOKE_PORT:-18991}"
TMP=$(mktemp -d); PID=""; trap '[[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true; rm -rf "$TMP"' EXIT
# This first version expects a model command supplied explicitly, avoiding unsafe YAML parsing.
CMD="${MODEL_CMD:?set MODEL_CMD to the exact llama-server command with \${PORT}}"
GPU_BEFORE=0; GPU_AFTER=0; GPU_PEAK=0
read_vram() {
  local n=0 f v
  if [[ "$DEVICE" == ROCm* ]]; then
    for f in /sys/class/drm/card*/device/mem_info_vram_used; do [[ -r "$f" ]] && v=$(cat "$f") && ((v>n)) && n=$v; done
  elif command -v nvidia-smi >/dev/null; then
    n=$(nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounit 2>/dev/null | awk 'NR==1{print $1*1048576}')
  fi
  printf '%s' "${n:-0}"
}
GPU_BEFORE=$(read_vram)
# shellcheck disable=SC2086
bash -c "${CMD//\$\{PORT\}/$PORT}" >"$TMP/server.log" 2>&1 & PID=$!
READY=0
for i in {1..180}; do
  x=$(read_vram); ((x>GPU_PEAK)) && GPU_PEAK=$x
  if curl -fsS "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; then READY=1; break; fi
  kill -0 "$PID" 2>/dev/null || break
  sleep 1
done
if (( ! READY )); then
  cat "$TMP/server.log" >&2; exit 1
fi
# Completion confirms the loaded model is actually usable.
curl -fsS --max-time 120 -X POST "http://127.0.0.1:$PORT/completion" -H 'Content-Type: application/json' -d '{"prompt":"Reply OK","n_predict":8,"temperature":0}' >/dev/null
for i in {1..10}; do x=$(read_vram); ((x>GPU_PEAK)) && GPU_PEAK=$x; sleep 1; done
GPU_AFTER=$(read_vram)
kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true; PID=""
python3 - "$API" "$MODEL_ID" "$DEVICE" "$GPU_BEFORE" "$GPU_PEAK" "$GPU_AFTER" <<'PY'
import json,sys,urllib.request,urllib.parse,time
api,model,device,before,peak,after=sys.argv[1:]
base=api.rstrip('/')+'/api/hugs/models/'+urllib.parse.quote(model,safe='')
payload={'model_id':model,'run_at':int(time.time()),'gpu_backend':device,'context_size':int(__import__('os').environ.get('CONTEXT_SIZE','0')),'gpu_vram_before_bytes':int(before),'gpu_vram_peak_bytes':int(peak),'gpu_vram_after_bytes':int(after),'success':1,'notes':'measured by backfill-smoke-vram.sh'}
req=urllib.request.Request(base+'/smoke',data=json.dumps(payload).encode(),method='POST',headers={'Content-Type':'application/json'})
with urllib.request.urlopen(req,timeout=15) as r: print(r.read().decode())
PY
printf 'measured model=%s device=%s before=%s peak=%s after=%s\n' "$MODEL_ID" "$DEVICE" "$GPU_BEFORE" "$GPU_PEAK" "$GPU_AFTER"
