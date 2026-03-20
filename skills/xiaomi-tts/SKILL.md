---
name: xiaomi-tts
description: Xiaomi MiMo TTS (文字转语音)。使用小米 MiMo TTS API 将文字转换为语音。
metadata:
  openclaw:
    requires:
      bins: ["curl", "base64"]
---

# Xiaomi MiMo TTS

小米文字转语音服务，基于 MiMo v2 TTS 模型。

## 安装

```bash
# 确保 curl 和 base64 可用（系统自带）
which curl base64
```

## 配置

1. 获取 Xiaomi MiMo API Key（https://api.xiaomimimo.com）
2. 编辑 `scripts/xiaomi-tts.sh`，替换 `API_KEY`：

```bash
API_KEY="你的API Key"
```

## 使用方法

```bash
./scripts/xiaomi-tts.sh "要转换的文字"
./scripts/xiaomi-tts.sh "要转换的文字" /tmp/output.mp3
```

### 参数

- 第一个参数：要转换的文字（必填）
- 第二个参数：输出文件路径（可选，默认 `/tmp/tts_output.mp3`）

### 返回值

- 成功：输出生成的音频文件路径
- 失败：输出错误信息到 stderr，exit 1

## 示例

```bash
# 生成默认路径
./scripts/xiaomi-tts.sh "你好，我是虾仔"

# 指定输出路径
./scripts/xiaomi-tts.sh "你好" /tmp/hello.mp3
```

## 注意事项

- 需要有效的 API Key
- API Key 放在脚本中，注意安全
- 输出格式为 MP3
