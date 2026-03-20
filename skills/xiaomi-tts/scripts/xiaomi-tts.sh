#!/bin/bash
# Xiaomi MiMo TTS Wrapper for OpenClaw
# 用法: ./xiaomi-tts.sh "要转换的文字" [输出路径]

API_KEY=""
BASE_URL="https://api.xiaomimimo.com/v1/chat/completions"
MODEL="mimo-v2-tts"

TEXT="$1"
OUTPUT="${2:-/tmp/tts_output.mp3}"

# 调用 API
RESPONSE=$(curl -s "$BASE_URL" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"model\": \"$MODEL\", \"messages\": [{\"role\": \"user\", \"content\": \"$TEXT\"}, {\"role\": \"assistant\", \"content\": \"$TEXT\"}]}")

# 提取 base64 音频数据
AUDIO_DATA=$(echo "$RESPONSE" | grep -o '"data":"[^"]*"' | sed 's/"data":"//;s/"$//')

if [ -z "$AUDIO_DATA" ]; then
  echo "Error: $RESPONSE" >&2
  exit 1
fi

# 解码并保存
echo "$AUDIO_DATA" | base64 -d > "$OUTPUT"

# 检查输出
if [ -s "$OUTPUT" ]; then
  echo "$OUTPUT"
else
  echo "Error: Failed to generate audio" >&2
  exit 1
fi
