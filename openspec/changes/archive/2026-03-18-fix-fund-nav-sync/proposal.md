## Why

当前系统在同步基金净值时存在严重缺陷：由于历史净值同步接口的 `pageSize` 参数过大（1000），导致天天基金接口拒绝请求并返回空数据；同时，数据聚合器（Aggregator）静默忽略了所有 Provider 的错误。这导致用户在执行 `fund list` 或 `fund sync` 后，无法查看到任何基金的最新净值，且系统未给出明确的错误提示。

## What Changes

- **修复接口请求限制**：将历史净值同步的单次分页大小（`pageSize`）从 1000 降低至 20（经测试验证的安全值），并支持多页连续抓取以确保数据完整性。
- **增强 Provider 鲁棒性与错误可见性**：
  - 在 `Aggregator` 中增加错误收集和日志输出，避免静默失败。
  - 改进 `EastmoneyJsProvider` 的字符编码处理，支持 `UTF-8,gbk` 等混合编码声明，防止解码异常。
- **规范请求头管理**：统一并验证天天基金 API 所需的 `Referer` 字段，确保请求不会因安全策略被拦截。

## Capabilities

### New Capabilities
- `nav-sync-reliability`: 增强基金净值同步的可靠性，包括分页抓取、编码处理和错误上报机制。

### Modified Capabilities
- `smart-nav-lookup`: 修改获取历史净值的逻辑，支持分页和更小的 `pageSize` 以适应接口限制。
- `unified-http-client`: 规范并统一请求头（如 `Referer`）的管理。

## Impact

- **Affected Code**: `sync.rs`, `provider/aggregator.rs`, `provider/eastmoney_lsjz.rs`, `provider/eastmoney_js.rs`, `provider/mod.rs`。
- **User Experience**: 用户将能够成功同步并看到最新的基金净值，且在网络或接口异常时能得到明确的错误反馈。
