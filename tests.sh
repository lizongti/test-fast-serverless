#!/usr/bin/env bash
set -euo pipefail

# 运行远程测试用例的便捷脚本。
# 注意：测试逻辑仍在 Go 的 _test.go 中；本脚本只负责设置必要环境变量并调用 go test。
#
# 用法：
#   ./tests.sh [stage] [stack_name] [repeat]
# 示例：
#   ./tests.sh dev
#   ./tests.sh dev test-serverless 20

STAGE="${1:-dev}"
STACK_NAME="${2:-}"
REPEAT="${3:-10}"

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

if [[ -z "$STACK_NAME" ]]; then
  SAMCONFIG="$ROOT_DIR/samconfig.toml"

  if [[ -f "$SAMCONFIG" ]]; then
    # 读取 [default.deploy.parameters].stack_name
    # 优先使用 yq（mikefarah yq v4 支持 -p=toml）；如果未安装 yq，则回退到 awk 解析。
    if command -v yq >/dev/null 2>&1; then
      STACK_NAME=$(yq -p=toml '.default.deploy.parameters.stack_name' "$SAMCONFIG" 2>/dev/null | tr -d '"' || true)
      if [[ "$STACK_NAME" == "null" ]]; then
        STACK_NAME=""
      fi
    fi

    if [[ -z "$STACK_NAME" ]]; then
      STACK_NAME=$(awk '
        BEGIN { inside=0 }
        /^\[default\.deploy\.parameters\]$/ { inside=1; next }
        /^\[/ { inside=0 }
        inside && $0 ~ /^stack_name[[:space:]]*=/ {
          if (match($0, /"[^"]+"/)) {
            print substr($0, RSTART+1, RLENGTH-2)
          }
          exit
        }
      ' "$SAMCONFIG")
    fi
  fi

  if [[ -z "$STACK_NAME" ]]; then
    STACK_NAME="testsqs-${STAGE}"
  fi
fi

export RUN_REMOTE_TESTS=1
export STAGE
export STACK_NAME
export REPEAT

echo "RUN_REMOTE_TESTS=$RUN_REMOTE_TESTS"
echo "STAGE=$STAGE"
echo "STACK_NAME=$STACK_NAME"
echo "REPEAT=$REPEAT"

# 运行测试并捕获输出，便于将结果块自动写入 result.md。
TMP_OUT="${TMPDIR:-/tmp}/testsqs-test-output.$$".log
go test -run TestStepFunctionsFlowLatency -v | tee "$TMP_OUT"

RESULT_MD="$ROOT_DIR/result.md"
EXTRACTED="${TMPDIR:-/tmp}/testsqs-result-block.$$".md

awk '
  /^===BEGIN_RESULT_MD===/ {inblock=1; next}
  /^===END_RESULT_MD===/   {inblock=0}
  inblock==1 {print}
' "$TMP_OUT" > "$EXTRACTED"

if [[ -s "$EXTRACTED" ]]; then
  TS=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  {
    echo
    echo "## Run $TS"
    echo
    cat "$EXTRACTED"
  } >> "$RESULT_MD"

  echo
  echo "Wrote test output block to: $RESULT_MD"
else
  echo
  echo "WARNING: no result block found in test output; result.md not updated."
fi

rm -f "$TMP_OUT" "$EXTRACTED"
