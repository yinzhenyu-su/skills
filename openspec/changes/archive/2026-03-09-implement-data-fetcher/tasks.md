## 1. 环境准备 (Infrastructure)

- [x] 1.1 在 `Cargo.toml` 中集成 `scraper`, `regex`, `reqwest`, `serde`, `serde_json`, `mockall`。
- [x] 1.2 创建 `src/provider/` 目录，定义统一的 `Provider` Trait。

## 2. JS 接口解析 (TDD: data-provider-js)

- [x] 2.1 **[RED]** 编写解析 `jsonpgz` 原始响应的单元测试（包含各种字符编码情况）。
- [x] 2.2 **[GREEN]** 实现 `regex` 剥壳和 `serde_json` 反序列化。
- [x] 2.3 **[REFACTOR]** 将正则常量化，优化字符串切片性能。

## 3. HTML 费率解析 (TDD: data-provider-html)

- [x] 3.1 **[RED]** 编写单元测试，使用硬编码的 HTML 片段验证 `.nowPrice` 选择器能否提取费率。
- [x] 3.2 **[GREEN]** 使用 `scraper` 实现 CSS 选择器提取逻辑。
- [x] 3.3 **[RED]** 编写异常文本处理测试（例如 "免手续费", "---"）。
- [x] 3.4 **[GREEN]** 实现鲁棒的数据清洗逻辑。

## 4. 数据聚合与弹性 (TDD: data-aggregator)

- [x] 4.1 **[RED]** 编写测试验证当 JS 成功但 HTML 失败时的“数据降级”合并逻辑。
- [x] 4.2 **[GREEN]** 实现异步并发调用逻辑，并在合并时处理 `Option` 缺失字段。
- [x] 4.3 **[REFACTOR]** 引入 `tokio::time::timeout` 确保全局请求不卡死。

## 5. 抓取逻辑最终验收

- [x] 5.1 编写针对真实 URL 的集成测试 (带 Skip 标记，用于本地环境调试)。
- [x] 5.2 确保所有 Provider 的错误返回均有清晰的 `Display` 实现。
