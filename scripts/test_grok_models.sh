#!/usr/bin/env bash
# scripts/test_grok_models.sh
#
# 一键测试 grok-search 池子里 fast/auto/expert 等模型是否真的可用。
# 输出每个模型的：成功/失败、实际命中的 endpoint、耗时、sources 数、回答片段。
#
# 用法：
#   ./scripts/test_grok_models.sh
#   ./scripts/test_grok_models.sh -m grok-4.20-fast -m grok-4.20-auto
#   ./scripts/test_grok_models.sh -q "今天是几号？" -t 60s
#   ./scripts/test_grok_models.sh --bin /abs/path/to/grok-search
#
# 退出码: 0 = 全部模型成功; 1 = 至少一个失败 / 配置异常
set -u

# ---- defaults --------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJ_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DEFAULT="$PROJ_DIR/grok-search"

QUERY='RackNerd 是家宽 IP 还是机房 IP？只用一句中文回答，并给一句原因。'
TIMEOUT='60s'
BIN="$BIN_DEFAULT"
MODELS=()

usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

Options:
  -q, --query    <text>     测试用的搜索 query (default: RackNerd...)
  -m, --model    <name>     追加一个要测的模型，可多次使用
                            默认测: grok-4.20-fast / -auto / -expert
  -t, --timeout  <dur>      单次搜索超时 (default: 60s)
      --bin      <path>     指定 grok-search 二进制 (default: ./grok-search)
  -h, --help                显示帮助
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    -q|--query)   QUERY="$2"; shift 2;;
    -m|--model)   MODELS+=("$2"); shift 2;;
    -t|--timeout) TIMEOUT="$2"; shift 2;;
    --bin)        BIN="$2"; shift 2;;
    -h|--help)    usage; exit 0;;
    *) echo "unknown arg: $1" >&2; usage; exit 2;;
  esac
done

if [ "${#MODELS[@]}" -eq 0 ]; then
  MODELS=(grok-4.20-fast grok-4.20-auto grok-4.20-expert)
fi

if [ ! -x "$BIN" ]; then
  echo "[FATAL] grok-search 二进制不存在或无执行权限: $BIN" >&2
  echo "        可以先在项目根执行: go build -o grok-search ." >&2
  exit 1
fi

# ---- helpers ---------------------------------------------------------------
have_python3() { command -v python3 >/dev/null 2>&1; }
have_jq()      { command -v jq      >/dev/null 2>&1; }

# 提取 JSON 字段；优先 python3，其次 jq；都没有就 grep。
json_get() {
  # $1 = file, $2 = key (顶层)
  local f="$1" k="$2"
  if have_python3; then
    python3 -c "import json,sys
o=json.load(open(sys.argv[1]))
v=o.get(sys.argv[2],'')
print('' if v is None else v)" "$f" "$k"
  elif have_jq; then
    jq -r --arg k "$k" '.[$k] // ""' "$f"
  else
    grep -oE "\"$k\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" "$f" | head -n1 | sed -E "s/.*:\s*\"([^\"]*)\".*/\1/"
  fi
}

json_get_int() {
  local f="$1" k="$2"
  if have_python3; then
    python3 -c "import json,sys
o=json.load(open(sys.argv[1]))
v=o.get(sys.argv[2])
print(0 if v is None else v)" "$f" "$k"
  elif have_jq; then
    jq -r --arg k "$k" '.[$k] // 0' "$f"
  else
    grep -oE "\"$k\"[[:space:]]*:[[:space:]]*[0-9]+" "$f" | head -n1 | grep -oE '[0-9]+$'
  fi
}

short() {
  # 截断到前 160 字符，去换行
  awk 'BEGIN{ORS=""} {gsub(/[\r\n]+/," "); print}' | cut -c1-160
}

# ---- step 1: probe ---------------------------------------------------------
echo "============================================================"
echo " grok-search models test"
echo "------------------------------------------------------------"
echo " bin     : $BIN"
echo " timeout : $TIMEOUT"
echo " query   : $QUERY"
echo " models  : ${MODELS[*]}"
echo "============================================================"
echo

PROBE_OUT="$(mktemp -t grok_probe.XXXXXX.json)"
trap 'rm -f "$PROBE_OUT"' EXIT

echo "[probe] 端点探活中..."
if ! "$BIN" cli probe --list-timeout 8s --json > "$PROBE_OUT" 2>/dev/null; then
  echo "[probe] FAILED：probe 命令调用失败" >&2
  exit 1
fi

if have_python3; then
  python3 - "$PROBE_OUT" <<'PY'
import json, sys
p=sys.argv[1]
obj=json.load(open(p))
eps=obj.get('endpoints', []) or []
print(f"[probe] tavily_enabled={obj.get('tavily_enabled')}  jina={obj.get('jina_api_url')}")
print(f"[probe] endpoints   ({len(eps)}):")
for ep in eps:
    models=ep.get('models') or []
    has=lambda m: m in models
    print(f"  - {ep.get('name'):<12} ok={ep.get('ok')!s:<5} default={ep.get('model'):<28} "
          f"fast={has('grok-4.20-fast')!s:<5} auto={has('grok-4.20-auto')!s:<5} expert={has('grok-4.20-expert')!s:<5} "
          f"models={ep.get('models_count')}")
PY
else
  echo "[probe] (python3 不可用，跳过详细解析；请直接看 $PROBE_OUT)"
fi
echo

# ---- step 2: 逐模型测试 ---------------------------------------------------
PASS=0
FAIL=0
declare -a SUMMARY=()

for m in "${MODELS[@]}"; do
  echo "------------------------------------------------------------"
  echo "[test] model = $m"
  OUT="$(mktemp -t "grok_$(echo "$m" | tr -c 'A-Za-z0-9' '_').XXXXXX.json")"
  ERR="${OUT}.err"

  T0=$(date +%s)
  if "$BIN" cli search "$QUERY" --model "$m" --json --timeout "$TIMEOUT" > "$OUT" 2> "$ERR"; then
    T1=$(date +%s); DUR=$((T1 - T0))
    engine="$(json_get "$OUT" engine)"
    endpoint="$(json_get "$OUT" endpoint_name)"
    real_model="$(json_get "$OUT" model)"
    src_count="$(json_get_int "$OUT" sources_count)"
    content_snip="$(json_get "$OUT" content | short)"
    echo "  status         : OK  (${DUR}s)"
    echo "  engine         : $engine"
    echo "  endpoint_name  : $endpoint"
    echo "  model_returned : $real_model"
    echo "  sources_count  : $src_count"
    echo "  content        : $content_snip"
    PASS=$((PASS+1))
    SUMMARY+=("OK   $m  -> engine=$engine endpoint=$endpoint sources=$src_count duration=${DUR}s")
  else
    T1=$(date +%s); DUR=$((T1 - T0))
    echo "  status         : FAILED  (${DUR}s)"
    echo "  stderr (head)  :"
    sed -n '1,10p' "$ERR" | sed 's/^/    /'
    FAIL=$((FAIL+1))
    SUMMARY+=("FAIL $m  -> see $ERR (duration=${DUR}s)")
  fi
  rm -f "$OUT" "$ERR" 2>/dev/null
  echo
done

# ---- step 3: 汇总 ---------------------------------------------------------
echo "============================================================"
echo " summary"
echo "------------------------------------------------------------"
for line in "${SUMMARY[@]}"; do echo "  $line"; done
echo "------------------------------------------------------------"
echo "  passed: $PASS    failed: $FAIL    total: ${#MODELS[@]}"
echo "============================================================"

[ "$FAIL" -eq 0 ]