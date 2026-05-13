## ADDED Requirements

### Requirement: 加密分片上传全流程

系统必须支持将本地 staging 中的明文文件加密后分片上传到夸克网盘 OSS，并在上传完成后执行哈希验证。

#### Scenario: Upload lifecycle
- **WHEN** 一个 dirty 文件被 `syncFile` 处理
- **THEN** 上传器按以下顺序执行：

```
1. deleteExistingFileByName — 按明文名查找并删除远程已有文件
2. UploadPre — 获取 OSS 上传会话（支持断点续传的 uploadID）
3. UploadPart — 读取 staging 文件 → 加密 (NaCl Secretbox) → OSS 分片上传
4. UpdateHash — 上报加密对象的 MD5+SHA1 哈希
5. 若 finish=true → 上传完成；否则 → UploadCommit + UploadFinish
```

#### Scenario: 断点续传
- **WHEN** 上传中断后恢复
- **AND** Node 中保存了 `uploadID` 和 `lastPart`
- **THEN** 上传器从 `lastPart + 1` 开始继续上传
- **AND** `UploadPre` 带上已有的 `uploadID`

### Requirement: 加密发生在上传路径中

加密不在写入 staging 时发生，而是在 `UploadPart` 读取 staging 明文文件后、发送到 OSS 之前进行 NaCL Secretbox 加密。

#### Scenario: Staging 为明文
- **WHEN** 用户在 FUSE 挂载点写入文件
- **THEN** staging 以明文存储
- **AND** `syncFile` 中 `UploadPart` 调用 `RcloneCipher.EncryptBlock` 将每个 64KB 块加密后发送

### Requirement: Hash 验证闭环

系统使用 `crypto/md5` 和 `crypto/sha1` 计算加密后对象的哈希，通过 `TeeReader` 在上传流式传输过程中同时计算完整哈希。

#### Scenario: TeeReader hash computation
- **WHEN** 分片上传到 OSS
- **THEN** 使用 `io.TeeReader` 同步计算加密数据的 MD5 和 SHA1
- **AND** 上传完成后通过 `/file/update/hash` API 上报
- **AND** 若 `finish=true`，跳过 commit 阶段直接完成
