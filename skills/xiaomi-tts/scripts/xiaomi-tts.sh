#!/bin/bash
# Xiaomi MiMo TTS Wrapper for OpenClaw

set -euo pipefail

API_KEY="${XIAOMI_MIMO_API_KEY:-your_api_key_here}"
BASE_URL="https://api.xiaomimimo.com/v1/chat/completions"
MODEL="mimo-v2-tts"
VOICE="mimo_default"
SPEED="1.0"
PITCH="0"
RESPONSE_FORMAT="mp3"
USER_PROMPT=""
STYLE_TAGS=""
OUTPUT=""
TEXT=""

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
  -h, --help                显示帮助

说明:
  1. 待合成文本会作为 assistant 角色发送，这是 MiMo TTS 的要求。
  2. 风格控制的本质是把 <style>...</style> 放在文本开头；可手写到文本里，
   也可通过 --style 自动追加。
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
