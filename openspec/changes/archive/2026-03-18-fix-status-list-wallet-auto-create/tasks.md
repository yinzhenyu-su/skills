## 1. 修改 `resolve_wallet_id()` 函数

- [x] 1.1 在 `resolve_wallet_id()` 中添加 `use inquire::Select;`
- [x] 1.2 修改无活跃钱包分支：检查是否有其他钱包
- [x] 1.3 如有其他钱包，添加 `inquire::Select` 交互式选择逻辑
- [x] 1.4 验证 `buy`、`sell`、`import` 命令仍正常工作（行为变更：有钱包无活跃时会询问选择）

## 2. CLI 定义修改

- [x] 2.1 给 `Status` 命令添加 `wallet: Option<String>` 字段（参考 `Buy` 命令）
- [x] 2.2 给 `FundCommands::List` 添加 `wallet: Option<String>` 字段

## 3. Main.rs 逻辑修改

- [x] 3.1 修改 `Commands::Status` 处理逻辑：使用 `resolve_wallet_id(&conn, wallet)`
- [x] 3.2 修改 `FundCommands::List` 处理逻辑：使用 `resolve_wallet_id(&conn, wallet)`

## 4. 验证

- [x] 4.1 运行 `cargo build` 确保编译通过
- [ ] 4.2 测试无任何钱包时执行 `fund status` 能自动创建"默认钱包"
- [ ] 4.3 测试有钱包但无活跃钱包时执行 `fund status` 能询问选择
- [ ] 4.4 测试 `fund status --wallet <name>` 能使用指定钱包
- [ ] 4.5 测试无任何钱包时执行 `fund list` 能自动创建"默认钱包"
- [ ] 4.6 测试有钱包但无活跃钱包时执行 `fund list` 能询问选择
- [ ] 4.7 测试 `fund list --wallet <name>` 能使用指定钱包
