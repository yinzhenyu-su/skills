## Context

`UploadPart` 函数（`quark.go:221-265`）直接构造 `http.Request` 而非通过 `d.request()` 发送，导致无法自动携带 `d.cookie`。上传目标 `ul-sz.pds.quark.cn` 是 OSS 存储层，需要 Cookie 验证用户身份。

## Goals / Non-Goals

**Goals:**
- 在 `UploadPart` 的请求中添加 `Cookie` 头部，使 OSS 存储层能识别当前用户

**Non-Goals:**
- 不改变上传流程的其他逻辑
- 不修改 `UploadAuth` 或其他函数
- 不涉及 spec 级别的需求变更

## Decisions

### Decision: 直接在 `UploadPart` 中添加 `Cookie` 头部

**Option A** (chosen): 在 `req.Header.Set` 链末尾添加 `req.Header.Set("Cookie", d.cookie)`
- 改动最小，一行代码
- 与 `UploadAuth` 路径保持一致的认证方式

**Option B**: 重构 `UploadPart` 使用 `d.request()` 封装
- 改动过大，`d.request()` 封装了 JSON 响应解析，不适合文件二进制上传

## Risks / Trade-offs

无显著风险——添加 `Cookie` 头部不影响现有逻辑，仅修复认证遗漏。
