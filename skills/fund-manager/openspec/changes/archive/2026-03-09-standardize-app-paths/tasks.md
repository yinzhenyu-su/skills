## 1. 路径重构 (Standardization)

- [x] 1.1 修改 `src/config.rs` 中的 `get_app_dir`，引入平台区分逻辑。
- [x] 1.2 在 Unix (macOS/Linux) 分支实现 `home_dir` + `.config` 构建逻辑。
- [x] 1.3 在 Windows 分支保留 `config_dir` 构建逻辑。

## 2. 测试与验证

- [x] 2.1 更新 `config.rs` 中的单元测试，断言不同平台下的路径模式。
- [x] 2.2 运行现有的所有集成测试，确保环境变量优先级不受影响。
