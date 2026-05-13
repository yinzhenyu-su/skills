## 1. 修复 Provider 接口抓取限制

- [x] 1.1 修改 `EastmoneyLsjzProvider::fetch_range`：将 `pageSize` 从 1000 改为 20。
- [x] 1.2 在 `EastmoneyLsjzProvider` 中实现基于 `TotalCount` 的分页循环抓取逻辑。
- [x] 1.3 验证 `000513` 基金的历史同步是否能成功抓取多个日期的数据。

## 2. 增强 Aggregator 鲁棒性

- [x] 2.1 修改 `Aggregator` 结构，增加错误收集字段（如 `Vec<String>`）。
- [x] 2.2 修改 `Aggregator::fetch_at_date`：记录各个 Provider 的失败详情。
- [x] 2.3 修改 `Aggregator` 返回逻辑：当没有任何 Provider 成功时返回汇总错误，部分失败时通过 `log` 或打印警告提示。

## 3. 改进编码解析与请求头

- [x] 3.1 引入 `encoding_rs` 库或手动处理 JS 响应的 GBK 解码（已手动处理混合编码）。
- [x] 3.2 验证 `EastmoneyJsProvider` 对 `UTF-8,gbk` 混合 Content-Type 的解析成功率。
- [x] 3.3 统一 `provider/mod.rs` 中 `build_http_client` 的默认 Referer，确保各 Provider 共用。

## 4. 最终验证

- [x] 4.1 运行 `fund fund sync 000513 --start 2026-03-01` 验证最新净值和历史记录是否均已成功入库。
- [x] 4.2 执行 `fund list` 检查“最新净值”列是否已正确显示。
- [x] 4.3 模拟网络断开，验证 `Aggregator` 是否给出了清晰的错误反馈。
