# Driver 参数自描述（ParamSpec）

将 driver 参数元信息从 config 层硬编码提升到 driver 注册层，为未来表单生成和动态验证做准备。

## 现状问题

`MountParams` 是一个按 driver 拼凑的 struct，所有 driver 的字段堆在一起：

```go
type MountParams struct {
    Cookie        string `toml:"cookie"`        // quark only
    RootPath      string `toml:"root_path"`     // quark only
    Authorization string `toml:"authorization"` // yun139 only
    RootID        string `toml:"root_id"`       // yun139 only
    LocalRoot     string `toml:"local_root"`    // localfs only
}
```

新增 driver 时需要改 `MountParams` struct + validation switch-case + `RootPathForMount` switch-case + `SessionKeyForMount` switch-case + `TomlCredentialStore` switch-case。

## 目标

1. `MountParams` → `map[string]string`（TOML 兼容，无需改配置文件格式）
2. 每个 driver 注册时声明 `[]ParamSpec`（名称、是否必填、帮助文本等）
3. 验证、表单生成都从注册信息动态获取
4. 消除 config 层所有 driver-specific 的 switch-case

## User Review Required

> [!IMPORTANT]
> `MountParams` 从 struct 变为 `map[string]string` 后，代码中所有 `m.Params.Cookie` 要改为 `m.Params["cookie"]`。
> 影响范围约 30+ 处（含测试）。TOML 文件格式**不变**，用户无感。

> [!WARNING]
> `SessionKeyForMount` 中的 credential key 提取目前按 driver type 写死。
> 方案中通过 driver 注册时声明 `CredentialKey` 字段来消除，但这改变了 `SessionKey` 的生成逻辑。
> 需要确认：现有的 session cache 是否会因 key 变化而失效？（答：SessionKey 是内存结构，不持久化，重启后重建，无影响。）

## Open Questions

> [!IMPORTANT]
> `RootPathForMount` 目前按 type 选不同的 param key（quark→root_path, yun139→root_id, localfs→local_root）。
> 建议让每个 driver 注册时声明一个 `RootKey` 字段，`RootPathForMount` 改为 `m.Params[info.RootKey]`。
> 你是否同意这个方式？

## Proposed Changes

### Component 1: Driver Registry

#### [MODIFY] [registry.go](file:///Users/yinzhenyu/Code/github/skills/skills/qrypt/drivers/registry.go)

新增 `ParamSpec` 和 `DriverMeta` 类型，修改注册 API：

```go
// ParamSpec describes a single driver parameter.
type ParamSpec struct {
    Key          string // TOML/JSON key, e.g. "cookie"
    Required     bool
    Help         string // human-readable description
    Default      string
    Type         string // "string" | "select" | "bool"（扩展用）
    Options      string // comma-separated for select type
}

// DriverMeta holds the constructor and parameter metadata for a registered driver.
type DriverMeta struct {
    Ctor           Constructor
    Params         []ParamSpec
    RootKey        string // which param key holds the root path/ID
    CredentialKey  string // which param key holds the credential for session dedup
}

func Register(name string, meta DriverMeta) { ... }
func GetMeta(name string) (DriverMeta, bool) { ... }
```

`New()` 函数签名不变（仍然是 `New(name string, params Params) (Driver, error)`），但内部调用 `meta.Ctor(params)`。

---

### Component 2: Driver Registration

#### [MODIFY] [quark/driver.go](file:///Users/yinzhenyu/Code/github/skills/skills/qrypt/drivers/quark/driver.go)

```go
func init() {
    drivers.Register("quark", drivers.DriverMeta{
        Ctor: func(params drivers.Params) (drivers.Driver, error) {
            cookie := params["cookie"]
            if cookie == "" {
                return nil, fmt.Errorf("missing cookie for quark driver")
            }
            return NewDriver(cookie, params["root_path"]), nil
        },
        Params: []drivers.ParamSpec{
            {Key: "cookie", Required: true, Help: "Quark cookie string"},
            {Key: "root_path", Help: "Remote root folder path", Default: "/"},
        },
        RootKey:       "root_path",
        CredentialKey: "cookie",
    })
}
```

#### [MODIFY] [yun139/driver.go](file:///Users/yinzhenyu/Code/github/skills/skills/qrypt/drivers/yun139/driver.go)

```go
func init() {
    drivers.Register("yun139", drivers.DriverMeta{
        Ctor: func(params drivers.Params) (drivers.Driver, error) { ... },
        Params: []drivers.ParamSpec{
            {Key: "authorization", Required: true, Help: "139 authorization token"},
            {Key: "root_id", Help: "Remote root folder ID", Default: "/"},
        },
        RootKey:       "root_id",
        CredentialKey: "authorization",
    })
}
```

#### [MODIFY] [localfs/driver.go](file:///Users/yinzhenyu/Code/github/skills/skills/qrypt/drivers/localfs/driver.go)

```go
func init() {
    drivers.Register("localfs", drivers.DriverMeta{
        Ctor: func(params drivers.Params) (drivers.Driver, error) { ... },
        Params: []drivers.ParamSpec{
            {Key: "local_root", Required: true, Help: "Local filesystem root path"},
        },
        RootKey:       "local_root",
        CredentialKey: "local_root",
    })
}
```

---

### Component 3: Config

#### [MODIFY] [config.go](file:///Users/yinzhenyu/Code/github/skills/skills/qrypt/internal/config/config.go)

1. `MountParams` 改为 `map[string]string`：

```diff
-type MountParams struct {
-    Cookie        string `toml:"cookie"`
-    RootPath      string `toml:"root_path"`
-    Authorization string `toml:"authorization"`
-    RootID        string `toml:"root_id"`
-    LocalRoot     string `toml:"local_root"`
-}
+// MountParams holds driver-specific configuration as a flat key-value map.
+// Keys correspond to the driver's registered ParamSpec.Key values.
+type MountParams = map[string]string
```

注意：`map[string]string` 可以直接被 TOML decoder 解析 `[mounts.params]` section，**用户配置文件格式不变**。

2. `RootPathForMount` 改用 `drivers.GetMeta`：

```diff
 func RootPathForMount(m MountInstance) string {
-    switch m.Type {
-    case "quark":
-        if m.Params.RootPath != "" { return m.Params.RootPath }
-    case "yun139":
-        ...
+    if meta, ok := drivers.GetMeta(m.Type); ok && meta.RootKey != "" {
+        if v := m.Params[meta.RootKey]; v != "" {
+            return v
+        }
     }
     return "/"
 }
```

3. 删除 legacy `DriveConfig`/`QuarkOptions`/`Yun139Options`/`LocalFSOptions`（如仍有使用者则保留）。

#### [MODIFY] [validation.go](file:///Users/yinzhenyu/Code/github/skills/skills/qrypt/internal/config/validation.go)

L74-94 的 driver 参数验证改为动态：

```go
if meta, ok := drivers.GetMeta(m.Type); ok {
    for _, spec := range meta.Params {
        val := m.Params[spec.Key]
        if val == "" && spec.Required {
            r.addCheck(prefix+".params."+spec.Key, "error", spec.Key+" is required for "+m.Type+" driver")
        } else if val != "" {
            r.addCheck(prefix+".params."+spec.Key, "ok", "set")
        }
    }
}
```

---

### Component 4: Consumer Updates

#### [MODIFY] [factory/factory.go](file:///Users/yinzhenyu/Code/github/skills/skills/qrypt/drivers/factory/factory.go)

`mountParamsToParams` 变为直接传递（类型相同了）：

```diff
 func NewDriverFromType(driverType string, params config.MountParams) (drivers.Driver, error) {
-    return drivers.New(driverType, mountParamsToParams(params))
+    return drivers.New(driverType, drivers.Params(params))
 }
-
-func mountParamsToParams(mp config.MountParams) drivers.Params { ... }
```

#### [MODIFY] [session.go](file:///Users/yinzhenyu/Code/github/skills/skills/qrypt/internal/mount/session.go)

`SessionKeyForMount` 改为动态：

```diff
 func SessionKeyForMount(rc *config.ResolvedMountConfig) qrypt.SessionKey {
-    switch rc.Type {
-    case "localfs":
-        return qrypt.SessionKey{Type: "localfs", CredKey: hashShort(rc.Params.LocalRoot)}
-    case "quark":
-        ...
+    if meta, ok := drivers.GetMeta(rc.Type); ok && meta.CredentialKey != "" {
+        return qrypt.SessionKey{Type: rc.Type, CredKey: hashShort(rc.Params[meta.CredentialKey])}
     }
+    return qrypt.SessionKey{Type: rc.Type, CredKey: rc.Name}
 }
```

#### [MODIFY] [platform.go](file:///Users/yinzhenyu/Code/github/skills/skills/qrypt/internal/platform/platform.go)

`TomlCredentialStore.Get` 改为动态：

```diff
 func (s *TomlCredentialStore) Get(key string) (string, error) {
     for _, m := range s.cfg.Mounts {
-        if "cookie_"+m.Name == key {
-            return m.Params.Cookie, nil
-        }
-        if "auth_"+m.Name == key {
-            return m.Params.Authorization, nil
+        if meta, ok := drivers.GetMeta(m.Type); ok && meta.CredentialKey != "" {
+            prefix := credKeyPrefix(m.Type)
+            if prefix+m.Name == key {
+                return m.Params[meta.CredentialKey], nil
+            }
         }
     }
```

#### [MODIFY] CLI commands

`cmd/qrypt/ls.go`, `mkdir.go`, `mv.go`, `rm.go` 中的 `mountCfg.Params.RootPath` 改为 `mountCfg.Params["root_path"]`。

#### [MODIFY] 所有测试

`config.MountParams{Cookie: "test"}` 改为 `config.MountParams{"cookie": "test"}`。

---

## Verification Plan

### Automated Tests
```bash
go build ./...
go test ./... -count=1
go vet ./...
```

### Manual Verification
- 确认现有 `qrypt.toml` 无需任何修改即可被正确解析
- `qrypt config validate` 输出正确的 driver 参数验证结果
