---
name: qrypt-testing
description: Validate qrypt FUSE mount correctness with real Quark API calls. Covers debounce timer, COW snapshot upload, rename/delete timer migration, conflict-free writes, and rapid concurrent operations. Use when testing qrypt on ~/Qrypt after code changes.
metadata:
  openclaw:
    requires:
      bins: [go, nohup]
---

# Qrypt FUSE Mount Stress Testing

验证 qrypt FUSE 挂载在高负载、并发、边界条件下的正确性。
所有测试在真实 Quark API 环境下运行，覆盖 timer、COW snapshot、并发写入、文件操作竞态等核心路径。

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

# 日志
tail -f ~/.qrypt/qrypt.log
```

测试前清理日志:

```bash
echo "" > ~/.qrypt/qrypt.log
```

---

## 测试场景

### 1. Debounce Timer — 快速连续写入

验证 `write_back_timeout`（默认 5s）只触发一次上传。

```bash
for i in $(seq 1 20); do
  echo "write_${i}" > ~/Qrypt/race.txt
  sleep 0.3
done
sleep 8
grep -c "syncFile: starting sync for /race.txt" ~/.qrypt/qrypt.log
# 预期: 1
cat ~/Qrypt/race.txt
# 预期: "write_20"
```

每 0.3s 写一次，持续 6s。timer 被不断重置，最终只触发一次上传。确认最终内容是最后一次写入的。

---

### 2. 同一文件连续编辑 — 模拟编辑器行为

核心场景：编辑器连续保存同一文件（如 IDE 自动保存、vim 的 backup copy）。

#### 2a. 快速连续保存（间隔 < timer）

每次保存都在 timer 窗口内，timer 被不断重置，只触发一次上传。

```bash
echo "" > ~/.qrypt/qrypt.log
for i in $(seq 1 30); do
  echo "edit_${i}_$(date +%s%N)" > ~/Qrypt/continuous_edit.txt
  sleep 0.5
done
sleep 8
grep -c "syncFile: starting sync for /continuous_edit.txt" ~/.qrypt/qrypt.log
# 预期: 1
cat ~/Qrypt/continuous_edit.txt
# 预期: "edit_30_..."（最后一次编辑内容）
```

#### 2b. 慢速 + 快速交替（跨越 timer 边界）

编辑节奏跨越 timer 触发点：部分编辑被合并，部分触发新 upload。

```bash
echo "=== Phase 1: rapid edits ==="
for i in $(seq 1 5); do
  echo "rapid_${i}" > ~/Qrypt/mixed_edit.txt
  sleep 0.3
done
echo "=== Wait for timer ==="
sleep 6  # timer 触发，上传 "rapid_5"
echo "=== Phase 2: slow edits ==="
for i in $(seq 1 3); do
  echo "slow_${i}" > ~/Qrypt/mixed_edit.txt
  sleep 2
done
echo "=== Wait for timer ==="
sleep 6  # timer 触发，上传 "slow_3"
echo "=== Phase 3: rapid again ==="
for i in $(seq 1 5); do
  echo "rapid2_${i}" > ~/Qrypt/mixed_edit.txt
  sleep 0.3
done
sleep 8

grep -c "syncFile: starting sync for /mixed_edit.txt" ~/.qrypt/qrypt.log
# 预期: 3（phase1→timer→phase2→timer→phase3→timer）
# 注意: 旧版 EBUSY 会让 phase2 写入失败，新版 COW 保证全部成功
cat ~/Qrypt/mixed_edit.txt
# 预期: "rapid2_5"
```

#### 2c. 编辑 + 读取验证（数据一致性）

写入后立即读取，确认不会读到中间状态。

```bash
for i in $(seq 1 20); do
  content="consistent_write_${i}_$(date +%s%N)"
  echo "$content" > ~/Qrypt/consistency.txt
  read_back=$(cat ~/Qrypt/consistency.txt)
  if [ "$content" != "$read_back" ]; then
    echo "MISMATCH at iteration $i: wrote '$content' read '$read_back'"
    break
  fi
done
echo "Done 20 iterations without mismatch"
# 预期: 无 MISMATCH
```

#### 2d. 编辑 + 删除 + 重建同名文件

模拟用户：打开文件 → 写 → 删 → 再创建同名文件。

```bash
for cycle in $(seq 1 5); do
  echo "cycle_${cycle}_v1" > ~/Qrypt/recreate.txt
  sleep 1
  rm ~/Qrypt/recreate.txt
  sleep 0.5
  echo "cycle_${cycle}_v2" > ~/Qrypt/recreate.txt
  sleep 1
done
sleep 10
ls ~/Qrypt/*Conflict* ~/Qrypt/*\(1\)* 2>&1
# 预期: "No matches found"
cat ~/Qrypt/recreate.txt
# 预期: "cycle_5_v2"
```

#### 2e. 编辑期间并发读 + ls

模拟用户：编辑文件的同时其他进程在读取目录。

```bash
for i in $(seq 1 10); do
  echo "concurrent_${i}" > ~/Qrypt/concurrent_edit.txt
  # 并发触发读操作
  cat ~/Qrypt/concurrent_edit.txt > /dev/null &
  ls ~/Qrypt/ > /dev/null &
  ls -la ~/Qrypt/concurrent_edit.txt > /dev/null &
  wait
  sleep 0.5
done
sleep 8
grep -c "syncFile: starting sync for /concurrent_edit.txt" ~/.qrypt/qrypt.log
# 预期: 1 或 2（取决于是否跨越 timer 边界）
ls ~/Qrypt/*Conflict* 2>&1
# 预期: "No matches found"


---

### 3. 写入 → 上传完成 → 再次写入 + 并发 ls（旧版最易触发 conflict 的场景）

```bash
for j in $(seq 1 5); do
  echo "v1_${j}" > ~/Qrypt/cycle_test.txt
  sleep 6
  for k in $(seq 1 10); do
    echo "v2_${j}_${k}" > ~/Qrypt/cycle_test.txt
    ls ~/Qrypt/ > /dev/null &
    ls ~/Qrypt/cycle_test.txt > /dev/null &
  done
  wait
  sleep 6
done

ls ~/Qrypt/*Conflict* ~/Qrypt/*\(1\)* 2>&1
# 预期: "No matches found"
grep -c resolveConflict ~/.qrypt/qrypt.log
# 预期: 0
```

5 轮 × 10 次快速写入 + 并发 ls 触发 MergeRemoteChanges。
验证无 `[Local Conflict]`、无 `name(1)`、无 `resolveConflict`。

---

### 4. 重命名链 — 连续三次重命名

```bash
echo "final" > ~/Qrypt/rename_chain.txt
mv ~/Qrypt/rename_chain.txt ~/Qrypt/rc_a.txt
mv ~/Qrypt/rc_a.txt ~/Qrypt/rc_b.txt
mv ~/Qrypt/rc_b.txt ~/Qrypt/rc_c.txt
sleep 8
grep -c "syncFile: starting sync for /rc_c.txt" ~/.qrypt/qrypt.log
# 预期: 1
grep "syncFile: starting sync" ~/.qrypt/qrypt.log | grep rc_
# 预期: 只有 /rc_c.txt 的 syncFile
cat ~/Qrypt/rc_c.txt
# 预期: "final"
```

---

### 5. 写入 + 删除 — Timer 取消

```bash
echo "cancel_me" > ~/Qrypt/cancel_me.txt
sleep 2
rm ~/Qrypt/cancel_me.txt
sleep 8
grep -c "syncFile: starting sync for /cancel_me.txt" ~/.qrypt/qrypt.log
# 预期: 0
ls ~/Qrypt/cancel_me.txt 2>&1
# 预期: "No such file"
```

---

### 6. 写入 + 目录重命名 — 子文件 timer 迁移

```bash
mkdir -p ~/Qrypt/nested_dir
echo "nested content" > ~/Qrypt/nested_dir/f.txt
sleep 2
mv ~/Qrypt/nested_dir ~/Qrypt/renamed_dir
sleep 8
grep -c "syncFile: starting sync for /renamed_dir/f.txt" ~/.qrypt/qrypt.log
# 预期: 1
grep "syncFile: starting sync for /nested_dir/f.txt" ~/.qrypt/qrypt.log
# 预期: 无输出
cat ~/Qrypt/renamed_dir/f.txt
# 预期: "nested content"
```

---

### 7. 多文件并发写入

验证多个文件同时写入时各自独立触发 timer，互不干扰。

```bash
for i in $(seq 1 10); do
  (
    echo "concurrent_file_${i}" > ~/Qrypt/conc_${i}.txt
  ) &
done
wait
sleep 8
for i in $(seq 1 10); do
  grep -c "syncFile: starting sync for /conc_${i}.txt" ~/.qrypt/qrypt.log
done
# 预期: 每行 1
for i in $(seq 1 10); do
  cat ~/Qrypt/conc_${i}.txt
done
# 预期: "concurrent_file_${i}"
```

---

### 8. 大文件分块上传

```bash
dd if=/dev/urandom of=/tmp/qrypt_large_test.bin bs=1M count=20 2>/dev/null
cp /tmp/qrypt_large_test.bin ~/Qrypt/large.bin
sleep 15
grep -c "PostUpload.*large.bin" ~/.qrypt/qrypt.log
# 预期: 1
diff <(xxd ~/Qrypt/large.bin) <(xxd /tmp/qrypt_large_test.bin)
# 预期: 无差异
rm /tmp/qrypt_large_test.bin
```

---

### 9. 极端操作竞态 — write / rename / delete / recreate 循环

```bash
for i in $(seq 1 20); do
  echo "content_${i}" > ~/Qrypt/race_dir/race_file_${i}.txt 2>/dev/null || mkdir -p ~/Qrypt/race_dir && echo "content_${i}" > ~/Qrypt/race_dir/race_file_${i}.txt
done
sleep 1
for i in $(seq 1 20); do
  mv ~/Qrypt/race_dir/race_file_${i}.txt ~/Qrypt/race_dir/moved_${i}.txt 2>/dev/null
done
sleep 1
for i in $(seq 1 20); do
  rm ~/Qrypt/race_dir/moved_${i}.txt 2>/dev/null
done
sleep 1
for i in $(seq 1 20); do
  echo "recreated_${i}" > ~/Qrypt/race_dir/recreated_${i}.txt 2>/dev/null || mkdir -p ~/Qrypt/race_dir && echo "recreated_${i}" > ~/Qrypt/race_dir/recreated_${i}.txt
done
sleep 10
ls ~/Qrypt/race_dir/ | wc -l
# 预期: 20（recreated 文件）
ls ~/Qrypt/*Conflict* 2>&1
# 预期: "No matches found"
rm -rf ~/Qrypt/race_dir
```

---

### 10. 混合操作（高负载综合场景）

```bash
echo "" > ~/.qrypt/qrypt.log
for i in $(seq 1 10); do
  echo "round${i}" > ~/Qrypt/mixed_write.txt
  ls ~/Qrypt/ > /dev/null
  mv ~/Qrypt/mixed_write.txt ~/Qrypt/mixed_renamed.txt 2>/dev/null
  ls ~/Qrypt/ > /dev/null
  echo "round${i}_v2" > ~/Qrypt/mixed_renamed.txt
  ls ~/Qrypt/mixed_renamed.txt > /dev/null &
  ls ~/Qrypt/ > /dev/null &
  wait
done
sleep 15
grep -c "syncFile: starting sync" ~/.qrypt/qrypt.log
# 预期: 1（所有操作被 timer 合并为一次上传）
tail -5 ~/.qrypt/qrypt.log | grep "PostUpload"
cat ~/Qrypt/mixed_renamed.txt
# 预期: "round10_v2"
```

---

### 11. 挂载状态保持 — 长时间运行

```bash
for i in $(seq 1 60); do
  echo "heartbeat_${i}" > ~/Qrypt/heartbeat.txt
  ls ~/Qrypt/ > /dev/null
  sleep 2
done
grep -c "syncFile: starting sync for /heartbeat.txt" ~/.qrypt/qrypt.log
# 预期: ~12（60次写入 ÷ 5s timer ≈ 12次上传）
grep -E "error|panic|shutdown" ~/.qrypt/qrypt.log
# 预期: 无
```

---

### 12. 内容一致性 — 随机写入模式

模拟真实编辑器行为：多次局部写入 + 读取验证。

```bash
python3 -c "
import os, time
path = os.path.expanduser('~/Qrypt/random_write.txt')

# 多轮写入，模拟编辑器
blocks = [b'A'*100, b'B'*200, b'C'*300, b'D'*400, b'E'*500]
with open(path, 'wb') as f:
    f.write(b'initial_content')
time.sleep(2)

for i, block in enumerate(blocks):
    with open(path, 'r+b') as f:
        f.seek(0)
        f.write(block)
    time.sleep(0.5)

time.sleep(10)

with open(path, 'rb') as f:
    content = f.read()
assert content.startswith(b'E'*500), f'Content mismatch, got {len(content)} bytes'
print('Content OK:', len(content), 'bytes')
"
```

---

## 验证检查清单

每次测试完成后使用以下清单确认：

| 检查项 | 命令 | 预期 |
|--------|------|------|
| 无 `[Local Conflict]` 文件 | `ls ~/Qrypt/*Conflict*` | `No matches found` |
| 无 `name(1)` 重复文件 | `ls ~/Qrypt/*\(1\)*` | `No matches found` |
| 无 `resolveConflict` 日志 | `grep resolveConflict ~/.qrypt/qrypt.log` | 无输出 |
| 无 panic | `grep -i panic ~/.qrypt/qrypt.log` | 无输出 |
| 无上传错误 | `grep "upload failed\|upload error" ~/.qrypt/qrypt.log` | 无输出 |
| 上传成功计数 | `grep -c "PostUpload" ~/.qrypt/qrypt.log` | ≥ 预期次数 |
| 内容正确 | `cat ~/Qrypt/test_file.txt` | 最后一次写入的内容 |
| 无孤儿 snapshot 文件 | `ls ~/.qrypt/cache/quark/staging/*.snap 2>&1` | `No matches found` |

## 一次性快速验证

```bash
echo "" > ~/.qrypt/qrypt.log
echo "=== 1. Debounce ===" && for i in $(seq 1 10); do echo "w${i}" > ~/Qrypt/quick.txt; sleep 0.2; done && \
echo "=== 2. Rename ===" && mv ~/Qrypt/quick.txt ~/Qrypt/quick_renamed.txt && \
echo "=== 3. Delete ===" && rm -f ~/Qrypt/quick_renamed.txt && \
echo "=== 4. Wait ===" && sleep 10 && \
echo "=== Result ===" && grep "syncFile" ~/.qrypt/qrypt.log | grep -v "PostUpload" && \
ls ~/Qrypt/*Conflict* ~/Qrypt/*\(1\)* 2>&1 || true && \
echo "=== Clean ===" && echo "done"
```

## 清理

```bash
rm -f ~/Qrypt/*.txt ~/Qrypt/*.bin ~/Qrypt/*Conflict*
rm -rf ~/Qrypt/race_dir 2>/dev/null
rm -f ~/.qrypt/cache/quark/staging/*.snap 2>/dev/null
```

## 注意事项

- `write_back_timeout` 在 `qrypt.toml` 中配置（默认 `5s`），测试中的 `sleep` 时间需对应调整（默认 5s 的 timer 需 sleep ≥ 6s）
- 所有测试在真实网盘环境下运行，会实际创建和删除文件，不要在包含重要数据的目录下测试
- 测试完成后确认 `ls ~/Qrypt/` 只含预期文件（通常是 `dist`）
- 如果测试失败，先检查 `~/.qrypt/qrypt.log` 中的错误日志，再检查 `/tmp/qrypt-mount.log` 中的进程输出
