## Why

`UploadPart` 函数在向上传端点 `ul-sz.pds.quark.cn` 发送分块上传请求时，没有携带用户的 Cookie，导致 OSS 存储层返回 `403 AccessDenied: Anonymous user has no right to access this object`。这是一个纯粹的 bug fix——`UploadAuth` 路径通过 `d.request()` 发送自动携带 Cookie，但 `UploadPart` 直接构造 HTTP 请求时遗漏了 Cookie。

## What Changes

- 修复 `QuarkDriver.UploadPart` 函数，在构造的 HTTP 请求中添加 `Cookie` 头部
- 不涉及新的功能或 API 变更

## Capabilities

### Modified Capabilities

- `quark-driver`: 修正分块上传请求的认证头部携带问题（仅实现层面修正，不改变原有需求契约）

## Impact

- **修改文件**: `skills/qrypt/internal/driver/quark.go`
- **影响功能**: 加密挂载后的文件上传（`UploadPart` 调用链）
