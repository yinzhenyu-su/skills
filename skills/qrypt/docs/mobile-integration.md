# Qrypt 跨平台核心集成指南

## 架构概述

```
┌────────────────────────────────────────────────────────┐
│                    Qrypt Core (Go)                     │
│                                                        │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │
│  │  drive   │ │  crypt   │ │  sync    │ │  cache   │  │
│  │ (云盘驱动)│ │ (加密引擎)│ │ (上传/下载)│ │ (缓存管理)│  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘  │
│                                                        │
│  ┌──────────┐ ┌──────────────┐ ┌──────────────────┐    │
│  │ protocol │ │   daemon     │ │  fs (可选，FUSE)  │    │
│  │(JSON-RPC)│ │ (后台服务)    │ │  macOS专用)       │    │
│  └──────────┘ └──────────────┘ └──────────────────┘    │
└────────────────────────────────────────────────────────┘
         ▲                     ▲                ▲
         │ JSON over           │ JNI/.so        │ LaunchAgent/
         │ Unix Socket         │ (cgo)          │ foreground svc
    ┌────┴────┐          ┌─────┴─────┐    ┌────┴─────┐
    │ macOS   │          │  Android  │    │  Linux   │
    │ App     │          │  App      │    │  Desktop │
    │ (Swift) │          │ (Kotlin)  │    │  (GTK)   │
    └─────────┘          └───────────┘    └──────────┘
```

## FUSE 依赖说明

FUSE 挂载 (`internal/fs`) 是**可选依赖**。通过 Go build tag `nofuse` 控制：

```bash
# macOS 原生构建（含 FUSE 挂载能力）
go build -o qrypt ./cmd/qrypt/

# Android / 无需 FUSE 场景
go build -tags nofuse -o qrypt ./cmd/qrypt/
```

`nofuse` 构建时 `internal/fs` 和 `cmd/qrypt/mount.go` 被排除，核心功能（drive/crypt/sync/cache/protocol/daemon）不受影响。

---

## 方案 A：macOS 原生 App（推荐）

### 架构

```
┌──────────────────────┐      JSON-RPC over      ┌──────────────────────────┐
│  macOS App (Swift)   │ ◄───── Unix Socket ────► │  qrypt mount (single    │
│                      │                          │  process, daemon+FUSE)  │
│  SwiftUI Layer       │    ~/.qrypt/qryptd.sock  │  Config Management      │
│  Network Service     │    (向后兼容)             │  Auth (Cookie/QR)       │
│  Notifications       │                          │  FUSE Mount             │
│  Menu Bar App        │                          │  Sync Workers           │
│  Status Bar Icon     │                          │  Cache Manager          │
└──────────────────────┘                          │  Drive Driver           │
                                                    └──────────────────────────┘
```

### 启动

```bash
# 编译
cd skills/qrypt
go build -o qrypt ./cmd/qrypt/

# 运行（自动启动 FUSE + WebSocket server，监听 ~/.qrypt/qryptd.sock）
./qrypt mount --config ./qrypt.toml --log-level info

# 后台模式（无 FUSE 挂载，仅 daemon 服务）
./qrypt mount --daemon --config ./qrypt.toml

# 或通过 launchd 管理（后台服务）
# cp com.qrypt.daemon.plist ~/Library/LaunchAgents/
# launchctl load ~/Library/LaunchAgents/com.qrypt.daemon.plist
```

### JSON-RPC API 参考

所有请求通过 Unix socket 发送 JSON-lines（`\n` 分隔）。

**请求格式：**
```json
{"id": 1, "method": "status"}
{"id": 2, "method": "list_dir", "params": {"mount_name": "default", "path": "/"}}
```

**响应格式：**
```json
{"id": 1, "result": {"version": "1.0", "mount_state": "mounted"}, "error": null}
```

**事件推送（订阅后自动推送）：**
```json
{"id": 0, "method": "event", "params": {"type": "sync_progress", "timestamp": 1716000000, "data": {"fid": "abc"}}}
```

### API 方法列表

完整方法列表见 `internal/protocol/types.go`。常用方法：

| 方法 | 用途 | 返回 |
|------|------|------|
| `status` | daemon 状态（版本/挂载/运行时间） | `DaemonStatus` |
| `list_dir` | 列出目录内容 | `ListDirResult` |
| `move` | 移动/重命名文件 | `MoveResult` |
| `find` | 搜索文件 | `FindResult` |
| `mkdir` | 创建目录 | `MkdirResult` |
| `remove` | 删除文件/目录 | `RemoveResult` |
| `push_start` | 上传文件 | `PushStartResult` |
| `pull_start` | 下载文件 | `PullStartResult` |
| `dashboard` | 聚合状态（状态+同步+缓存+传输） | `DashboardData` |
| `subscribe_events` | 订阅实时事件（长连接） | 事件推送流 |

### 类型定义

参见 `internal/protocol/types.go`。

```go
type DaemonStatus struct {
    Version    string
    Uptime     string
    ConfigPath string
    MountPoint string
    MountState MountState // mounted/unmounted/mounting/unmounting/error
    DriveType  string
    LastError  string
    Mounts     []MountSummary
}
```

### 事件类型

| 事件 | 触发时机 |
|------|---------|
| `mount_state_changed` | 挂载/卸载状态变化 |
| `sync_progress` | 同步进度更新 |
| `sync_completed` | 同步完成 |
| `sync_failed` | 同步失败 |
| `disk_space_low` | 磁盘空间不足 |
| `error` | daemon 内部错误 |

### Swift 客户端示例

```swift
// QryptService.swift
class QryptService {
    let socketPath = "\(NSHomeDirectory())/.qrypt/qryptd.sock"
    var requestId: Int64 = 0
    
    func call<T: Decodable>(method: String, params: Encodable? = nil) async throws -> T {
        let id = OSAtomicIncrement64(&requestId)
        let request = JSONRPCRequest(id: id, method: method, params: params)
        let data = try JSONEncoder().encode(request)
        
        let socket = try await connectUnixSocket(path: socketPath)
        try socket.write(data + "\n".data(using: .utf8)!)
        
        let response = try await socket.readJSONLine() as JSONRPCResponse<T>
        return response.result
    }
    
    func status() async throws -> DaemonStatus {
        try await call(method: "status")
    }
    
    func listDir(_ path: String) async throws -> ListDirResult {
        try await call(method: "list_dir", params: ["mount_name": "default", "path": path])
    }
    
    func subscribeEvents() -> AsyncStream<QryptEvent> {
        AsyncStream { continuation in
            Task {
                let _: StatusResult = try await call(method: "subscribe_events")
                // Receive events...
            }
        }
    }
}
```

### macOS LaunchDaemon 配置

```xml
<!-- ~/Library/LaunchAgents/com.qrypt.daemon.plist -->
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.qrypt.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/qrypt</string>
        <string>mount</string>
        <string>--daemon</string>
        <string>--config</string>
        <string>/Users/you/.config/qrypt/qrypt.toml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/Users/you/.qrypt/qryptd.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/you/.qrypt/qryptd.log</string>
</dict>
</plist>
```

---

## 方案 B：Android App 集成

### 架构

```
┌──────────────────────────────────────────────────────┐
│  Android App (Kotlin)                                │
│  ┌────────────────────────────────────────────────┐  │
│  │  JNI Bridge (libqrypt_core.so)                 │  │
│  │  ┌──────────────────────────────────────────┐  │  │
│  │  │  Go Core (nofuse build)                  │  │  │
│  │  │  drive.QuarkDriver  → 网络层 (net/http)   │  │  │
│  │  │  crypt.RcloneCipher  → 纯计算             │  │  │
│  │  │  sync.Uploader/Downloader → 网络+计算     │  │  │
│  │  │  cache.CacheManager → 文件系统 (data dir) │  │  │
│  │  └──────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────┘  │
│                                                       │
│  ┌────────────────────────────────────────────────┐  │
│  │  Android Native Layer                          │  │
│  │  ForegroundService (后台同步)                   │  │
│  │  WorkManager (定期任务)                         │  │
│  │  ContentProvider / MediaStore (文件访问替代FUSE) │  │
│  └────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

### 编译 .so 库

前置条件：安装 Android NDK。

```bash
# 设置 NDK 交叉编译器
export NDK=$HOME/Library/Android/sdk/ndk/27.0.12077973
export TOOLCHAIN=$NDK/toolchains/llvm/prebuilt/darwin-x86_64
export CC=$TOOLCHAIN/bin/aarch64-linux-android21-clang

# 编译 arm64-v8a（主流设备）
mkdir -p android/jniLibs/arm64-v8a
cd skills/qrypt
CGO_ENABLED=1 \
GOOS=android \
GOARCH=arm64 \
CC=$CC \
go build -tags nofuse \
  -buildmode=c-shared \
  -o android/jniLibs/arm64-v8a/libqrypt_core.so \
  ./mobile/

# 编译 armeabi-v7a（旧设备）
# export CC=$TOOLCHAIN/bin/armv7a-linux-androideabi21-clang
# CGO_ENABLED=1 GOOS=android GOARCH=arm CC=$CC \
#   go build -tags nofuse -buildmode=c-shared \
#   -o android/jniLibs/armeabi-v7a/libqrypt_core.so ./mobile/
```

### JNI 接口定义（示例）

```go
// mobile/bridge.go
package main

import "C"
import (
    "qrypt/internal/crypt"
    "qrypt/internal/drive/factory"
    "qrypt/internal/drive"
    "qrypt/internal/sync"
)

var (
    driver  drive.Driver
    cipher  *crypt.RcloneCipher
    uploader *sync.Uploader
)

//export Java_com_qrypt_core_QryptCore_init
func Init(env *C.JNIEnv, cookie *C.char, password *C.char) *C.char {
    driver, _ = factory.NewDriverFromConfig(...)
    cipher, _ = crypt.NewRcloneCipher(C.GoString(password), "")
    return C.CString("ok")
}

//export Java_com_qrypt_core_QryptCore_listFiles
func ListFiles(parentFid *C.char) *C.char {
    entries, _ := driver.List(context.Background(), C.GoString(parentFid))
    data, _ := json.Marshal(entries)
    return C.CString(string(data))
}

//export Java_com_qrypt_core_QryptCore_upload
func Upload(localPath *C.char, parentFid *C.char) *C.char {
    req := sync.Request{...}
    result, _ := uploader.Upload(context.Background(), req)
    data, _ := json.Marshal(result)
    return C.CString(string(data))
}

func main() {}
```

### Kotlin 端调用

```kotlin
// QryptCore.kt
class QryptCore {
    companion object {
        init {
            System.loadLibrary("qrypt_core")
        }
    }

    /** 初始化加密引擎和云盘驱动 */
    external fun init(cookie: String, password: String): String

    /** 列出目录下的文件，返回 JSON 数组 */
    external fun listFiles(parentFid: String): String

    /** 上传文件 */
    external fun upload(localPath: String, parentFid: String): String

    /** 下载文件 */
    external fun download(fid: String, localPath: String): String

    /** 获取同步状态 */
    external fun syncStatus(): String

    /** 获取缓存使用量 */
    external fun cacheUsage(): String
}
```

### 关键限制与应对

| 限制 | 说明 | 应对方案 |
|------|------|---------|
| **无 FUSE** | Android 内核不提供 FUSE 支持 | 用 `ContentProvider` + `MediaStore` 替代，或直接操作应用私有目录 |
| **后台限制** | Android 8+ 严格限制后台服务 | 使用 `ForegroundService`（前台通知）+ `WorkManager` 定期任务 |
| **文件沙箱** | 应用只能访问 `data/data/<pkg>/` | `CacheManager` 的 `Dir` 配置为 `context.cacheDir` |
| **网络权限** | 需要声明 `INTERNET` 权限 | 在 `AndroidManifest.xml` 中添加 |
| **内存限制** | Android 对 JNI 堆内存有限制 | 大文件建议流式上传/下载，避免一次性加载到内存 |
| **加密库** | crypto 包在 Android NDK 下可能需要 BoringSSL | Go 标准库 `crypto/aes` + `golang.org/x/crypto` 在 Android 上支持良好 |

---

## 可复用代码矩阵

| 包 | macOS | Android | 说明 |
|----|-------|---------|------|
| `internal/drive/` | ✅ | ✅ | 纯网络层，平台无关 |
| `internal/crypt/` | ✅ | ✅ | 纯计算，平台无关 |
| `internal/sync/` | ✅ | ✅ | 依赖 net/http + os，Android 上 os 可用 |
| `internal/cache/` | ✅ | ✅ | 依赖 os.File，Android 上可用 |
| `internal/config/` | ✅ | ✅ | TOML 解析，平台无关 |
| `internal/log/` | ✅ | ✅ | 文件日志，平台无关 |
| `internal/protocol/` | ✅ | ❌ | JSON-RPC 编码，仅 macOS daemon 需要 |
| `internal/daemon/` | ✅ | ❌ | 后台服务管理，仅 macOS 需要 |
| `internal/fs/` | ✅ (FUSE) | ❌ | FUSE 文件系统，Android 不可用 |

---

## 快速开始

### macOS

```bash
# 1. 编译
cd skills/qrypt
go build -o qrypt ./cmd/qrypt/

# 2. 创建配置文件
cp qrypt.toml ~/.config/qrypt/
# 编辑 ~/.config/qrypt/qrypt.toml 填入 cookie 和密码

# 3. 启动（单进程，daemon + FUSE）
./qrypt mount --config ~/.config/qrypt/qrypt.toml

# 4. 在 Swift App 中连接 ~/.qrypt/qryptd.sock 进行 RPC 通信
```

### Android

```bash
# 1. 编译 .so
export CC=$NDK/toolchains/llvm/prebuilt/darwin-x86_64/bin/aarch64-linux-android21-clang
cd skills/qrypt
CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC=$CC \
  go build -tags nofuse -buildmode=c-shared \
  -o app/src/main/jniLibs/arm64-v8a/libqrypt_core.so \
  ./mobile/

# 2. 在 Android 项目中使用
```
