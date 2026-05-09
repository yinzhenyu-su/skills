# 代码库审计修复

按优先级修复审计发现的问题。

## 1. goroutine panic recover

所有 `go func()` 必须加 defer recover，防止 JSON 解析/DB 操作 panic 导致进程退出。

**范围：**
- read.go: prefetch + getDecryptedChunk 并发
- manager.go: UpdateAccessTime、EvictIfNeeded、MaintenanceStart
- path_state.go: 后台子树预取
- sync.go: enqueueSyncDelay 延迟投递

## 2. MemCacheSizeMB 可配置化

当前硬编码 512MB，改为从配置文件读取 `cache.mem_cache_size_mb`。

## 3. 优雅退出

收到 SIGINT/SIGTERM 时：
1. 停止接受新上传任务
2. 等待 inflight upload 完成
3. 关闭 DB 连接
4. 卸载 FUSE

## 4. 杂物清理

- docs/todo/ duplicate-file-fix.md → done/
- .gitignore 补充 cache db
- 整理

## 5. 配置空洞修复

- dir_cache_ttl 接入实际代码
- max_retries 配置真正生效
- 字符串大小解析验证（10GB vs 10GiB）
