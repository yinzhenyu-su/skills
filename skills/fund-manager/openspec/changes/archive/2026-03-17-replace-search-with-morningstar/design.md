## Context

目前 `fund-manager` 使用同花顺的模糊搜索 API 来联想基金代码。该 API 返回 JSONP 格式，需要使用正则提取数据，且返回信息较少。晨星提供的 `fund-cache` 接口返回标准的结构化 JSON，数据质量更高，且包含基金类型。

## Goals / Non-Goals

**Goals:**
- 将模糊搜索 Provider 迁移至晨星。
- 提升搜索结果的稳定性。
- 在搜索结果中引入基金类型信息。

**Non-Goals:**
- 不改变现有的 `resolver` 逻辑框架。
- 暂时不利用晨星 ID (`fundClassId`) 进行进一步分析（预留字段）。

## Decisions

### 1. 移除 `ThsSearchProvider` 并新增 `MorningstarSearchProvider`
晨星 API 更加简洁，不需要复杂的正则解析。
- **URL**: `https://www.morningstar.cn/cn-api/public/v1/fund-cache/{keyword}`
- **解析方式**: 使用 `serde_json` 直接反序列化。

### 2. 扩展 `SearchResult` 结构
为了利用晨星提供的类型信息，我们将 `SearchResult` 结构体进行扩展：
```rust
pub struct SearchResult {
    pub code: String,
    pub name: String,
    pub fund_type: Option<String>,
}
```

### 3. 修改 `resolver.rs` 的交互逻辑
在多结果选择时，格式化输出以包含基金类型，提升用户识别准确度。

## Risks / Trade-offs

- **[Risk] 晨星接口响应延迟** → **Mitigation**: 晨星 API 响应通常很快，且返回结果数量可控。
- **[Trade-off] 失去同花顺的部分特有基金** → **Mitigation**: 晨星涵盖了国内绝大多数公募基金，且支持联接基金，覆盖度足以满足需求。
