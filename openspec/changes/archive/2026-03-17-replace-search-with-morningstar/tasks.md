## 1. 定义新 Provider

- [x] 1.1 创建 `skills/fund-manager/src/provider/morningstar_search.rs`，定义 `MorningstarSearchProvider` 结构体。
- [x] 1.2 在 `morningstar_search.rs` 中实现 `SearchResult` 的新定义，包含 `fund_type`。
- [x] 1.3 实现晨星 API 的反序列化逻辑 (`_meta`, `data` 等)。

## 2. 移除旧 Provider

- [x] 2.1 删除 `skills/fund-manager/src/provider/ths_search.rs`。
- [x] 2.2 在 `skills/fund-manager/src/provider/mod.rs` 中移除 `ths_search` 模块导出，并添加 `morningstar_search`。

## 3. 集成与适配

- [x] 3.1 修改 `skills/fund-manager/src/resolver.rs`，将 `ThsSearchProvider` 替换为 `MorningstarSearchProvider`。
- [x] 3.2 适配 `resolve_fund` 中的交互逻辑，在选择列表中显示基金类型。
- [x] 3.3 确保代码编译通过并处理任何由此产生的类型不匹配问题。

## 4. 验证与测试

- [x] 4.1 在 `morningstar_search.rs` 中编写单元测试，验证 JSON 解析逻辑。
- [x] 4.2 手动运行 `fund inspect` 并触发远程搜索，验证晨星建议功能是否正常。
