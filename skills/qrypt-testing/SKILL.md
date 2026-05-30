---
name: qrypt-testing
description: Validate qrypt FUSE mount correctness with real Quark API calls. Covers debounce timer, COW snapshot upload, rename/delete timer migration, conflict-free writes, and rapid concurrent operations. Use when testing qrypt on ~/Qrypt after code changes.
metadata:
  openclaw:
    requires:
      bins: [go, nohup]
---

# Qrypt FUSE Mount Testing

验证 qrypt 的真实挂载操作，确保修改后的代码在 Quark API 实际环境下正常工作。

## 构建与挂载

```bash
cd skills/qrypt
go build -o qrypt ./cmd/qrypt

# 清理旧状态
sudo umount ~/Qrypt 2>/dev/null
pkill -f "qrypt mount" 2>/dev/null
rm -f ~/.qrypt/qryptd.sock
sleep 2

# 挂载
nohup ./qrypt mount quark > /tmp/qrypt-mount.log 2>&1 &
sleep 8
ls ~/Qrypt/
# 预期: dist (或其他网盘文件)
```

## 日志

```bash
tail -f ~/.qrypt/qrypt.log
```

测试前清理日志:

```bash
echo "" > ~/.qrypt/qrypt.log
```

## 测试场景

### 1. Debounce Timer — 快速连续写入

验证 `write_back_timeout`（默认 5s）只触发一次上传:

```bash
for i in $(seq 1 20); do
  echo "write_${i}" > ~/Qrypt/race.txt
  sleep 0.3
done
sleep 8
tail -5 ~/.qrypt/qrypt.log | grep "syncFile"
# 预期: 只有 1 行 "syncFile: starting sync for /race.txt"
```

每 0.3s 写一次，持续 6s。timer 被不断重置，最终只触发一次上传。

### 2. 写入 + 重命名 — Timer 迁移

验证 `write → rename` 后 timer 迁移到新路径:

```bash
echo "content" > ~/Qrypt/migrate.txt
sleep 2
mv ~/Qrypt/migrate.txt ~/Qrypt/migrated.txt
sleep 8
tail -5 ~/.qrypt/qrypt.log | grep "syncFile"
# 预期: "syncFile: starting sync for /migrated.txt"（新路径）
cat ~/Qrypt/migrated.txt
# 预期: "content"
```

### 3. 写入 + 删除 — Timer 取消

```bash
echo "data" > ~/Qrypt/cancel.txt
sleep 2
rm ~/Qrypt/cancel.txt
sleep 8
tail -5 ~/.qrypt/qrypt.log | grep "syncFile"
# 预期: 没有 /cancel.txt 的 syncFile
ls ~/Qrypt/cancel.txt 2>&1
# 预期: "No such file"
```

### 4. 写入 → 上传完成 → 再次写入（最可能触发冲突的场景）

这是之前最容易出现 `[Local Conflict]` 和 `name(1)` 的场景:

```bash
for j in $(seq 1 3); do
  echo "v1_${j}" > ~/Qrypt/worstcase.txt
  sleep 6    # 等第一次上传完成
  for k in $(seq 1 10); do
    echo "v2_${j}_${k}" > ~/Qrypt/worstcase.txt
    ls ~/Qrypt/ > /dev/null    # 触发 MergeRemoteChanges
  done
  sleep 6
done
```

验证:

```bash
ls ~/Qrypt/*Conflict* ~/Qrypt/*\(1\)* 2>&1
# 预期: "No matches found"（无冲突文件）

tail -10 ~/.qrypt/qrypt.log | grep -E "syncFile|resolveConflict"
# 预期: 只有 syncFile 行，没有 resolveConflict 行

cat ~/Qrypt/worstcase.txt
# 预期: "v2_3_10"（最后一次写入的内容）
```

### 5. 重命名链 — 连续三次重命名

```bash
echo "final" > ~/Qrypt/a.txt
mv ~/Qrypt/a.txt ~/Qrypt/b.txt
mv ~/Qrypt/b.txt ~/Qrypt/c.txt
mv ~/Qrypt/c.txt ~/Qrypt/d.txt
sleep 8
tail -5 ~/.qrypt/qrypt.log | grep "syncFile"
# 预期: only "syncFile: starting sync for /d.txt"
cat ~/Qrypt/d.txt
# 预期: "final"
```

### 6. 写入 + 目录重命名 — 子文件 timer 迁移

```bash
mkdir ~/Qrypt/mydir
echo "nested" > ~/Qrypt/mydir/f.txt
sleep 2
mv ~/Qrypt/mydir ~/Qrypt/newdir
sleep 8
tail -5 ~/.qrypt/qrypt.log | grep "syncFile"
# 预期: "syncFile: starting sync for /newdir/f.txt"
cat ~/Qrypt/newdir/f.txt
# 预期: "nested"
```

### 7. 混合操作（高负载）

```bash
echo "" > ~/.qrypt/qrypt.log
for i in $(seq 1 5); do
  echo "round${i}" > ~/Qrypt/stress.txt
  ls ~/Qrypt/ > /dev/null
  mv ~/Qrypt/stress.txt ~/Qrypt/renamed.txt
  ls ~/Qrypt/ > /dev/null
  echo "round${i}_v2" > ~/Qrypt/renamed.txt
  ls ~/Qrypt/ > /dev/null
done
sleep 12
tail -10 ~/.qrypt/qrypt.log | grep -E "syncFile|resolveConflict"
ls ~/Qrypt/*Conflict* ~/Qrypt/*\(1\)* 2>&1
```

## 验证检查清单

| 检查项 | 命令 | 预期 |
|--------|------|------|
| 无 `[Local Conflict]` 文件 | `ls ~/Qrypt/*Conflict*` | `No matches found` |
| 无 `name(1)` 重复文件 | `ls ~/Qrypt/*\(1\)*` | `No matches found` |
| 无 `resolveConflict` 日志 | `grep resolveConflict ~/.qrypt/qrypt.log` | 无输出 |
| 无 `resource busy` | `grep "resource busy" /tmp/qrypt-mount.log` | 无输出（旧行为，新代码已去除） |
| 上传成功 | `grep "PostUpload" ~/.qrypt/qrypt.log` | 有 `replacing fid index` 行 |
| 内容正确 | `cat ~/Qrypt/<file>` | 最后一次写入的内容 |

## 清理

```bash
rm -f ~/Qrypt/race.txt ~/Qrypt/migrate.txt ~/Qrypt/migrated.txt ~/Qrypt/cancel.txt ~/Qrypt/worstcase.txt ~/Qrypt/a.txt ~/Qrypt/b.txt ~/Qrypt/c.txt ~/Qrypt/d.txt ~/Qrypt/stress.txt ~/Qrypt/renamed.txt
rm -rf ~/Qrypt/mydir ~/Qrypt/newdir 2>/dev/null
```

## 注意事项

- `write_back_timeout` 在 `qrypt.toml` 中配置（默认 `5s`），测试中的 `sleep` 时间需对应调整
- Quark API 可能有索引延迟（30s 窗口），新代码用 fid 直删避免依赖 List 目录
- 测试在真实网盘环境下运行，会实际创建和删除文件，不要在重要目录下测试
- 测试完成后确认 `ls ~/Qrypt/` 只含预期文件
