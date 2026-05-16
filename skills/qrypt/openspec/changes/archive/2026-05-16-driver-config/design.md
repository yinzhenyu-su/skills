## Context

当前 `internal/config.Config` 结构体将 Quark 配置作为顶级字段：

```go
type Config struct {
    Quark      QuarkConfig      `toml:"quark"`      // 硬编码
    Encryption EncryptionConfig `toml:"encryption"`
    Cache      CacheConfig      `toml:"cache"`
    Mount      MountConfig      `toml:"mount"`
    Sync       SyncConfig       `toml:"sync"`
    Log        LogConfig        `toml:"log"`
}
```

Drive Abstraction Layer 已完成 `drive.Driver` 接口定义和 Quark/yun139 两个驱动实现。但配置层仍只能描述 Quark——`config.go` 不知道 `yun139`、无法表达其 `authorization` 参数。

## Goals / Non-Goals

**Goals:**
- 配置层支持任意驱动，驱动参数用独立 TOML 节表达
- 工厂函数 `NewDriverFromConfig` 按 `drive.type` 实例化对应驱动
- 向后兼容：现有 `qrypt.toml` 文件不修改也能工作
- 加密配置（`password`/`salt`）与驱动配置解耦——加密是跨驱动能力，不是 Quark 专属

**Non-Goals:**
- 不改 CLI 工具中仍用旧 `internal/quark/` 包的部分（ls/cat/rm/mv/find/pull）
- 不改 `drive/localfs` 或 `drive/quarkmock`（无配置变化）
- 不改 FUSE/sync/cache 等公共配置项

## Decisions

### D1: Config 结构：指针字段 + type 分发

```go
type Config struct {
    Drive      DriveConfig      `toml:"drive"`
    Encryption EncryptionConfig `toml:"encryption"`
    Cache      CacheConfig      `toml:"cache"`
    Mount      MountConfig      `toml:"mount"`
    Sync       SyncConfig       `toml:"sync"`
    Log        LogConfig        `toml:"log"`
}

type DriveConfig struct {
    Type   string          `toml:"type"`   // "quark" | "yun139"
    Quark  *QuarkOptions   `toml:"quark"`
    Yun139 *Yun139Options  `toml:"yun139"`
}

type QuarkOptions struct {
    Cookie   string `toml:"cookie"`
    RootPath string `toml:"root_path"`
}

type Yun139Options struct {
    Authorization string `toml:"authorization"`
    RootID        string `toml:"root_id"`
}
```

TOML 表现：

```toml
[drive]
type = "quark"

[drive.quark]
cookie = "..."
root_path = "/Test"
```

新增驱动时：向 `DriveConfig` 加一个 `*XXXOptions` 指针字段。

### D2: 向后兼容——默认 fallback 到 Quark

过渡期内，允许两种配置格式共存：

```go
func LoadConfig(path string) (*Config, error) {
    config := DefaultConfig()
    // ... 解析 TOML ...
    
    // 兼容：如果 drive.type 未设置但旧 [quark] 节存在
    if config.Drive.Type == "" && legacyQuarkNonEmpty(config) {
        config.Drive.Type = "quark"
        config.Drive.Quark = &QuarkOptions{
            Cookie:   legacy.Quark.Cookie,
            RootPath: legacy.Quark.RootPath,
        }
    }
    return config, nil
}
```

保留旧 `QuarkConfig` 字段，用 `Deprecated` 标记。两个版本后移除。

### D3: 工厂函数

```go
package drive

func NewDriverFromConfig(cfg DriveConfig) (Driver, error) {
    switch cfg.Type {
    case "quark":
        if cfg.Quark == nil {
            return nil, errors.New("missing quark config")
        }
        return quark.NewDriver(cfg.Quark.Cookie, cfg.Quark.RootPath), nil
    case "yun139":
        if cfg.Yun139 == nil {
            return nil, errors.New("missing yun139 config")
        }
        return yun139.NewDriver(cfg.Yun139.Authorization, cfg.Yun139.RootID), nil
    default:
        return nil, fmt.Errorf("unknown driver type: %s", cfg.Type)
    }
}
```

放在 `internal/drive/factory/factory.go`（因 `drive/quark` 导入 `drive` 包导致 import cycle，无法放在 `drive` 包根目录）。

### D4: CLI flag 调整

`mount.go` 的 `--cookie` / `--password` flag 行为调整：

- 新增 `--drive-type` flag（覆盖配置文件的 `drive.type`）
- `--cookie` 保持兼容：type=quark 时覆盖 `QuarkOptions.Cookie`
- `--password` / `--salt` 保持不变（始终设置 `EncryptionConfig`）
- 未来驱动专用 flag 如 `--yun139-auth` 暂不加，等 139 真实使用再说

## Risks / Trade-offs

| 风险 | 缓解 |
|------|------|
| **旧配置格式用户升级后配置失效** | 兼容层自动迁移，打印一条 notice |
| **新驱动加 Options 指针字段时容易漏掉工厂函数 case** | 编译检查：工厂函数 switch 上加 `default` panic，运行时立即暴露 |
| **配置结构嵌套加深** | 只有两层（`drive` → `drive.xxx`），不继续加深 |

## Migration Plan

1. `config.go`：新增 `DriveConfig` + `QuarkOptions` + `Yun139Options`，保留旧字段
2. `drive/factory.go`：新建 `NewDriverFromConfig` 工厂函数
3. `LoadConfig`：添加向后兼容迁移逻辑
4. `mount.go` + `util.go`：使用工厂函数替代手动驱动创建
5. 更新 `qrypt.toml` 示例配置
6. 验证：旧配置格式仍可用，新配置格式可用
