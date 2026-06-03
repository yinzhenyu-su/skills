# Phase 4: 平台端实现 Platform Services (各 0.5d)

目标：各平台实现 `core/qrypt.DirResolver`、`CredentialStore`、`NetworkConfig` 等接口。

前置依赖：Phase 1（接口定义）已完成。

## 任务 4.1: 桌面 CLI — Platform Services 实现

桌面 CLI (macOS/Linux/Windows) 使用 TOML 配置 + 文件系统，实现最轻量的 Platform Services。

### 4.1a: DirResolver — 基于 config.WorkDir

参考 `internal/config/config.go:241-249`。

```go
// cmd/qrypt/platform.go

type DesktopDirResolver struct{}

func (DesktopDirResolver) CacheDir() string {
    return filepath.Join(config.WorkDir(), "cache")
}

func (DesktopDirResolver) DataDir() string {
    return config.WorkDir()
}

func (DesktopDirResolver) ConfigDir() string {
    return config.WorkDir()
}
```

### 4.1b: TomlCredentialStore — TOML 桥接

架构文档 D11 (L577-601)，用于桌面端切换到原生安全存储之前的过渡期。

```go
// cmd/qrypt/platform.go

type TomlCredentialStore struct {
    cfg *config.Config
}

func (s *TomlCredentialStore) Get(key string) (string, error) {
    for _, m := range s.cfg.Mounts {
        if "cookie_"+m.Name == key {
            return m.Params.Cookie, nil
        }
        if "auth_"+m.Name == key {
            return m.Params.Authorization, nil
        }
    }
    return "", qrypt.ErrNotFound
}

func (s *TomlCredentialStore) Set(key, value string) error {
    return fmt.Errorf("toml credential store is read-only; edit qrypt.toml directly")
}

func (s *TomlCredentialStore) Delete(key string) error {
    return fmt.Errorf("toml credential store is read-only; edit qrypt.toml directly")
}
```

### 4.1c: DefaultNetworkConfig

```go
type DefaultNetworkConfig struct{}

func (DefaultNetworkConfig) HTTPClient() *http.Client {
    return http.DefaultClient
}
```

### 4.1d: NoopBackgroundTask + NoopAppLifecycle

Phase 1 中已定义 `NoopBackgroundTask` 和 `NoopAppLifecycle`，桌面 CLI 直接使用。

## 任务 4.2: 桌面 CLI — DriverFactory + Cipher 适配

### 4.2a: DriverFactory

```go
// cmd/qrypt/factory.go

type DesktopDriverFactory struct{}

func (f DesktopDriverFactory) CreateDriver(ctx context.Context, cfg qrypt.SessionConfig) (qrypt.Driver, error) {
    // 将 qrypt.SessionConfig 转为 drive/factory 能接受的参数
    // 调用 factory.NewDriverFromType
    // 返回 qrypt.Driver 适配器
}
```

### 4.2b: Cipher 适配器

```go
type cipherAdapter struct {
    inner *crypt.RcloneCipher
}

func (c *cipherAdapter) EncryptSegment(plain string) string {
    return c.inner.EncryptSegment(plain)
}
// ... 其他方法委派
```

## 任务 4.3: CacheInvalidatorHooks — daemon/ 适配

参考 Phase 2 任务 2.6 的 `daemonCacheHooks`。

## 各平台实现对照表

| 接口 | macOS CLI | Android | iOS |
|------|-----------|---------|-----|
| DirResolver | `config.WorkDir()` | `context.filesDir/cacheDir` | `NSSearchPath` |
| CredentialStore | `TomlCredentialStore` | EncryptedSharedPrefs | Keychain |
| NetworkConfig | `http.DefaultClient` | 系统代理 | ATS |
| BackgroundTask | `NoopBackgroundTask` | WorkManager | BGTaskScheduler |
| AppLifecycle | `NoopAppLifecycle` | Activity lifecycle | UIApplicationDelegate |

**注意**：移动端实现（Android/iOS）当前阶段**不做**，待 Phase 6 启动后再实现。

## 验证方式

```bash
cd skills/qrypt && go build ./cmd/qrypt/
```
Platform Services 的实现验证通过 `go build` 和 CLI 功能测试。
