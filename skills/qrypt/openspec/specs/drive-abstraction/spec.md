# Drive Abstraction Specification

## Purpose

Define the `drive.Driver` interface and unified `Entry` type for abstracting storage backends (Quark, Yun139, etc.) behind a common API consumed by the FUSE layer.

## Requirements

### Requirement: Driver Interface Definition
系统 SHALL 定义一个 `drive.Driver` 接口，采用组合接口模式，包含 `Meta`、`Reader`、`Writer`、`Uploader` 四个子接口。所有文件系统操作通过该接口表达，具体存储后端通过实现该接口接入。

#### Scenario: 最小化实现
- **WHEN** 一个存储后端只需要只读能力
- **THEN** 它 SHALL 只实现 `Meta` + `Reader` 接口
- **AND** `QryptFS` SHALL 通过类型断言检查 `Writer` / `Uploader` 是否存在，若不存在则禁用对应操作

#### Scenario: 读写实现
- **WHEN** 一个存储后端支持读写
- **THEN** 它 SHALL 同时实现 `Meta` + `Reader` + `Writer` + `Uploader`

### Requirement: 通用 Entry 类型
系统 SHALL 定义统一的 `Entry` 结构体，作为所有 driver 的 List/Read/Write 操作返回的文件描述类型。

#### Scenario: Entry 包含字段
- **WHEN** 任意 driver 返回文件条目
- **THEN** Entry SHALL 包含 ID、Name、IsDir、Size、ModTime 字段
- **AND** Entry MAY 包含 Extra 字段用于透传 driver 特有元数据

### Requirement: Driver 接入点
系统 SHALL 支持通过依赖注入的方式将 driver 实例传入 FUSE 层，不需要注册表或全局状态。

#### Scenario: QryptFS 构造
- **WHEN** 创建 `QryptFS` 实例
- **THEN** 构造函数 SHALL 接收 `drive.Driver` 接口参数
- **AND** 不直接依赖任何具体 driver 类型

### Requirement: List 操作
所有 driver SHALL 支持通过 `List(ctx, parentID)` 返回目录下的文件和子目录列表。

#### Scenario: 列出根目录
- **WHEN** 调用 `List(ctx, "0")`
- **THEN** 返回 `[]Entry` 列表
- **AND** 每个 Entry 的 ID、Name、IsDir、Size、ModTime 字段 MUST 被正确填充

### Requirement: Read 操作
所有 driver SHALL 支持通过 `Read(ctx, entry, offset, size)` 读取文件指定字节范围的内容。

#### Scenario: 读取文件片段
- **WHEN** 调用 `Read(ctx, entry, 1048576, 1048576)` 读取第 1MB~2MB 范围
- **THEN** 返回 `io.ReadCloser`，读取该范围内的原始字节

### Requirement: Write 操作
支持写操作的 driver SHALL 实现 `Writer` 接口，包含 Mkdir、Move、Rename、Remove 操作。

#### Scenario: 创建目录
- **WHEN** 调用 `Mkdir(ctx, parentID, "newdir")`
- **THEN** 在 parentID 目录下创建名为 "newdir" 的子目录
- **AND** 返回新目录的 Entry

#### Scenario: 移动文件
- **WHEN** 调用 `Move(ctx, entry, dstParentID)`
- **THEN** 将 entry 移入 dstParentID 目录

#### Scenario: 重命名文件
- **WHEN** 调用 `Rename(ctx, entry, "newname.txt")`
- **THEN** 将 entry 的文件名改为 "newname.txt"

#### Scenario: 删除文件
- **WHEN** 调用 `Remove(ctx, entry)`
- **THEN** 删除 entry 对应的文件或目录

### Requirement: Upload 操作
支持上传的 driver SHALL 实现 `Uploader` 接口的 `Put` 方法，接收父目录 ID、文件名、文件大小和内容流，返回上传后的 Entry。

#### Scenario: 上传文件
- **WHEN** 调用 `Put(ctx, parentID, "file.txt", 1024, reader)`
- **THEN** 完成文件上传
- **AND** 返回包含 fid、name、size 等字段的 Entry
