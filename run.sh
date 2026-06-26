#!/usr/bin/env bash
# narcissus 런처 — JSONL 뷰어(8765)와 FleetBoard(7777)를 한 번에 띄우고
# Ctrl+C 한 번에 같이 내린다. ADO_* 등 환경변수는 그대로 상속된다.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "▶ building…"
( cd "$ROOT/src"        && go build -o jsonl-viewer . )
( cd "$ROOT/fleetboard" && go build -o fleetboard   . )

PIDS=()
cleanup() {
  echo
  echo "▶ stopping…"
  kill "${PIDS[@]}" 2>/dev/null || true
  wait "${PIDS[@]}" 2>/dev/null || true
}
trap cleanup INT TERM

echo "▶ jsonl-viewer → http://localhost:8765  (브라우저 자동 오픈)"
"$ROOT/src/jsonl-viewer" &
PIDS+=("$!")

echo "▶ fleetboard   → http://127.0.0.1:7777"
"$ROOT/fleetboard/fleetboard" &
PIDS+=("$!")

echo "▶ 둘 다 떴습니다. 종료하려면 Ctrl+C."
wait
