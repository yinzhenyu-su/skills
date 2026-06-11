# 文件操作合并与抵消设计指南

## 背景

Finder、IDE、构建工具在 FUSE 挂载点上操作目录时，通常不会只发起一个语义明确的高层操作。一个用户动作可能被拆成多次 `Create`、`Write`、`Release`、`Rename`、`Unlink`、`Rmdir` 和目录探测。

典型问题：

- 写入临时文件后 rename 成目标文件，导致临时文件和目标文件都排队上传。
- 创建文件后短时间内删除，仍然上传了无意义文件。
- 上传还未开始或正在排队时 rename，旧路径和新路径产生重复远端对象。
- 删除目录时 Finder 先删子文件再删目录，导致远端接口调用放大。

本指南定义一层操作合并抽象，用于在短时间窗口内把低层 FUSE 事件归并成更接近用户意图的远端操作。

## 目标

1. 减少无效远端接口调用。
2. 避免 create/upload/rename/delete 竞态造成重复文件或重复目录。
3. 保证本地可见性优先：FUSE 调用成功后，本地树立即反映结果。
4. 保证数据不丢：只抵消远端操作，不丢弃仍需上传的最新内容。
5. 保持失败可恢复：进程崩溃后 pending journal 能恢复未完成的最终意图。

## 非目标

- 不改变 FUSE 同步语义。FUSE handler 仍应尽快返回。
- 不等待远端接口完成后才更新本地树。
- 不把所有操作无限期延迟。合并窗口必须有上限。
- 不依赖 Finder 特定行为。规则应适用于 CLI、IDE、构建工具。

## 核心抽象

引入一个 `OperationCoalescer`，作为 FUSE handler 与后台 worker 之间的缓冲层。

```
FUSE handler
  │
  │ local tree update + staging update
  ▼
OperationCoalescer
  │  归并、抵消、改写远端意图
  ▼
metadata/upload workers
  │
  ▼
driver API
```

FUSE handler 只负责：

- 修改内存树，使 Finder 立即看到结果。
- 写入 staging，保证数据落本地。
- 提交一条结构化操作事件。

Coalescer 负责：

- 在时间窗口内收集相关事件。
- 将事件归并为最少的远端操作。
- 维护 path、fid、staging、upload task 之间的状态映射。
- 在窗口结束或强制 flush 时输出最终远端意图。

## 事件模型

每个事件必须带足够的信息，不能只靠字符串路径推断。

```go
type OpKind string

const (
    OpCreate OpKind = "CREATE"
    OpWrite  OpKind = "WRITE"
    OpUpload OpKind = "UPLOAD"
    OpRename OpKind = "RENAME"
    OpDelete OpKind = "DELETE"
)

type FileOp struct {
    Kind       OpKind
    Path       string
    NewPath    string
    Fid        string
    ParentFid  string
    IsDir      bool
    Staging    string
    Generation uint64
    Time       time.Time
}
```

关键字段：

| 字段 | 用途 |
|------|------|
| `Fid` | 识别同一个节点，避免 rename 后只靠旧路径找不到任务 |
| `Path/NewPath` | 表示路径迁移 |
| `Staging` | 指向待上传内容 |
| `Generation` | 区分同一路径的不同内容版本 |
| `IsDir` | 区分文件上传和目录元数据操作 |

## 不变量

### I1. 本地状态立即生效

`Create`、`Rename`、`Delete` 在 FUSE handler 内先更新本地树，再提交后台意图。Coalescer 不能阻塞 Finder 等待远端。

### I2. 一个节点最多一个有效上传意图

同一 `Fid` 或同一逻辑节点在任意时刻只能有一个最新 `UPLOAD` 意图。新的 `Write/Release` 只能替换旧 generation，不能追加并发上传。

### I3. Rename 转移意图，不复制意图

rename 发生后，旧路径上的 pending upload 必须迁移到新路径。不能保留旧路径 upload 再新增新路径 upload。

### I4. Delete 可以吸收子操作

目录 delete 可以吸收其子路径的 create、rename、upload、delete。只要父目录最终被删除，子路径远端操作都不应发出。

### I5. 抵消只作用于远端意图

如果文件已经写入 staging，抵消 upload 时可以清理 staging，但必须确认该 staging 不再被任何新 generation 引用。

## 合并窗口

建议使用短窗口：

| 类型 | 默认窗口 | 目的 |
|------|----------|------|
| 文件 create/write/upload | 200ms | 吸收编辑器连续写入 |
| rename/create/upload | 600ms | 吸收临时文件 rename 到目标文件 |
| delete family | 600ms | 吸收 Finder 子文件删除 + 目录删除 |
| 删除 tombstone TTL | 10min | 吸收窗口外迟到的子删除 |

窗口是性能参数，不是正确性前提。即使事件超过窗口，也必须通过 tombstone、generation 和 active task 检查保持正确。

## 抵消规则

### 1. Create 后 Delete

```
CREATE /a.txt
WRITE  /a.txt
UPLOAD /a.txt
DELETE /a.txt
```

如果 upload 尚未开始：

- 取消远端 upload。
- 删除本地 staging。
- 不调用远端 delete，因为远端从未存在。

如果 upload 已经开始：

- 不强杀正在执行的 driver 调用。
- 标记该 generation 为 deleted。
- 上传成功后立即补发 delete，或如果 driver 支持 context cancellation，则取消上传并清理残留。

### 2. Create 后 Rename

```
CREATE /tmp/file.tmp
WRITE  /tmp/file.tmp
UPLOAD /tmp/file.tmp
RENAME /tmp/file.tmp -> /dist/file.js
```

如果 upload 尚未开始：

- 将 upload 意图迁移到 `/dist/file.js`。
- 使用新 parent fid 和新加密文件名上传。
- 不为 `/tmp/file.tmp` 发起远端 create/upload。

如果 upload 已经开始：

- 若远端尚未 commit，优先取消旧 upload，并以新路径重新排队。
- 若已经 commit，输出 `remote rename` 或 `upload new + delete old`，取决于 driver 能力。

### 3. Rename 后 Rename

```
RENAME /a -> /b
RENAME /b -> /c
```

合并为：

```
RENAME /a -> /c
```

中间路径 `/b` 只更新本地树，不产生远端调用。

### 4. Rename 后 Delete

```
RENAME /a -> /b
DELETE /b
```

如果 `/a` 已存在远端：

- 不需要远端 rename。
- 只对原 fid 发起 delete。

如果 `/a` 是本地新建且未上传：

- 抵消 rename 和 upload。
- 清理 staging。
- 不调用远端。

### 5. Delete 后 Create 同路径

```
DELETE /a.txt
CREATE /a.txt
WRITE  /a.txt
UPLOAD /a.txt
```

这是替换文件，不是抵消。

要求：

- delete 绑定旧 fid。
- upload 绑定新 generation。
- 两者可以合并为 `replace by name`，但不能丢掉 delete 语义。
- 如果 driver 上传前会按同名删除旧文件，需要确认旧 fid delete 已被覆盖，否则仍保留 tombstone 直到远端列表确认。

### 6. Delete 目录后子操作迟到

```
DELETE /dist
...窗口结束，远端目录 delete 已排队...
DELETE /dist/a.js
DELETE /dist/sub
```

处理规则：

- `DELETE /dist` 建立 path tombstone。
- tombstone TTL 内所有 `/dist/...` 操作都被吸收为本地清理。
- 不再为子路径发起远端 delete、upload、rename。
- 远端列表确认 `/dist` 消失后，tombstone 标记为 done，但保留到 TTL 结束，吸收 Finder 迟到调用。

### 7. Upload 后 Rename 迟到

```
UPLOAD /a.txt generation=7 已开始
RENAME /a.txt -> /b.txt
```

处理规则：

- 如果 upload 未 commit：取消旧上传，迁移到新路径。
- 如果 upload 已 commit：发起远端 rename。
- 如果 driver 不支持 rename：上传新路径，再删除旧 fid。
- 如果后续又 delete `/b.txt`：跳过 rename，只删除旧 fid 或取消新 upload。

## 状态表

建议维护两个索引。

### Node Intent Index

按逻辑节点或 fid 索引：

```go
type PendingIntent struct {
    Fid          string
    Path         string
    IsDir        bool
    Generation   uint64
    Upload       *UploadIntent
    RenameFrom   string
    Delete       bool
    RemoteExists bool
    Started      bool
    Committed    bool
}
```

用途：

- 找到 rename 前路径上的 pending upload。
- 判断 delete 是否可以抵消 upload。
- 防止同一 fid 产生多个远端操作。

### Path Tombstone Index

按路径前缀索引：

```go
type PathTombstone struct {
    Path      string
    Recursive bool
    ExpiresAt time.Time
    Done      bool
}
```

用途：

- 吸收删除目录后的迟到子操作。
- 阻止 refresh/readdir 把正在删除的远端子项重新合并回本地树。

## Flush 策略

触发 flush 的条件：

1. 合并窗口到期。
2. 队列长度达到上限。
3. 收到依赖远端结果的操作，例如需要确认 parent fid。
4. unmount/shutdown。
5. 手动 `qrypt push`。

flush 输出的是远端意图，不是原始 FUSE 事件：

```go
type RemoteIntent struct {
    Kind      string // UPLOAD, DELETE, RENAME, MKDIR
    Path      string
    NewPath   string
    Fid       string
    ParentFid string
    Staging   string
}
```

## 失败恢复

### Journal 记录最终意图

pending journal 应记录 coalescer 输出后的最终意图，而不是所有原始 FUSE 事件。

原因：

- 原始事件恢复后重放容易再次触发竞态。
- 最终意图更小，更接近远端需要执行的动作。

### 崩溃场景

| 崩溃点 | 恢复策略 |
|--------|----------|
| 窗口内崩溃 | 从本地 tree/staging 扫描重建 upload intent |
| upload 前崩溃 | journal 中的 upload 继续执行 |
| upload commit 后、状态更新前崩溃 | 通过 parent listing 按 fid/name 修正 |
| delete API 成功后崩溃 | tombstone 保留，listing 确认后清理 |
| rename API 成功后崩溃 | listing 确认新路径，旧路径 tombstone 清理 |

## Driver 能力差异

不同网盘能力不同，coalescer 不应假设所有 driver 都支持原子 rename 或批量 delete。

建议抽象能力：

```go
type Capabilities struct {
    BatchDelete bool
    Rename      bool
    Move        bool
    CancelUpload bool
}
```

降级策略：

| 能力缺失 | 替代方案 |
|----------|----------|
| 无 rename | upload new + delete old |
| 无 batch delete | postorder delete |
| 无 cancel upload | commit 后补偿 delete |
| 无 move directory | mkdir new + move/upload children + delete old |

## 日志要求

每次 flush 必须记录：

- 输入事件数量。
- 被抵消的 create/upload/delete 数量。
- 被 rename 迁移的 upload 数量。
- 输出远端意图数量。
- 每个远端 intent 的 kind、path、fid、generation。

示例：

```
coalescer: flush 12 ops -> 2 remote intents, canceled=6 renamed_uploads=1 absorbed_by_delete=3
coalescer: intent UPLOAD path=/dist/app.js fid=local_x generation=8
coalescer: intent DELETE path=/dist-old fid=Fxxx recursive=true
```

## 测试准则

必须覆盖以下场景：

1. `create -> write -> delete` 不调用远端。
2. `create -> write -> rename -> upload` 只上传新路径。
3. `rename -> rename` 只发一次最终 rename。
4. `rename -> delete` 不发 rename，只发 delete 或本地清理。
5. `delete directory -> late child delete` 被 tombstone 吸收。
6. `delete old -> create same path` 不抵消，保留替换语义。
7. upload 已开始后 rename，按 driver capability 选择 cancel、rename 或补偿。
8. 进程崩溃后 journal 只恢复最终意图，不恢复已抵消事件。

测试应使用 fake driver 计数远端调用，断言调用数量和顺序，而不是只断言本地树状态。

## 实施顺序

建议分阶段实现：

1. 先将现有 metadata delete coalescing 抽出为通用 coalescer 框架。
2. 接入 file upload intent，支持 create/delete 抵消。
3. 接入 rename，支持 pending upload 路径迁移。
4. 增加 started/committed 状态，处理上传已开始后的 rename/delete。
5. 将最终意图写入 journal，补齐崩溃恢复。
6. 根据 driver capability 优化 rename、batch delete、cancel upload。

每一步都必须保持当前 FUSE handler 的快速返回特性，不能为了合并操作阻塞 Finder。
