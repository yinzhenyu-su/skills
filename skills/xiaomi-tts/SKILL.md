---
name: xiaomi-tts
description: Xiaomi MiMo TTS (语音合成)。基于 MiMo-v2 模型，支持预置音色、语速、音调、风格标签和多种响应格式。
metadata:
  openclaw:
    homepage: https://github.com/yinzhenyu-su/skills/
    requires:
      bins: ["curl", "base64", "jq"]
      env: ["XIAOMI_MIMO_API_KEY"]
---

# Xiaomi MiMo TTS

小米 MiMo 语音合成（TTS）服务，基于先进的 MiMo-v2 模型，提供高质量的文字转语音功能。

## 安装与依赖

1. **基础工具**：确保系统已安装 `curl`、`base64` 和 `jq`。
2. **脚本权限**：确保 `scripts/xiaomi-tts.sh` 具有执行权限。

```bash
chmod +x scripts/xiaomi-tts.sh
```

## 配置

1. **获取 API Key**：访问 [小米 MiMo 开放平台](https://api.xiaomimimo.com) 注册并获取 API Key。
2. **设置环境变量**：为了安全，建议通过环境变量设置 API Key，或直接修改脚本。

```bash
export XIAOMI_MIMO_API_KEY="你的_API_KEY"
```

## 扩展配置

脚本支持将常用参数保存为默认值，避免每次调用时重复指定。

### 配置文件位置

| 层级 | 路径 | 优先级 |
|------|------|--------|
| 项目级 | `{pwd}/.xiaomi-tts/EXTEND.md` | 高 |
| 用户级 | `~/.config/xiaomi-tts/EXTEND.md` | 低 |

### 保存配置

```bash
# 保存当前参数为默认值
./scripts/xiaomi-tts.sh --save-config --voice default_zh --speed 1.2 --pitch 2

# 保存风格配置
./scripts/xiaomi-tts.sh --save-config --style "东北话"
```

### 查看当前配置

```bash
./scripts/xiaomi-tts.sh --list-config
```

输出示例：

```
当前 xiaomi-tts 配置：

  --voice   = default_zh
  --speed   = 1.2
  --pitch   = 2
  --format  = mp3
  --model   = mimo-v2-tts
  --style   = 东北话

配置文件：
  用户级: /home/user/.config/xiaomi-tts/EXTEND.md
```

### 配置项说明

| 配置项 | 说明 | 可选值 |
|--------|------|--------|
| `default_voice` | 默认音色 | `default_zh`, `mimo_default`, `default_en` |
| `default_speed` | 默认语速 | `0.5` - `2.0` |
| `default_pitch` | 默认音调 | `-10` - `10` |
| `default_format` | 默认输出格式 | `mp3`, `wav`, `pcm` |
| `default_model` | 默认模型 | `mimo-v2-tts` |
| `default_style` | 默认风格标签 | 如 `开心`, `东北话`, `粤语` |

### 配置优先级

1. **CLI 参数** > 扩展配置 > 硬编码默认值
2. **项目级配置** > 用户级配置
3. 对风格控制而言，`--style` 会覆盖文本中手动写入的 `<style>...</style>` 标签。

### EXTEND.md 格式示例

```yaml
---
default_voice: default_zh
default_speed: 1.0
default_pitch: 0
default_format: mp3
default_model: mimo-v2-tts
default_style: ""
---
```

## 使用方法

### 命令行调用

```bash
./scripts/xiaomi-tts.sh [选项] "待合成文本" [输出路径]
```

### 核心参数

- **文本** (必填): 要转换的文字内容。
- **输出路径** (可选): 生成的音频文件路径，默认存放在 `/tmp/tts_output.mp3`。

### 进阶配置

脚本已支持以下高级参数：

| 参数 | 说明 | 可选值 / 范围 |
| :--- | :--- | :--- |
| `model` | 模型版本 | `mimo-v2-tts` (默认) |
| `voice` | 预置音色 | `default_zh` (默认), `mimo_default`, `default_en` |
| `speed` | 语速倍率 | `0.5` - `2.0` (默认 `1.0`) |
| `pitch` | 音调偏移 | `-10` - `10` (默认 `0`) |
| `style` | 风格控制 | 通过 `--style "开心"` 自动追加 `<style>开心</style>`，或手动写入文本开头 |
| `user_prompt` | 辅助提示 | 可选，作为 `user` 角色补充语气与上下文 |
| `response_format` | 响应格式 | `mp3` (默认), `wav`, `pcm` |

## 示例

```bash
# 基础用法
./scripts/xiaomi-tts.sh "你好，欢迎使用小米语音合成。"

# 指定音色、格式与输出路径
./scripts/xiaomi-tts.sh --voice default_zh --format wav "正在生成测试音频" ./output/test.wav

# 自动追加整体风格标签
./scripts/xiaomi-tts.sh --style "开心" "明天就是周五了，真开心！"

# 提供可选的 user 角色提示
./scripts/xiaomi-tts.sh --user-prompt "请用温柔的晚安电台语气。" "今晚也辛苦了，早点休息。"
```

## 可选预置音色
使用时，可在 `audio.voice` 中设置预置音色。

| 音色名 | `voice` 参数 |
| :--- | :--- |
| MiMo-默认 | `mimo_default` |
| MiMo-中文女声 | `default_zh` |
| MiMo-英文女声 | `default_en` |

当前不支持音色克隆。

## 风格控制
### 整体风格控制

将 `<style>style</style>` 置于目标文本开头，其中 `style` 为需要生成的音频风格。如需设置多种风格，请将多个风格名称置于同一个 `<style>` 标签内，分隔符不限。

格式：`<style>风格1 风格2</style>待合成内容`

脚本支持两种用法：

- 直接把 `<style>...</style>` 写进文本开头。
- 使用 `--style "风格1 风格2"`，由脚本自动追加标签。

推荐风格示例：

| 风格类型 | 风格示例 |
| :--- | :--- |
| 语速控制 | `变快` / `变慢` |
| 情绪变化 | `开心` / `悲伤` / `生气` |
| 角色扮演 | `孙悟空` / `林黛玉` |
| 风格变化 | `悄悄话` / `夹子音` / `台湾腔` |
| 方言 | `东北话` / `四川话` / `河南话` / `粤语` |

示例：

```text
<style>开心</style>明天就是周五了，真开心！
<style>东北话</style>哎呀妈呀，这天儿也忒冷了吧！你说这风，嗖嗖的，跟刀子似的，割脸啊！
<style>粤语</style>呢个真係好正啊！食过一次就唔会忘记！
```

### 音频标签细粒度控制

通过音频标签，可以对声音进行更细粒度的控制，例如语气、情绪、停顿、呼吸声、咳嗽和语速变化等。

示例：

```text
（紧张，深呼吸）呼……冷静，冷静。不就是一个面试吗……（语速加快，碎碎念）自我介绍已经背了五十遍了，应该没问题的。加油，你可以的……（小声）哎呀，领带歪没歪？
（极其疲惫，有气无力）师傅……到地方了叫我一声……（长叹一口气）我先眯一会儿，这班加得我魂儿都要散了。
如果我当时……（沉默片刻）哪怕再坚持一秒钟，结果是不是就不一样了？（苦笑）呵，没如果了。
（寒冷导致的急促呼吸）呼——呼——这、这大兴安岭的雪……（咳嗽）简直能把人骨头冻透了……别、别停下，走，快走。
（提高音量喊话）大姐！这鱼新鲜着呢！早上刚捞上来的！哎！那个谁，别乱翻，压坏了你赔啊？！
```

### 注意事项

- 语音合成的目标文本需填写在 `assistant` 角色的消息中，不可放在 `user` 角色中。
- `user` 角色消息为可选参数，但建议按场景提供，可辅助调整整体语气与风格。
- 手动指定整体语音风格时，需将 `<style>...</style>` 放在目标文本最开头；若使用 `--style` 参数则由脚本自动处理。
- 如需体验更佳的唱歌风格，必须在文本最开头仅添加 `<style>唱歌</style>`，格式为 `<style>唱歌</style>目标文本`。

### 风格控制调用示例

```bash
# 手动写入 style 标签
./scripts/xiaomi-tts.sh "<style>开心</style>明天就是周五了，真开心！"

# 通过参数自动追加 style 标签
./scripts/xiaomi-tts.sh --style "东北话" "哎呀妈呀，这天儿也忒冷了吧！"

# 结合 user 提示与细粒度标签
./scripts/xiaomi-tts.sh \
  --user-prompt "请模拟夜间电台主持人的轻声讲述。" \
  "（小声）今晚的风有点凉，回家的路上记得把外套拉好。"
```

## 常见问题

- **401 Unauthorized**: 请检查 API Key 是否正确配置。
- **429 Too Many Requests**: 触发频率限制，请稍后再试或检查配额。
- **文本长度**: 建议单次合成文本不超过 500 字以获得最佳体验。

## 相关链接

- [小米 MiMo 官方文档](https://platform.xiaomimimo.com/#/docs/usage-guide/speech-synthesis)
- [API 仪表盘](https://api.xiaomimimo.com)
