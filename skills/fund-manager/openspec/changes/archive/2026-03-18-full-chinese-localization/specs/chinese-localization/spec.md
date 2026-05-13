## ADDED Requirements

### Requirement: 命令行接口 (CLI) 中文描述
系统 SHALL 将所有命令行子命令的 `about`, `help` 和 `long_about` 描述替换为中文，同时保留一级子命令的英文名称以维持操作习惯。

#### Scenario: 查看全局帮助
- **WHEN** 用户执行 `fund --help`
- **THEN** 系统展示的关于工具本身的描述和所有子命令（wallet, fund, status等）的简要说明必须为中文。

#### Scenario: 查看特定子命令帮助
- **WHEN** 用户执行 `fund buy --help`
- **THEN** 系统展示的买入操作示例、参数说明（如 --money, --shares等）必须为中文。

### Requirement: 报表表头 (Table Headers) 中文显示
系统在终端输出的所有表格数据 SHALL 使用中文表头。

#### Scenario: 查看持仓状态表格
- **WHEN** 用户执行 `fund status`
- **THEN** 表格表头 SHALL 显示为：基金, 份额, 持仓成本, 当前净值, 当前市值, 盈亏, 收益率。

#### Scenario: 查看基金列表表格
- **WHEN** 用户执行 `fund fund list`
- **THEN** 表格表头 SHALL 显示为：代码, 名称, 类型, 风险等级, 基金经理, 最后同步。

### Requirement: 运行日志与确认提示中文显示
系统 SHALL 使用中文记录运行状态、错误日志以及与用户的交互确认提示。

#### Scenario: 删除基金确认
- **WHEN** 用户执行 `fund fund delete 000300`
- **THEN** 系统 SHALL 弹出提示：您确定要删除基金 000300 及其所有交易记录吗？[y/N]:

#### Scenario: 导入成功提示
- **WHEN** 批量导入操作完成
- **THEN** 系统 SHALL 输出：🚀 导入流程已完成！ ✅ 成功：N ❌ 失败：M。
