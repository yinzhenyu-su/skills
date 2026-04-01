#!/bin/bash
# Xiaomi MiMo TTS Wrapper for OpenClaw

set -euo pipefail
API_KEY="${XIAOMI_MIMO_API_KEY:-your_api_key_here}"

# 如果 API_KEY 未设置，尝试从 $HOME/.config/yinzhenyu/.env 加载环境变量，如果文件存在
if [[ "$API_KEY" == "your_api_key_here" ]]; then
  ENV_FILE="$HOME/.config/yinzhenyu/.env"
  if [[ -f "$ENV_FILE" ]]; then
    # shellcheck disable=SC1090
    source "$ENV_FILE"
  else
    echo "Error: $ENV_FILE not found. Please create it and set XIAOMI_MIMO_API_KEY environment variable." >&2
  fi
fi

# ========== 扩展配置 ==========
# 配置路径（优先级：项目级 > 用户级）
CONFIG_PROJECT_DIR="${PWD}/.xiaomi-tts"
CONFIG_USER_DIR="$HOME/.config/xiaomi-tts"
CONFIG_FILE="EXTEND.md"

# 从 YAML front matter 解析扩展配置
parse_extend_config() {
  local config_path="$1"
  local -A config

  if [[ ! -f "$config_path" ]]; then
    return 1
  fi

  # 提取 --- 包裹的 YAML 内容
  local yaml_content
  yaml_content=$(sed -n '/^---$/,/^---$/p' "$config_path" | sed '1d;$d')

  # 解析 YAML 行: key: value
  while IFS= read -r line; do
    # 跳过空行和注释
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue

    # 解析 key: value
    if [[ "$line" =~ ^[[:space:]]*([^:]+):[[:space:]]*(.*) ]]; then
      local key="${BASH_REMATCH[1]#[[:space:]]}"
      local value="${BASH_REMATCH[2]#[[:space:]]}"
      # 去除引号
      value="${value#\"}"; value="${value%\"}"
      value="${value#'}"; value="${value%'}"
      # 跳过空值
      [[ -z "$value" || "$value" == "null" ]] && continue
      config["$key"]="$value"
    fi
  done <<< "$yaml_content"

  # 设置配置变量
  [[ -n "${config[default_voice]:-}" ]] && VOICE="${config[default_voice]}"
  [[ -n "${config[default_speed]:-}" ]] && SPEED="${config[default_speed]}"
  [[ -n "${config[default_pitch]:-}" ]] && PITCH="${config[default_pitch]}"
  [[ -n "${config[default_format]:-}" ]] && RESPONSE_FORMAT="${config[default_format]}"
  [[ -n "${config[default_model]:-}" ]] && MODEL="${config[default_model]}"
  [[ -n "${config[default_style]:-}" ]] && STYLE_TAGS="${config[default_style]}"
}

# 加载扩展配置（按优先级）
load_extend_config() {
  if [[ -f "${CONFIG_PROJECT_DIR}/${CONFIG_FILE}" ]]; then
    parse_extend_config "${CONFIG_PROJECT_DIR}/${CONFIG_FILE}"
  elif [[ -f "${CONFIG_USER_DIR}/${CONFIG_FILE}" ]]; then
    parse_extend_config "${CONFIG_USER_DIR}/${CONFIG_FILE}"
  fi
}

# 保存扩展配置
save_extend_config() {
  local target_dir="$1"
  local key="$2"
  local value="$3"

  # 创建目录
  mkdir -p "$target_dir"

  local config_path="${target_dir}/${CONFIG_FILE}"
  local temp_file
  temp_file=$(mktemp)

  # 如果文件存在，读取现有内容
  local existing_content=""
  if [[ -f "$config_path" ]]; then
    existing_content=$(cat "$config_path")
  fi

  # 构建新的 front matter
  {
    echo "---"
    # 解析现有配置
    if [[ -n "$existing_content" ]]; then
      local yaml_content
      yaml_content=$(echo "$existing_content" | sed -n '/^---$/,/^---$/p' | sed '1d;$d')
      local -A config
      while IFS= read -r line; do
        [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
        if [[ "$line" =~ ^[[:space:]]*([^:]+):[[:space:]]*(.*) ]]; then
          local k="${BASH_REMATCH[1]#[[:space:]]}"
          local v="${BASH_REMATCH[2]#[[:space:]]}"
          v="${v#\"}"; v="${v%\"}"
          v="${v#'}"; v="${v%'}"
          [[ -n "$v" && "$v" != "null" ]] && config["$k"]="$v"
        fi
      done <<< "$yaml_content"

      # 保留其他配置项，更新目标键
      for k in "${!config[@]}"; do
        if [[ "$k" == "$key" ]]; then
          echo "${k}: $value"
        else
          echo "${k}: ${config[$k]}"
        fi
      done
    fi
    # 如果文件不存在或没有该键，追加
    if [[ -z "$existing_content" || ! "$existing_content" =~ default_${key} ]]; then
      echo "${key}: $value"
    fi
    echo "---"
  } > "$temp_file"

  mv "$temp_file" "$config_path"
}

# 列出当前配置
list_extend_config() {
  echo "当前 xiaomi-tts 配置："
  echo ""
  echo "  --voice   = ${VOICE}"
  echo "  --speed   = ${SPEED}"
  echo "  --pitch   = ${PITCH}"
  echo "  --format  = ${RESPONSE_FORMAT}"
  echo "  --model   = ${MODEL}"
  echo "  --style   = ${STYLE_TAGS}"
  echo ""
  echo "配置文件："
  if [[ -f "${CONFIG_PROJECT_DIR}/${CONFIG_FILE}" ]]; then
    echo "  项目级: ${CONFIG_PROJECT_DIR}/${CONFIG_FILE}"
  fi
  if [[ -f "${CONFIG_USER_DIR}/${CONFIG_FILE}" ]]; then
    echo "  用户级: ${CONFIG_USER_DIR}/${CONFIG_FILE}"
  fi
  if [[ ! -f "${CONFIG_PROJECT_DIR}/${CONFIG_FILE}" && ! -f "${CONFIG_USER_DIR}/${CONFIG_FILE}" ]]; then
    echo "  (未找到配置文件)"
  fi
}

# 预先加载扩展配置
load_extend_config

BASE_URL="https://api.xiaomimimo.com/v1/chat/completions"
MODEL="${MODEL:-mimo-v2-tts}"
VOICE="${VOICE:-default_zh}"
SPEED="${SPEED:-1.0}"
PITCH="${PITCH:-0}"
RESPONSE_FORMAT="${RESPONSE_FORMAT:-mp3}"
USER_PROMPT=""
STYLE_TAGS="${STYLE_TAGS:-}"
OUTPUT=""
TEXT=""
SAVE_CONFIG_MODE=""

usage() {
  cat <<'EOF'
用法:
  ./scripts/xiaomi-tts.sh [选项] "待合成文本" [输出路径]

选项:
  -o, --output PATH         输出音频路径，默认 /tmp/tts_output.<format>
    --voice NAME          预置音色，默认 mimo_default
    --speed VALUE         语速倍率，默认 1.0
    --pitch VALUE         音调偏移，默认 0
    --format FORMAT       输出格式: mp3, wav, pcm，默认 mp3
    --model NAME          模型名称，默认 mimo-v2-tts
    --style TEXT          自动在文本开头追加 <style>TEXT</style>
    --user-prompt TEXT    可选的 user 角色提示，用于辅助语气与风格
    --save-config         将当前参数保存为默认值（保存到用户级配置）
    --list-config         显示当前配置
  -h, --help                显示帮助

扩展配置:
  配置文件位置（优先级）:
    1. {pwd}/.xiaomi-tts/EXTEND.md (项目级)
    2. ~/.config/xiaomi-tts/EXTEND.md (用户级)

  保存配置示例:
    ./scripts/xiaomi-tts.sh --save-config --voice default_zh --speed 1.2 --pitch 2

  配置项: default_voice, default_speed, default_pitch, default_format,
         default_model, default_style

说明:
  1. 待合成文本会作为 assistant 角色发送，这是 MiMo TTS 的要求。
  2. 风格控制的本质是把 <style>...</style> 放在文本开头；可手写到文本里，
   也可通过 --style 自动追加。
  3. CLI 参数会覆盖保存的配置默认值。
EOF
}

decode_base64() {
  if base64 --help 2>/dev/null | grep -q -- '--decode'; then
    base64 --decode
  else
    base64 -D
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -o|--output)
      OUTPUT="$2"
      shift 2
      ;;
    --voice)
      VOICE="$2"
      shift 2
      ;;
    --speed)
      SPEED="$2"
      shift 2
      ;;
    --pitch)
      PITCH="$2"
      shift 2
      ;;
    --format)
      RESPONSE_FORMAT="$2"
      shift 2
      ;;
    --model)
      MODEL="$2"
      shift 2
      ;;
    --style)
      STYLE_TAGS="$2"
      shift 2
      ;;
    --user-prompt)
      USER_PROMPT="$2"
      shift 2
      ;;
    --save-config)
      SAVE_CONFIG_MODE="1"
      shift
      ;;
    --list-config)
      list_extend_config
      exit 0
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -*)
      echo "Error: Unknown option $1" >&2
      usage >&2
      exit 1
      ;;
    *)
      if [[ -z "$TEXT" ]]; then
        TEXT="$1"
      elif [[ -z "$OUTPUT" ]]; then
        OUTPUT="$1"
      else
        echo "Error: Unexpected argument $1" >&2
        usage >&2
        exit 1
      fi
      shift
      ;;
  esac
done

# --save-config 模式：保存当前参数并退出
if [[ -n "$SAVE_CONFIG_MODE" ]]; then
  # 保存到用户级配置目录
  save_extend_config "$CONFIG_USER_DIR" "default_voice" "$VOICE"
  save_extend_config "$CONFIG_USER_DIR" "default_speed" "$SPEED"
  save_extend_config "$CONFIG_USER_DIR" "default_pitch" "$PITCH"
  save_extend_config "$CONFIG_USER_DIR" "default_format" "$RESPONSE_FORMAT"
  save_extend_config "$CONFIG_USER_DIR" "default_model" "$MODEL"
  save_extend_config "$CONFIG_USER_DIR" "default_style" "$STYLE_TAGS"
  echo "配置已保存到: ${CONFIG_USER_DIR}/${CONFIG_FILE}"
  exit 0
fi

if [[ -z "$TEXT" ]]; then
  usage >&2
  exit 1
fi

if [[ -z "$OUTPUT" ]]; then
  OUTPUT="/tmp/tts_output.${RESPONSE_FORMAT}"
fi

if [[ "$API_KEY" == "your_api_key_here" ]]; then
  echo "Error: Please set XIAOMI_MIMO_API_KEY before calling the script." >&2
  exit 1
fi

ASSISTANT_TEXT="$TEXT"
if [[ -n "$STYLE_TAGS" ]]; then
  ASSISTANT_TEXT="<style>${STYLE_TAGS}</style>${TEXT}"
fi

MESSAGES=$(jq -n \
  --arg user_prompt "$USER_PROMPT" \
  --arg assistant_text "$ASSISTANT_TEXT" \
  '(
    if $user_prompt == "" then
      []
    else
      [{"role": "user", "content": $user_prompt}]
    end
  ) + [{"role": "assistant", "content": $assistant_text}]')

REQUEST_BODY=$(jq -n \
  --arg model "$MODEL" \
  --arg voice "$VOICE" \
  --arg format "$RESPONSE_FORMAT" \
  --argjson speed "$SPEED" \
  --argjson pitch "$PITCH" \
  --argjson messages "$MESSAGES" \
  '{
    model: $model,
    messages: $messages,
    audio: {
      voice: $voice,
      format: $format,
      speed: $speed,
      pitch: $pitch
    }
  }')

RESPONSE=$(curl -sS "$BASE_URL" \
  -H "api-key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d "$REQUEST_BODY")

AUDIO_DATA=$(printf '%s' "$RESPONSE" | jq -r '.choices[0].message.audio.data // empty')

if [[ -z "$AUDIO_DATA" ]]; then
  ERROR_MSG=$(printf '%s' "$RESPONSE" | jq -r '.error.message // .message // "Unknown error"')
  echo "Error: $ERROR_MSG" >&2
  echo "Raw Response: $RESPONSE" >&2
  exit 1
fi

printf '%s' "$AUDIO_DATA" | decode_base64 > "$OUTPUT"

if [[ -s "$OUTPUT" ]]; then
  echo "$OUTPUT"
else
  echo "Error: Failed to generate audio file at $OUTPUT" >&2
  exit 1
fi
