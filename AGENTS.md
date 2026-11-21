# PROJECT KNOWLEDGE BASE

**Branch:** main
**Languages:** Rust, JavaScript, Shell, Markdown

## OVERVIEW
Skills monorepo — collection of AI agent skills for OpenCode. Contains a Rust CLI (fund-manager), userscripts, TTS scripts, and skill definitions.

## STRUCTURE
```
./
├── skills/
│   ├── fund-manager/  # Rust CLI: 基金持仓追踪 & 净值同步
│   ├── bili/          # Tampermonkey 视频下载脚本
│   ├── joke-learner/  # 喜剧解构创作引擎 (skill definition)
│   ├── trending/      # 热搜榜聚合 (skill definition)
│   ├── xiaomi-tts/    # MiMo TTS 语音合成 (skill definition)
│   └── mi10s-unlock/  # 小米 10S 解锁 BL + Magisk root (skill definition)
├── openspec/
│   ├── specs/         # OpenSpec 详细规格文档
│   └── changes/       # 进行中的变更
└── AGENTS.md / CLAUDE.md / GEMINI.md
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| 基金管理 CLI | `skills/fund-manager/` | ~9k SLOC, Rust + SQLite |
| tampermonkey 脚本 | `skills/bili/` | Single JS userscript |
| 热搜榜 | `skills/trending/` | Skill definition only |
| TTS | `skills/xiaomi-tts/` | Shell script skill |
| 小米解锁 BL / root | `skills/mi10s-unlock/` | Skill + scripts（探测 / 绑定绕过 / 跨平台解锁 / Magisk） |
| OpenSpec 文档 | `openspec/` | Specs + changes + archive |

## CONVENTIONS
- **Languages**: Rust (2024 edition + tokio), JS (tampermonkey), Shell
- **Build**: fund-manager via cargo
- **CLI**: Rust uses clap derive
- **Config**: TOML for fund-manager
- **Finances**: `rust_decimal::Decimal` (no floats), `YYYY-MM-DD` dates, 6-digit fund codes
- **User output**: Chinese (fund-manager)
- **Exit codes**: 0=success, 1=error, 3=confirm needed (fund-manager)
- **CI**: GitHub Actions release workflow for binary skills

## WORK GUIDELINES
- 不要轻易执行文件改动，在变更前先理解代码逻辑
- 不确定的逻辑和业务需求需要询问用户

## COMMANDS
```bash
cd skills/fund-manager && cargo build       # Build fund-manager
cd skills/fund-manager && cargo test         # Test fund-manager
```
