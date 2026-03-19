# 任务：统一 CLI 错误信息格式

## 1. 修改 CLI 参数定义（src/cli.rs）

- [x] 清理所有命令中关于 `（省略则交互式选择）` 的注释，改为 `（省略将列出可用基金并提示）`
- [x] `history` 的 `fund: String` → `fund: Option<String>`
- [x] `FundCommands::Delete` 的 `fund: String` → `fund: Option<String>`
- [x] `FundCommands::Inspect` 的 `fund: String` → `fund: Option<String>`
- [x] `Buy` 的 `fund: String` → `fund: Option<String>`
- [x] `Sell` 的 `fund: String` → `fund: Option<String>`
- [x] `Buy` 的 `money: Decimal` → `money: Option<Decimal>`

## 2. 增强 fund 缺失检查函数（src/main.rs）

- [x] 创建/修改 `require_fund_or_exit(conn, fund_option, wallet_id, subcommand)` 函数
  - fund 存在时直接返回
  - fund 缺失时，根据 `subcommand` 决定列出哪些基金（如 sell 只列出有持仓的基金，其余列出所有追踪基金）
  - 打印包含可用基金列表和用法示例的友好错误信息，然后 exit(1)

## 3. 统一错误信息格式（src/main.rs）

- [x] 所有 `eprintln!("❌ 错误：", e)` 改为 `eprintln!("❌ {}", e)`（去掉冗余的"错误："）
- [x] 统一引号为单引号 `'{}'`
- [x] 消除括号注释 `"（执行过程失败）"`
- [x] 给缺少上下文的错误补充用法示例

## 4. 消除 panic（src/main.rs）

- [x] Sell handler 中所有 `.expect()` 替换为 `eprintln!` + `exit(1)`
- [x] Buy handler 中所有 `.expect()` 替换为 `eprintln!` + `exit(1)`
- [x] 保留 `init_db`、`open_conn` 等初始化阶段的 `.expect()`（启动失败可以 panic）

## 5. 适配 handler 入口（src/main.rs）

- [x] `history` handler 添加 fund 缺失检查
- [x] `fund delete` handler 添加 fund 缺失检查
- [x] `fund inspect` handler 添加 fund 缺失检查
- [x] `buy` handler 添加 fund/money 缺失检查
- [x] `sell` handler 添加 fund 缺失检查

## 6. 更新测试

- [x] 运行 `cargo test`，修复因错误信息变化而失败的测试
- [x] 验证 clap 不再输出英文错误
