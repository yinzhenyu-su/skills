# Qrypt 开发进度简报

## 当前结论

- 上传链路已打通，`203 CallbackFailed` 不再是主阻塞。
- 删除/修改稳定化已完成并归档，核心一致性问题已收敛。
- 当前主要问题是 macOS 挂载目录写入被系统层拦截：复制到挂载点报 `Operation not permitted`。

## 已完成（核心）

- 补齐上传闭环：分片上传后增加 `/file/update/hash`，并在可完成场景直接结束上传。
- 修复分片策略：32B rclone 头不再单独分片，避免 `EntityTooSmall`。
- 删除接口与参数对齐 AList 行为，修复 DNS/404/400 参数类问题。
- 写回稳定化完成：`pending_nodes` 恢复校验、重试上限、删除后统一清理、`Release -> Flush` 补齐。
- `name_cache` 已落库并接入 `Readdir`，目录展示性能改善。

## 当前阻塞（唯一）

- 现象：`cp` 到挂载目录稳定复现 `Operation not permitted`。
- 范围：在 `/Users/...` 与 `/tmp/...` 挂载点均可复现。
- 判断：更接近 macOS/FUSE 权限策略拦截，非上传 API 逻辑错误。

## 验证状态

- `go test ./...`（`skills/qrypt`）通过。
- 挂载命令可成功启动并解析 root path。
- 上传成功路径已验证；问题集中在挂载写入前置权限阶段。

## 下一步

1. 在本机完成终端/IDE/macFUSE 权限放行后复测 `cp`。
2. 通过后补充一条端到端写入验证记录（含日志），作为关闭该问题的验收依据。
