# 上传链路多重容错修复方案 (v2)

> **核心思路: 三层重试架构。仅 OSS 层做 per-part 重试（幂等），不需要删 placeholder 再从头传。**

## 关键发现

1. **StarRocks API doc 确认:** Quark 支持 `upload_id` 断点续传。OSS 同 `upload_id` + 同 `part_number` 幂等写入——重传相同数据只是覆盖，不产生重复
2. **`UpPreResp` 已有 `UploadId`** (`types.go:110`—`json:"upload_id"`)——Quark 返回了，但从未存储和利用
3. **`UploadPre` 不传 `hash`**——秒传从未生效，所有文件都走完整 multipart upload
4. **每次重试 nonce 不同**——加密数据会变，所以不能复用旧分片。但**在同一 Sync 内** per-part 重试的 nonce 相同，数据相同，幂等安全

## 三层重试架构

```
                       时间线
                       
Layer 1: HTTP Transport  ─── 500ms ─── 1s ─── 2s ─── (最多 3 次)
  ↓ 仍失败
Layer 2: Per-Part OSS     ─── 200ms ─── 500ms ─── 1s ─── (最多 3 次/分片)
  ↓ 仍失败 (极罕见)
Layer 3: VFS Worker       ─── 2s ─── 4s ─── 8s ─── (最多 5 次)
```

### Layer 1: HTTP 传输重试 (`quark.go`)

**范围:** `requestWithBase`（所有 Quark API 调用）+ `UploadPart`（OSS PUT）+ `UploadCommit`（OSS POST）

**触发条件:** DNS 错误、连接超时、连接拒绝、TLS 握手失败、HTTP 429/5xx

**策略:** 指数退避 500ms/1s/2s，jitter ±25%，最多 3 次

**不触发:** HTTP 4xx（除 429）、业务 code 错误

### Layer 2: 分片级重试 (`manager.go` Sync)

**范围:** `UploadPart` 循环内，每个单个分片

**策略:**
```go
for partNumber := 1; ; partNumber++ {
    // 读 staging + 加密 → buf[:n]
    for attempt := 0; attempt < 3; attempt++ {
        etag, err := m.driver.UploadPart(pre, partNumber, buf[:n])
        if err == nil {
            break // 成功
        }
        // 幂等：OSS 同 part_number 重传=覆盖，无需清理
        time.Sleep(retryBackoffPart(attempt)) // 200ms/500ms/1s
    }
}
```

**为什么不需要删 placeholder：** 同一个 `upload_id`（来自第一次 UploadPre），同一个 `part_number`，同一份加密数据（nonce 不变）。OSS PUT 是幂等的。

**UploadPre/UpdateHash/UploadCommit/UploadFinish 不在这里重试：** 这些是 Quark API 调用，Layer 1 HTTP 重试覆盖。而且它们失败概率远低于 OSS PUT（quark API 响应通常很稳定，OSS 偶尔 503）。

### Layer 3: VFS Worker 重试 (`sync.go` uploadWorker)

**范围:** `syncFile` 整体失败后

**策略:** 使用现有的 `retryState sync.Map`（目前死代码），指数退避 2s/4s/8s/16s/32s，最多 `maxRetries` 次（默认 5）

**触发条件:** Layer 1+2 全部耗尽仍失败（极罕见——需要 OSS 持续 503 超过 3+3=6 次重试）

**副作用:** 最后一种手段，重试时 call UploadPre 会得到新 `upload_id`（因为这是 VFS 层面的全新重试）

## 修改文件清单

### 1. `internal/driver/quark.go` — HTTP 传输层重试

#### 新增函数

```go
// isRetryableHTTPError 判断网络层错误是否可重试
func isRetryableHTTPError(err error) bool {
    if err == nil {
        return false
    }
    var netErr net.Error
    if errors.As(err, &netErr) {
        return true // 所有 net.Error（timeout, reset, refuse 等）都重试
    }
    var dnsErr *net.DNSError
    if errors.As(err, &dnsErr) {
        return true
    }
    msg := strings.ToLower(err.Error())
    return strings.Contains(msg, "tls handshake") ||
        strings.Contains(msg, "broken pipe") ||
        strings.Contains(msg, "no such host")
}

// isRetryableHTTPStatus 判断 HTTP 状态码是否可重试
func isRetryableHTTPStatus(code int) bool {
    return code == http.StatusTooManyRequests ||
        code >= 500 // 5xx 服务端错误
}

// retryBackoff 加权 jitter 的指数退避
func retryBackoff(attempt int) time.Duration {
    d := time.Duration(500<<uint(attempt)) * time.Millisecond // 500ms, 1s, 2s
    jitter := time.Duration(float64(d) * (0.75 + float64(rand.Intn(51))/100.0))
    return jitter
}
```

#### `requestWithBase` 修改

```go
func (d *QuarkDriver) requestWithBase(method, baseURL, path string, query map[string]string, body interface{}, result interface{}) error {
    const maxRetries = 3
    for attempt := 0; attempt <= maxRetries; attempt++ {
        // ... build req (same logic) ...
        
        resp, err := d.client.Do(req)
        if err != nil {
            if attempt < maxRetries && isRetryableHTTPError(err) {
                driver.Log.Debugf("HTTP retry %d/%d: %s %s error: %v\n", attempt+1, maxRetries, method, path, err)
                time.Sleep(retryBackoff(attempt))
                continue
            }
            return err
        }
        defer func() { io.Copy(io.Discard, resp.Body); resp.Body.Close() }()
        
        if resp.StatusCode >= 400 {
            if attempt < maxRetries && isRetryableHTTPStatus(resp.StatusCode) {
                driver.Log.Debugf("HTTP retry %d/%d: %s %s status=%d\n", attempt+1, maxRetries, method, path, resp.StatusCode)
                time.Sleep(retryBackoff(attempt))
                continue
            }
            // ... existing error handling ...
        }
        
        if result != nil {
            return json.NewDecoder(resp.Body).Decode(result)
        }
        return nil
    }
    return nil // unreachable
}
```

**注意：** `requestWithBase` 原来的 defer body close 在循环内有问题——需要在失败分支手动 close。改为在失败分支 `resp.Body.Close()` 后继续。

#### `UploadPart` OSS PUT 重试

```go
func (d *QuarkDriver) UploadPart(pre *UpPreResp, partNumber int, data []byte) (string, error) {
    // ... existing logic: build auth, build req ...
    const ossMaxRetries = 3
    for attempt := 0; attempt <= ossMaxRetries; attempt++ {
        resp, err := d.client.Do(req)
        if err != nil {
            if attempt < ossMaxRetries && isRetryableHTTPError(err) {
                driver.Log.Debugf("OSS UploadPart retry %d/%d: part=%d error=%v\n", attempt+1, ossMaxRetries, partNumber, err)
                time.Sleep(retryBackoffPart(attempt))
                continue
            }
            return "", err
        }
        resp.Body.Close() // need to close each attempt
        
        if resp.StatusCode != http.StatusOK {
            if attempt < ossMaxRetries && isRetryableHTTPStatus(resp.StatusCode) {
                driver.Log.Debugf("OSS UploadPart retry %d/%d: part=%d status=%d\n", attempt+1, ossMaxRetries, partNumber, resp.StatusCode)
                time.Sleep(retryBackoffPart(attempt))
                continue
            }
            bodyBytes, _ := io.ReadAll(resp.Body)
            return "", fmt.Errorf("oss put status: %d, error: %s", resp.StatusCode, string(bodyBytes))
        }
        return resp.Header.Get("Etag"), nil
    }
    return "", fmt.Errorf("oss put failed after %d retries", ossMaxRetries)
}
```

#### `UploadCommit` OSS POST 重试

`UploadCommit` 最后一步是 POST XML 到 OSS（line 457-488）。同样加 retry 循环。

---

### 2. `internal/upload/manager.go` — 分片级重试

```go
const partRetryMax = 3

func partRetryBackoff(attempt int) time.Duration {
    d := time.Duration(200<<uint(attempt)) * time.Millisecond // 200ms, 400ms, 800ms
    jitter := time.Duration(float64(d) * (0.75 + float64(rand.Intn(51))/100.0))
    return jitter
}
```

**Sync 方法修改——UploadPart 循环加重试：**

```go
for partNumber := 1; ; partNumber++ {
    n, readErr := io.ReadFull(encReader, buf)
    if readErr == io.EOF && n == 0 {
        break
    }
    if readErr != nil && readErr != io.ErrUnexpectedEOF {
        return result, readErr
    }
    // ... md5/sha1 hash ...

    partData := buf[:n]
    var etag string
    var partErr error
    for attempt := 0; attempt < partRetryMax; attempt++ {
        partStart := time.Now()
        etag, partErr = m.driver.UploadPart(pre, partNumber, partData)
        result.UploadPartDuration += time.Since(partStart)
        if partErr == nil {
            break // 成功
        }
        if attempt < partRetryMax-1 {
            driver.Log.Warnf("Sync: UploadPart %d retry %d/%d for %s: %v\n", partNumber, attempt+1, partRetryMax, req.Path, partErr)
            time.Sleep(partRetryBackoff(attempt))
        }
    }
    if partErr != nil {
        return result, fmt.Errorf("UploadPart %d failed after %d retries: %v", partNumber, partRetryMax, partErr)
    }
    
    etags = append(etags, etag)
    result.PartCount++
    result.UploadedBytes += int64(n)
    // ... readErr check ...
}
```

---

### 3. `internal/vfs/sync.go` — uploadWorker 重试 + `retryState` 接入

#### `retryState` 使用
声明了但从未 Load/Store 的 `retryState sync.Map` 现在真正接入。

```go
// 在 QryptFS 上新增方法
func (fs *QryptFS) getRetryCount(n *node) int {
    if v, ok := fs.retryState.Load(n); ok {
        return v.(int)
    }
    return 0
}

func (fs *QryptFS) incrementRetryCount(n *node) int {
    count := fs.getRetryCount(n) + 1
    fs.retryState.Store(n, count)
    return count
}

func (fs *QryptFS) resetRetryCount(n *node) {
    fs.retryState.Delete(n)
}
```

#### `uploadWorker` 修改

```go
func (fs *QryptFS) uploadWorker() {
    // ... existing defer recover ...
    for task := range fs.uploadChan {
        savedPath := task.node.currentPath
        err := fs.syncFile(task.node.currentPath, task.node)
        
        if err != nil {
            driver.Log.Errorf("Sync: failed to sync %s: %v\n", task.node.currentPath, err)
            
            // VFS 级重试（Layer 3）
            retryCount := fs.incrementRetryCount(task.node)
            if retryCount < fs.maxRetries {
                backoff := time.Duration(2<<uint(retryCount-1)) * time.Second // 2s, 4s, 8s, 16s, 32s
                driver.Log.Warnf("uploadWorker: retry %d/%d for %s after %v\n",
                    retryCount+1, fs.maxRetries, task.node.currentPath, backoff)
                
                go func(n *node, d time.Duration) {
                    defer func() {
                        if r := recover(); r != nil {
                            driver.Log.Errorf("PANIC in uploadWorker retry goroutine: %v\n%s\n", r, debug.Stack())
                        }
                    }()
                    time.Sleep(d)
                    // 重入前检查节点是否仍有效
                    n.mu.RLock()
                    cp := n.currentPath
                    cancelled := n.isCancelled()
                    n.mu.RUnlock()
                    if cp == "" || cancelled {
                        return // 节点已被删除，不重试
                    }
                    n.mu.Lock()
                    n.syncQueued = true
                    n.mu.Unlock()
                    fs.uploadChan <- syncTask{node: n}
                }(task.node, backoff)
            } else {
                driver.Log.Errorf("uploadWorker: max retries (%d) exhausted for %s, giving up\n", fs.maxRetries, task.node.currentPath)
                // 放弃：清理本地状态
                task.node.mu.Lock()
                task.node.isDirty = false
                task.node.syncQueued = false
                task.node.mu.Unlock()
                if fs.cache != nil {
                    _ = fs.cache.RemovePendingNode(task.node.currentPath)
                }
                fs.retryState.Delete(task.node)
            }
            
            if task.opsLogID > 0 && fs.cache != nil {
                if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
                    _ = db.UpdateOpsLogStatus(task.opsLogID, "FAILED")
                }
            }
        } else {
            // 成功路径
            fs.resetRetryCount(task.node) // 清除重试计数
            
            // 幽灵文件检测（现有逻辑不变）...
            // opsLog DONE（现有逻辑不变）...
        }
    }
}
```

**重试 goroutine 的超时保护：** 最长的 re-queue 延迟是 32s（第 5 次重试时的 2^4=16s 再乘 2）。goroutine 会在 system-shutdown 时被 `uploadChan` 关闭阻塞，不会泄漏。

#### 重试成功路径 `syncFile` 的 defer

`syncFile` 现有的 defer 逻辑不变。它只处理 `isStillDirty && err == nil` 的情况（上传期间有新 Write 写入）。新的 VFS 重试在 `uploadWorker` 层。

---

### 4. 清理相关

#### `deleteNodePath` 清理 `retryState`

```go
// path_state.go — deleteNodePath 或其调用处
fs.retryState.Delete(n)
```

#### `Shutdown` 清理

Shutdown 时 `close(fs.uploadChan)`，等待中的 goroutine 会因写入关闭的 channel 而 panic。所以重试 goroutine 必须在写入前感知 shutdown。

**方案：** 在 enqueueSyncDelay 的 goroutine 中，re-enqueue 时加 shutdown 检查。

```go
// uploadWorker retry goroutine
select {
case fs.uploadChan <- syncTask{node: n}:
case <-fs.shutdownCh: // 新增 shutdown channel
    return
}
```

实际上更简单：`uploadChan` 是 buffered channel (1000)，关闭后写入会 panic。但我们的 re-enqueue goroutine 在 shutdown 时不应该再写入。

最简方案：重试 goroutine 写入前检查 `uploadChan` 是否已关闭（通过一个 atomic flag）。

```go
func (fs *QryptFS) isShuttingDown() bool {
    return atomic.LoadInt32(&fs.shuttingDown) == 1
}
```

Shutdown 时：
```go
func (fs *QryptFS) Shutdown() {
    atomic.StoreInt32(&fs.shuttingDown, 1)
    close(fs.uploadChan)
    // ...
}
```

重试 goroutine 写入前：
```go
if fs.isShuttingDown() { return }
fs.uploadChan <- syncTask{node: n}
```

---

## 重试叠加耗时

| 层 | 每层重试次数 | backoff | 该层最大耗时 |
|----|------------|---------|------------|
| Layer 1 (HTTP) | 3 | 500ms/1s/2s | ~3.5s |
| Layer 2 (Per-Part) | 3 | 200ms/400ms/800ms | ~1.4s/分片 |
| Layer 3 (VFS) | 5 | 2s/4s/8s/16s/32s | ~62s |
| **总上限** | | | **~67s** |

实际上 Layer 1 的 HTTP 重试覆盖了 90%+ 的场景（网络闪断通常 <1s），Layer 2 覆盖 99%+。Layer 3 只有在极端情况（OSS 连续 503 超过 3+3=6 次重试）才会被触发。

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `internal/driver/quark.go` | 新增 `isRetryableHTTPError`, `isRetryableHTTPStatus`, `retryBackoff`；修改 `requestWithBase`, `UploadPart`, `UploadCommit` |
| `internal/upload/manager.go` | 新增 `partRetryMax`, `partRetryBackoff`；修改 `Sync` 的 UploadPart 循环 |
| `internal/vfs/sync.go` | 修改 `uploadWorker`（重试+放弃逻辑）；新增 `getRetryCount`, `incrementRetryCount`, `resetRetryCount` |
| `internal/vfs/path_state.go` | `deleteNodePath` 中清理 `retryState` |
| `internal/vfs/facade.go` | 新增 `Shutdown` 的 `shuttingDown` atomic flag |

## 可以但暂不做的优化

1. **秒传(hash)支持:** `UploadPre` 传入 SHA1/MD5 → 文件已在服务器时直接秒传。目前 rclone cipher 阶段后可以得到加密数据的 hash，但需要先计算再请求，会有一轮额外 I/O。

2. **UploadPart 并行上传:** 当前串行上传分片，大文件可以并行 N 个分片加速。但会大幅增加复杂度（重试逻辑、分片顺序保证、并发控制），且需要在 Layer 2 处理更多并发问题。

3. **上传进度通知:** 给用户反馈当前上传状态。可以通过 `syncObserver` 接口扩展，但需要前端配合。
