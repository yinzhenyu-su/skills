# Driver 自注册重构方案

> 目标：消除 driver 创建逻辑中的 switch-case 重复，采用 `database/sql` 风格的自注册模式，使新增 driver 时无需修改 factory。

## 1. 现状问题

### 1.1 三处重复的 switch-case

| 位置 | 用途 |
|------|------|
| `drivers/factory/factory.go` `NewDriverFromConfig` | CLI 从 TOML 配置创建 driver |
| `drivers/factory/factory.go` `NewDriverFromType` | CLI / daemon 从 MountParams 创建 driver |
| `mobile/driver_factory.go` `CreateDriver` | 移动端从 CredentialStore 创建 driver |

每次新增 driver（如未来的 aliyundrive、115 等），必须同时修改以上所有文件。

### 1.2 `SetCipher` 缺乏正式接口

当前 `QuarkDriver` 和 `LocalDriver` 实现了 `SetCipher(cipher.Cipher)`，调用方通过匿名接口断言：

```go
// internal/mount/manager.go:222, internal/platform/core.go:38,87
if setter, ok := drv.(interface{ SetCipher(cipher.Cipher) }); ok {
    setter.SetCipher(rcloneCipher)
}
```

问题：没有编译期保障，重命名或签名变更不会报错。

### 1.3 `splitTypeMount` 重复

`internal/mount/session.go` 和 `mobile/driver_factory.go` 各有一份相同的 `splitTypeMount` 函数。

---

## 2. 目标架构

```
drivers/
├── driver.go        # 接口定义（不变 + 新增 CipherSetter）
├── registry.go      # [NEW] Register / New / SupportedTypes
├── errors.go        # 不变
├── quark/
│   └── driver.go    # init() { drivers.Register("quark", ...) }
├── yun139/
│   └── driver.go    # init() { drivers.Register("yun139", ...) }
└── localfs/
    └── driver.go    # init() { drivers.Register("localfs", ...) }

drivers/factory/     # 变为薄适配层，config → Params → drivers.New()
```

---

## 3. 变更详情

### 3.1 [MODIFY] `drivers/driver.go` — 新增 CipherSetter 接口

```diff
+// CipherSetter is implemented by drivers that support client-side encryption.
+// After construction, callers should check for this interface and inject the cipher.
+type CipherSetter interface {
+    SetCipher(c cipher.Cipher)
+}
```

在 `driver.go` 尾部追加，不影响现有 `Driver`/`Writer`/`Uploader`/`PathResolver` 接口。

需要在 import 中新增 `cipher` 包引用。

### 3.2 [NEW] `drivers/registry.go` — 注册表

```go
package drivers

import (
    "fmt"
    "sort"
    "strings"
    "sync"
)

// Params is a flat key-value bag passed to driver constructors.
// Each driver defines its own required/optional keys (e.g. "cookie", "root_path").
type Params map[string]string

// Constructor creates a Driver from the given Params.
// Implementations should validate required params and return descriptive errors.
type Constructor func(params Params) (Driver, error)

var (
    mu       sync.RWMutex
    registry = make(map[string]Constructor)
)

// Register makes a driver constructor available by name.
// Panics on duplicate registration (same behavior as database/sql).
// Typically called from init() in each driver package.
func Register(name string, ctor Constructor) {
    mu.Lock()
    defer mu.Unlock()
    if _, dup := registry[name]; dup {
        panic("drivers: Register called twice for driver " + name)
    }
    registry[name] = ctor
}

// New creates a Driver by looking up the registered constructor for the given name.
func New(name string, params Params) (Driver, error) {
    mu.RLock()
    ctor, ok := registry[name]
    mu.RUnlock()
    if !ok {
        return nil, fmt.Errorf("unknown driver type: %q (registered: %s)", name, strings.Join(SupportedTypes(), ", "))
    }
    return ctor(params)
}

// SupportedTypes returns a sorted list of registered driver type names.
func SupportedTypes() []string {
    mu.RLock()
    defer mu.RUnlock()
    names := make([]string, 0, len(registry))
    for name := range registry {
        names = append(names, name)
    }
    sort.Strings(names)
    return names
}
```

### 3.3 [MODIFY] 各 driver 包 — 添加 `init()` 自注册

#### `drivers/quark/driver.go`

```diff
+func init() {
+    drivers.Register("quark", func(params drivers.Params) (drivers.Driver, error) {
+        cookie := params["cookie"]
+        if cookie == "" {
+            return nil, fmt.Errorf("missing cookie for quark driver")
+        }
+        return NewDriver(cookie, params["root_path"]), nil
+    })
+}
```

#### `drivers/yun139/driver.go`

```diff
+func init() {
+    drivers.Register("yun139", func(params drivers.Params) (drivers.Driver, error) {
+        auth := params["authorization"]
+        if auth == "" {
+            return nil, fmt.Errorf("missing authorization for yun139 driver")
+        }
+        return NewDriver(auth, params["root_id"]), nil
+    })
+}
```

#### `drivers/localfs/driver.go`

```diff
+func init() {
+    drivers.Register("localfs", func(params drivers.Params) (drivers.Driver, error) {
+        root := params["local_root"]
+        if root == "" {
+            root = params["root_path"]
+        }
+        if root == "" {
+            return nil, fmt.Errorf("missing local_root for localfs driver")
+        }
+        return NewDriver(root), nil
+    })
+}
```

### 3.4 [MODIFY] `drivers/factory/factory.go` — 薄适配层

switch-case 全部移除，替换为 `drivers.New()` 调用：

```go
package factory

import (
    "github.com/yinzhenyu/skills/qrypt/drivers"
    "github.com/yinzhenyu/skills/qrypt/internal/config"

    // Side-effect imports: register all built-in drivers.
    _ "github.com/yinzhenyu/skills/qrypt/drivers/localfs"
    _ "github.com/yinzhenyu/skills/qrypt/drivers/quark"
    _ "github.com/yinzhenyu/skills/qrypt/drivers/yun139"
)

// NewDriverFromConfig creates a Driver from the given DriveConfig.
func NewDriverFromConfig(cfg config.DriveConfig) (drivers.Driver, error) {
    return drivers.New(cfg.Type, configToParams(cfg))
}

// NewDriverFromType creates a Driver from a type string and MountParams.
func NewDriverFromType(driverType string, params config.MountParams) (drivers.Driver, error) {
    return drivers.New(driverType, mountParamsToParams(params))
}

func configToParams(cfg config.DriveConfig) drivers.Params {
    p := drivers.Params{}
    switch {
    case cfg.Quark != nil:
        p["cookie"] = cfg.Quark.Cookie
        p["root_path"] = cfg.Quark.RootPath
    case cfg.Yun139 != nil:
        p["authorization"] = cfg.Yun139.Authorization
        p["root_id"] = cfg.Yun139.RootID
    case cfg.LocalFS != nil:
        p["local_root"] = cfg.LocalFS.RootPath
    }
    return p
}

func mountParamsToParams(mp config.MountParams) drivers.Params {
    return drivers.Params{
        "cookie":        mp.Cookie,
        "authorization": mp.Authorization,
        "root_path":     mp.RootPath,
        "root_id":       mp.RootID,
        "local_root":    mp.LocalRoot,
    }
}
```

**关键点**：`_ "drivers/quark"` 等 blank import 触发 `init()` 注册。所有 switch-case 消失。

### 3.5 [MODIFY] `mobile/driver_factory.go` — 同步使用 registry

```diff
-import (
-    "github.com/yinzhenyu/skills/qrypt/drivers/quark"
-    "github.com/yinzhenyu/skills/qrypt/drivers/yun139"
-)
+import (
+    "github.com/yinzhenyu/skills/qrypt/drivers"
+
+    // Register drivers available on mobile.
+    _ "github.com/yinzhenyu/skills/qrypt/drivers/quark"
+    _ "github.com/yinzhenyu/skills/qrypt/drivers/yun139"
+    // localfs intentionally omitted on mobile
+)

 func (f *mobileDriverFactory) CreateDriver(ctx context.Context, cfg qrypt.SessionConfig) (drivers.Driver, error) {
     backendType, mountName := splitTypeMount(cfg.Type)
-    switch backendType {
-    case "quark":
-        cookie, err := f.creds.Get("cookie_" + mountName)
-        ...
-    case "yun139":
-        ...
-    }
+
+    // Derive credential key and param name from backend type.
+    credPrefix := credKeyPrefix(backendType)
+    cred, err := f.creds.Get(credPrefix + mountName)
+    if err != nil {
+        return nil, fmt.Errorf("get credential for %s mount %q: %w", backendType, mountName, err)
+    }
+
+    params := drivers.Params{
+        credParamName(backendType): cred,
+        "root_id":                  cfg.RootID,
+    }
+    return drivers.New(backendType, params)
 }
+
+func credKeyPrefix(backendType string) string {
+    switch backendType {
+    case "quark":
+        return "cookie_"
+    case "yun139":
+        return "auth_"
+    default:
+        return "cred_"
+    }
+}
+
+func credParamName(backendType string) string {
+    switch backendType {
+    case "quark":
+        return "cookie"
+    case "yun139":
+        return "authorization"
+    default:
+        return "credential"
+    }
+}
```

> **注意**：mobile 端的 credential → param 映射仍有 switch-case，但这不是 driver 构造逻辑的重复，而是 credential key 命名约定。如果未来这里也膨胀，可以让 driver 注册时同时声明 credential metadata。

### 3.6 [MODIFY] 消费端 — 使用 `drivers.CipherSetter`

三处匿名接口断言改为具名接口：

```diff
-if setter, ok := drv.(interface{ SetCipher(cipher.Cipher) }); ok {
+if setter, ok := drv.(drivers.CipherSetter); ok {
     setter.SetCipher(ciph)
 }
```

涉及文件：
- `internal/mount/manager.go:222`
- `internal/platform/core.go:38`
- `internal/platform/core.go:87`

---

## 4. Param Key 规范

为避免各 driver 包使用不一致的 key 名称：

| Key | 含义 | 使用者 |
|-----|------|--------|
| `cookie` | HTTP cookie 字符串 | quark |
| `authorization` | 认证 token/header | yun139 |
| `root_path` | 远端根路径 | quark |
| `root_id` | 远端根目录 ID | yun139 |
| `local_root` | 本地根路径 | localfs |

这些 key 只是 `map[string]string` 的约定，由各 driver 的 `Constructor` 自行校验。

---

## 5. 迁移检查清单

- [ ] 创建 `drivers/registry.go`
- [ ] 在 `drivers/driver.go` 新增 `CipherSetter` 接口
- [ ] 在 `quark/driver.go` 添加 `init()` 注册
- [ ] 在 `yun139/driver.go` 添加 `init()` 注册
- [ ] 在 `localfs/driver.go` 添加 `init()` 注册
- [ ] 重写 `drivers/factory/factory.go`（移除 switch-case）
- [ ] 重写 `mobile/driver_factory.go`（使用 `drivers.New`）
- [ ] 替换三处匿名 `SetCipher` 断言为 `drivers.CipherSetter`
- [ ] 更新 `drivers/factory/factory_test.go`
- [ ] `go build ./...` 通过
- [ ] `go test ./...` 通过（包括 `drivers/factory/`）
- [ ] `go vet ./...` 无警告

---

## 6. 新增 Driver 流程（重构后）

以假设的 `aliyundrive` 为例：

```
1. 新建 drivers/aliyundrive/driver.go
2. 实现 Driver + Writer + Uploader 接口
3. 添加 init() { drivers.Register("aliyundrive", ...) }
4. 在需要的 factory 中添加 blank import：
   - factory/factory.go: _ "qrypt/drivers/aliyundrive"
   - mobile/driver_factory.go: （如移动端需要）
5. config 中新增对应的 TOML section
```

无需修改 factory 的任何逻辑代码，只加一行 import。

---

## 7. 风险评估

| 风险 | 级别 | 缓解 |
|------|------|------|
| `init()` 注册顺序不可控 | 低 | 各 driver 注册独立，无依赖关系 |
| `Params map[string]string` 丢失类型安全 | 中 | 校验逻辑集中在各 Constructor 内部，错误消息清晰 |
| blank import 遗漏导致运行时 "unknown driver" | 中 | factory_test.go 覆盖所有 built-in driver 类型 |
| `cipher` 包导入引入 `drivers/driver.go` | 低 | `cipher` 包无外部依赖，不形成循环 |
