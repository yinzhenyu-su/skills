# 2026-04-30 上传未完成时 Readdir 误删文件

## 问题描述

上传一个前端生成的 `dist` 目录（含多个文件），在整个上传没有完成时进入本地挂载的网盘 `dist` 目录，VFS 触发了 `dist` 目录的查询流程（Readdir → MergeRemoteChanges），导致尚未被夸克 API 索引到的已上传文件被误删。

## 根因分析

### 上传流程

1. `cp -r dist /mount-point/` → FUSE Create() 为每个文件创建 node：`source="local"`, `fid="local_xxx"`, `isDirty=true`
2. Release() → enqueueSync() → `syncQueued=true`, 进入上传队列
3. syncFile() 上传成功后：
   - `fid` 从 `local_xxx` 变为服务端真实 FID
   - **`source` 从 `"local"` 变为 `"remote"`**（sync.go line 641）
   - `isDirty` 变为 `false`
   - `syncQueued` 变为 `false`
   - `lastUploadTime` 设置为 `time.Now()`

### 问题时序

```
T=0s:   File A 开始上传
T=2s:   File A 上传完成 → source="remote", fid=real_fid
T=5s:   File B 开始上传
T=7s:   File B 上传完成 → source="remote", fid=real_fid
...
T=30s:  用户执行 ls /mount/dist/
T=31s:  Readdir → fetchFiles(dist_fid) → 远程列表
        夸克 API 索引延迟，File A 不在远程列表中
T=31s:  MergeRemoteChanges 检查 File A:
        - isDirty=false ✓ (已上传)
        - source="remote" → 不走 source 保护
        - fid 不以 local_ 开头 → 不走 fid 保护
        - lastUploadTime=29s前 → time.Since < 30s → 勉强保护
        但如果索引延迟 > 30s，或上传顺序导致间隔更长：
        - lastUploadTime=35s前 → time.Since >= 30s → 保护失效！
        - 判定为"远程已删除" → deleteNodePath() → 文件被误删 ✗
```

### 核心矛盾

`syncFile()` 在上传成功后立即将 `source` 设为 `"remote"`，但此时夸克 API 可能还没有索引到该文件。
MergeRemoteChanges 的 source 保护逻辑（line 805-820）只保护 `source=="local"` 或 `source=="merged"` 的文件，
对 `source=="remote"` 的文件仅依赖 30 秒时间窗口（`lastUploadTime`）。

**30 秒时间窗口在以下场景不够：**
- 文件数量多，上传队列长，第一个文件上传完成到最后一个文件上传完成间隔 > 30s
- 夸克 API 索引延迟 > 30s（大目录、高负载场景）

## 修复方案

### 核心思路

**不在 syncFile 中将 source 设为 "remote"**，而是延迟到 MergeRemoteChanges 在远程列表中确认该文件存在后才转为 "remote"。

### 修改内容

1. **sync.go line 641**: 删除 `n.source = "remote"`
   - 上传成功后，source 保持 "local"（Create 时设置的值）
   - 上传成功后，fid 从 local_xxx 变为真实 FID，expectedFid 也设置为真实 FID
   - lastUploadTime 正常设置

2. **MergeRemoteChanges 已有自动转换逻辑**（path_state.go line 792-801）：
   ```go
   if exists && rf.Fid == expectedFid {
       // ...
       n.source = "remote"  // ← 已有！确认后自动转换
   }
   ```
   当文件出现在远程列表且 FID 匹配时，source 自动从 "local" 转为 "remote"。

### 保护链分析

修复后的保护链（从强到弱）：

| 状态 | 保护机制 | 可靠性 |
|------|---------|--------|
| 正在写入（未 Release） | 不在 children 中，MergeRemoteChanges 看不到 | ✅ 绝对 |
| 已 Release，等待上传 | `syncQueued=true`（line 784） | ✅ 绝对 |
| 上传进行中 | `syncQueued=true` | ✅ 绝对 |
| 上传完成，API 未索引 | `source="local"`（line 805/828） | ✅ 绝对（新增） |
| 上传完成，API 已索引 | `source="remote"`（由 MergeRemoteChanges 设置） | ✅ 绝对 |
| 远程确实删除 | `source="remote"` + 不在远程列表 + 30s 已过 | ✅ 正确删除 |

### 副作用分析

1. **正常场景**：文件上传 → MergeRemoteChanges 确认 → source 转为 "remote"，行为无变化
2. **服务端删除**：文件上传完成 + API 索引 + 用户从网页删除 → MergeRemoteChanges 检测到不在列表 → 删除本地节点，行为正确
3. **边界情况**：文件上传完成 + API 索引 + 用户从网页删除 + 在同一 Readdir 周期内 → source 仍为 "local" → 不删除 → 下次 Readdir 时 source 已转为 "remote" → 正确删除。延迟一个周期可接受。

## 测试方案

修改 `merge_upload_delay_test.go`，新增测试用例模拟：
- 文件上传完成（source="local", fid=real_fid, expectedFid=real_fid, isDirty=false）
- 远程列表为空（API 未索引）
- 验证文件不被误删
- 验证当远程列表包含该文件时，source 自动转为 "remote"
