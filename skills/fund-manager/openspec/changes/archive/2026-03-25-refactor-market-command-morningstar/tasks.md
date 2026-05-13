## 1. 数据模型与解析重构 (morningstar_market.rs)

- [x] 1.1 更新 `IndexItem` 结构体，捕获 `w52h`, `w52l`, `status`, `cur`, `ts` 等字段。
- [x] 1.2 引入 `MarketCategory` 枚举和归一化的 `MarketItem` 结构体。
- [x] 1.3 更新 `WatchListData` 以包含 `exchangeRate`, `commodity`, `hotAssets` 数组。
- [x] 1.4 更新 `fetch_indices` 函数，支持根据 `MarketCategory` 过滤并返回全类别数据。
- [x] 1.5 编写解析逻辑的单元测试，验证各分类数据的正确性。

## 2. UI 渲染组件开发 (main.rs / UI utils)

- [x] 2.1 实现 `render_sparkline` 函数，将价格序列转换为 ASCII 字符画。
- [x] 2.2 实现 `render_range_bar` 函数，生成 52 周水位对比进度条。
- [x] 2.3 在 `main.rs` 中重构表格渲染逻辑，支持动态列（Detail 视图和 Trend 视图）。
- [x] 2.4 实现根据 `status` 字段显示 🟢/🔴 状态灯的逻辑。

## 3. CLI 指令与参数增强 (main.rs)

- [x] 3.1 扩展 `fund market` 参数解析，支持 `--fx`, `--com`, `--index`, `--hot` 标志。
- [x] 3.2 增加 `--detail` (-d) 和 `--trend` (-t) 参数支持。
- [x] 3.3 修改主逻辑，支持按类别合并或过滤显示结果。

## 4. 配置与本地化支持 (config.rs)

- [x] 4.1 在 `Config` 结构体中增加 `default_market_items` 配置字段。
- [x] 4.2 实现默认行情加载逻辑：如果用户未传参且配置了 watchlist，则加载指定项。
- [x] 4.3 完善本地化：确保所有资产名称在中文环境下显示正常。

## 5. 验证与集成测试

- [x] 5.1 运行集成测试，验证 `fund market` 各种参数下的输出格式。
- [x] 5.2 验证外汇及黄金趋势图在不同终端宽度下的表现。
- [x] 5.3 检查错误处理逻辑：API 超时或解析失败时的友好提示。
