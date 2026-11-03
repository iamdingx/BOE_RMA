#!/usr/bin/env sh
# ============================================================
# BOE US RMA —— 回滚脚本
# 用法：
#   ./rollback.sh                     # 回滚到 PREV_TAG（默认 0.9.0）
#   ./rollback.sh 1.0.0-rc1           # 回滚到指定镜像 tag
# 说明：镜像回滚不自动回退数据库结构。DDL 变更请先执行 db/rollback_<ver>.sql（若有）。
# ============================================================
set -eu

PREV_TAG="${1:-${PREV_TAG:-0.9.0}}"
COMPOSE_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$COMPOSE_DIR"

echo "[1/4] 当前运行镜像："
docker compose ps --format 'table {{.Name}}\t{{.Image}}\t{{.Status}}' || true

echo "[2/4] 回滚到 tag=${PREV_TAG}"
TAG="$PREV_TAG" docker compose up -d --no-deps rma-api rma-web

echo "[3/4] 等待健康检查…"
i=0
while [ $i -lt 30 ]; do
  code=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:"${WEB_PORT:-8088}"/health || true)
  if [ "$code" = "200" ]; then
    echo "  健康检查通过"
    break
  fi
  i=$((i + 1))
  sleep 2
done
if [ $i -ge 30 ]; then
  echo "  !! 健康检查未通过，请检查日志：docker compose logs --tail=200 rma-api"
  exit 1
fi

echo "[4/4] 回滚完成"
docker compose ps --format 'table {{.Name}}\t{{.Image}}\t{{.Status}}'
echo "提示：如需回滚数据库，请手工执行对应版本的回退 SQL，并在审计中记录本次回滚操作。"
