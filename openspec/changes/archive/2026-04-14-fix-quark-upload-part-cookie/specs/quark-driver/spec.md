## MODIFIED Requirements

本变更仅修复 `UploadPart` 函数的实现 bug（缺少 Cookie 头部），不改变任何 spec 层面的需求。原 `quark-driver` spec 中的所有 requirements 保持不变。

### Requirement: 分块上传认证（原有需求，现实现修正）

原需求描述见 `openspec/changes/quark-crypt-mount/specs/quark-driver/spec.md`。

**本变更修正**：原实现中 `UploadPart` 函数未携带 `Cookie` 头部，导致 OSS 存储层返回 403。此为实现缺陷，不影响需求本身。
