## MODIFIED Requirements

### Requirement: 运行日志与确认提示中文显示
系统 SHALL 使用中文记录运行状态、错误日志以及与用户的交互确认提示。汉化范围 SHALL 覆盖到底层逻辑报错、详细背景说明以及批量导入过程中的分行定位信息。

#### Scenario: 批量导入 CSV 失败
- **WHEN** 导入 CSV 文件且第 3 行金额无效
- **THEN** 系统 SHALL 输出：`失败原因：金额必须为正数 (CSV 第 3 行)`

#### Scenario: 深度分析背景说明
- **WHEN** 查看基金深度报告且存在较大的投资者缺口（Gap）
- **THEN** 系统 SHALL 输出：`投资者常因追涨杀跌（择时错误）导致亏损`

#### Scenario: 钱包环境异常
- **WHEN** 未选择活跃钱包执行操作
- **THEN** 系统 SHALL 报错：`❌ 错误：未选择活跃钱包。请使用 'fund wallet use <名称>' 或指定 --wallet 参数。`
