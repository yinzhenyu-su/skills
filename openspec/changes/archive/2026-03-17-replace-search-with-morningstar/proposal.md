## Why

目前使用的同花顺搜索接口返回的是 JSONP 格式，解析逻辑脆弱且依赖正则提取。此外，同花顺接口返回的数据仅包含代码和名称。替换为晨星接口将提供更稳定的结构化 JSON 响应，并能同时获取基金类型和晨星内部 ID，为后续功能扩展打下基础。

## What Changes

- **移除同花顺搜索支持**: 删除 `ThsSearchProvider` 及其相关的正则解析逻辑。
- **引入晨星搜索支持**: 新增 `MorningstarSearchProvider`，对接晨星 `fund-cache` API。
- **扩展数据模型**: 更新 `SearchResult` 结构以包含基金类型，并在 `resolver` 中更好地利用这些信息。
- **增强搜索能力**: 晨星接口支持更精准的中文名称和代码联想，提升远程搜索的成功率。

## Capabilities

### New Capabilities
- `morningstar-search`: 提供基于晨星官方 API 的基金搜索和建议功能。

### Modified Capabilities
- `smart-search`: 将底层搜索实现从同花顺切换到晨星。

## Impact

- `skills/fund-manager/src/provider/ths_search.rs`: 被移除。
- `skills/fund-manager/src/provider/morningstar_search.rs`: 新增。
- `skills/fund-manager/src/resolver.rs`: 修改引用，切换 Provider 实例。
- `skills/fund-manager/src/provider/mod.rs`: 更新模块导出。
