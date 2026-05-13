# Unified HTTP Client

## Requirements

1.  **统一入口**: 提供一个中心化的 `get_client()` 函数。
2.  **User-Agent 配置**: 必须从 `AppConfig` 读取 UA，若为空则回退到稳定的默认 UA。
3.  **Referer 默认注入**: 所有请求应默认包含 `Referer: https://fundf10.eastmoney.com/`，以应对东方财富的防爬规则。
4.  **性能优化**: (后续可扩展) 尽可能复用同一 Client 以利用连接池。

## Interface

### Config
- `DEFAULT_USER_AGENT`: `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36`

### Provider Utility
- `get_client(ua: Option<String>) -> reqwest::Client`: 构建并返回配置好的 Client。
