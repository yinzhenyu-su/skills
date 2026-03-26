## Context

fund-manager 使用 SQLite 数据库存储所有个人数据（钱包、基金、交易历史、净值、配置）。用户有时需要清除所有数据重新开始，目前只能逐个删除，缺乏统一入口。

## Goals / Non-Goals

**Goals:**
- 提供 `fund-manager reset` 命令，一键清除所有个人数据
- 保留数据库表结构（schema），下次启动自动重建
- 操作不可逆，需交互确认

**Non-Goals:**
- 不支持选择性清除（保留某基金等）
- 不支持导出备份
- 不保留任何个人数据

## Decisions

### 1. 数据库清除策略

直接删除数据库文件后重建，而非逐表清空。

**选择：删除数据库文件 + 重新 init**

- **优势**：干净利落，不留残余数据；代码简单，无需逐表编写 DELETE 语句
- **替代方案**：逐表 TRUNCATE/DELETE — 繁琐且容易遗漏新增表

实现：
```rust
fn reset_all_data(conn: &Connection, db_path: &Path) -> Result<()> {
    drop(conn);
    std::fs::remove_file(db_path)?;
    db::init_db(db_path)?;
    Ok(())
}
```

### 2. 命令层级

放在 `Commands::Reset` 下，即 `fund-manager reset`。

**理由**：与 `status`、`history` 等顶层命令平级，用户操作路径更直接。

### 3. 确认交互

使用已有的 `confirm_action()` 确认函数，提示语：
> "确定要重置所有数据吗？这将永久删除所有钱包、基金、交易历史、净值记录和配置，且无法恢复！"

全局 `-y` 参数可跳过确认。

## Risks / Trade-offs

- **风险**：用户误操作 — 缓解：强制确认提示
- **风险**：数据库文件被占用时删除失败 — 需确保 `conn` drop 后再删文件
