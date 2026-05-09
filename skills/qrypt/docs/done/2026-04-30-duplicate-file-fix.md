# 2026-04-30 修复重复文件 (1) 后缀 — Release-only sync + FID 替换

## 问题

cp -r 上传 dist 目录到夸克网盘时，出现重复文件（带 `(1)` 后缀）。

### 根因

两个竞态叠加：

1. **mtime 竞态**：Write 触发 sync → 上传成功 → 但上传期间有新 Write → mtime 变化 → `isDirty` 不清除 → 不必要地重新入队
2. **API 索引延迟**：二次 sync 时 `deleteExistingFileByName` 调用 `ListFiles`，但夸克 API 还没索引到刚上传的文件 → 返回 0 个文件 → 跳过删除 → 创建新 placeholder → 夸克自动加 `(1)` 后缀

### 日志证据

```
15:15:49 syncFile: Sync OK for /dist/version.json: resultFid=aae65276... encSize=86
15:15:50 syncFile: /dist/version.json still dirty after sync, re-enqueuing...
15:15:50 deleteExistingFileByName: ListFiles returned 0 files in parent 3fbd75db...
         (应该找到 version.json 但找不到，因为 API 还没索引)
```

## 方案

### 核心改动

1. **Write 不触发 sync**：只写 staging，设置 isDirty=true
2. **Release 触发 sync**：文件关闭时一次性上传
3. **syncFile 无条件清除 isDirty**：上传成功即清除，不比较 mtime
4. **FID 直接替换**：node 记录 uploadedFid，re-upload 时用 FID 直接删除旧文件，不走 ListFiles

### 具体改动

#### types.go
- node 新增 `uploadedFid string` 字段

#### writeback.go
- Write: 删除 syncTimer/debounce 逻辑（第 119-125 行）
- Release: 简化，直接 `enqueueSync(node)`（已有逻辑基本不变）

#### sync.go
- 第 713 行: `if n.mtime.Equal(snapshotMtime)` → 改为无条件 `n.isDirty = false`
- 上传成功后: `n.uploadedFid = result.Fid`
- 上传前: 如果 `n.uploadedFid != ""`，先用 FID 删除旧文件（绕过 ListFiles）

#### upload/manager.go
- SyncRequest 新增 `OldFid string` 字段
- Sync(): 如果 OldFid 不为空，用 `driver.Delete([]string{OldFid})` 替代 `deleteExistingFileByName`

### 边界情况

| 场景 | 处理 |
|------|------|
| cp -r | 每个文件 Write→Release→sync 一次，无重复 |
| vim 编辑 | Write→Release→sync→再编辑→Release→sync，FID 替换 |
| 进程 SIGKILL | isDirty 已持久化，下次启动 recoverDirtyFiles |
| 大文件 | 等 Release 后整文件上传 |
| uploadedFid 为空（首次上传） | 退化为 deleteExistingFileByName |

## 状态

- [ ] 实现中
