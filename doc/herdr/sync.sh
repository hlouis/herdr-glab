#!/usr/bin/env bash
# 从 herdr 官方仓库拉取指定版本的文档。用法：doc/herdr/sync.sh [版本，默认 v0.9.0]
set -euo pipefail

VERSION="${1:-v0.9.0}"
DIR="$(cd "$(dirname "$0")" && pwd)"
BASE="https://raw.githubusercontent.com/herdrdev/herdr/${VERSION}/docs/next/website/src"
PAGES=(plugins socket-api cli-reference configuration concepts marketplace keyboard agent-automation session-state troubleshooting)

for page in "${PAGES[@]}"; do
  curl -sfL "${BASE}/content/docs/${page}.mdx" -o "${DIR}/${page}.md"
done
curl -sfL "${BASE}/data/config-reference.json" -o "${DIR}/config-reference.json"
herdr api schema --output "${DIR}/api-schema.json"

echo "synced herdr docs ${VERSION} (api schema from local herdr $(herdr --version))"
