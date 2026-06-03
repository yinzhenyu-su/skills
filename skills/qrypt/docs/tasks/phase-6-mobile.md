# Phase 6: 移动端集成 (各 1d) — 暂不执行

目标：Android 和 iOS 端通过 gomobile 绑定 `core/qrypt`，调用 MobileAPI。

前置依赖：Phase 3（FileAPI + MobileAPI 实现）。

> ⚠️ **当前阶段暂不执行**，待桌面 CLI 切换完成后启动。

## 任务 6.1: Android 集成 (1d)

### 构建 AAR

```bash
gomobile bind -target android -o qrypt-core.aar \
  -androidapi 24 ./core/qrypt/
```

### Android 端工作

1. 实现 `DirResolver`（通过 `context.filesDir` / `cacheDir`）
2. 实现 `CredentialStore`（通过 EncryptedSharedPreferences）
3. 实现 `BackgroundTask`（通过 WorkManager）
4. 实现 `AppLifecycle`（通过 Activity lifecycle 回调）
5. Kotlin 调用 MobileAPI：
   ```kotlin
   val api = QryptMobileAPI(ctx)
   val json = api.list(ctx, "my-mount", "/")
   val entries = JSONArray(json)
   ```

### 文件结构

```
qrypt/
├── android/
│   ├── bridge.go         ← gomobile 导出层
│   ├── platform.go       ← Android Platform Services 实现
│   └── jniLibs/          ← NDK 库
```

## 任务 6.2: iOS 集成 (1d)

### 构建 XCFramework

```bash
gomobile bind -target ios -o QryptCore.xcframework \
  ./core/qrypt/
```

### iOS 端工作

1. 实现 `DirResolver`（通过 `NSSearchPathForDirectoriesInDomains`）
2. 实现 `CredentialStore`（通过 iOS Keychain）
3. 实现 `NetworkConfig`（ATS 配置）
4. 实现 `BackgroundTask`（通过 BGTaskScheduler）
5. 实现 `AppLifecycle`（通过 UIApplicationDelegate）
6. Swift 调用 MobileAPI：
   ```swift
   let api = QryptMobileAPI()
   let json = api.list("my-mount", path: "/")
   ```

### 文件结构

```
qrypt/
├── ios/
│   ├── bridge.go         ← gomobile 导出层
│   └── platform.go       ← iOS Platform Services 实现
```

## 任务 6.3: 大文件流式传输策略

架构文档 D9 (L548-553) 描述了移动端大文件的临时文件方案：

```
远程文件 → PullFile(tmpPath) → 原生端 mmap/流式读取 tmpPath
本地文件 → 写入 tmpPath → PushFile(tmpPath, remotePath)
```

### Journal 恢复

架构文档 Risks (L746)：移动端 App 可能被 Kill，使用 journal 恢复未完成任务。

```go
type Journal struct {
    Tasks []JournalEntry `json:"tasks"`
}

type JournalEntry struct {
    TaskID     string `json:"task_id"`
    Direction  string `json:"direction"` // "push" | "pull"
    LocalPath  string `json:"local_path"`
    RemotePath string `json:"remote_path"`
    Mount      string `json:"mount"`
    State      string `json:"state"` // "pending" | "in_progress"
    BytesDone  int64  `json:"bytes_done"`
}
```

## 任务 6.4: Platform Services 实现对照

| 接口 | Android | iOS |
|------|---------|-----|
| DirResolver | `context.filesDir` / `cacheDir` | `NSSearchPathForDirectoriesInDomains` |
| CredentialStore | EncryptedSharedPreferences | Keychain (Security.framework) |
| NetworkConfig | Network Security Config (XML) | ATS (Info.plist) |
| BackgroundTask | WorkManager | BGTaskScheduler |
| AppLifecycle | Activity lifecycle | UIApplicationDelegate |

## 验证方式

```bash
# Android
gomobile bind -target android -o qrypt-core.aar -androidapi 24 ./core/qrypt/

# iOS
gomobile bind -target ios -o QryptCore.xcframework ./core/qrypt/

# 集成测试需要在对应的 Android/iOS 工程中运行
```
