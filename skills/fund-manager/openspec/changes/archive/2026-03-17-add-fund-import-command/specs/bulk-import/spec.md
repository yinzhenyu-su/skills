## ADDED Requirements

### Requirement: CSV File Import
系统必须支持通过 `--file` 参数接收并解析 CSV 文件，文件应包含基金名称/代码和金额两列。

#### Scenario: Valid CSV format
- **WHEN** 用户执行 `fund import --file data.csv`，且文件包含有效的名称和正数金额
- **THEN** 系统解析这些行并逐一处理导入

#### Scenario: Invalid CSV data
- **WHEN** CSV 行缺少金额或金额无法解析为正数
- **THEN** 系统跳过该行，并在最终的失败报告中记录原因

### Requirement: Positional Arguments Import
系统必须支持通过变长参数对直接在命令行中导入。

#### Scenario: Valid positional pairs
- **WHEN** 用户执行 `fund import "基金A" 1000 "基金B" 2000`
- **THEN** 系统解析出两对数据并逐一处理

#### Scenario: Unmatched parameters
- **WHEN** 用户执行 `fund import "基金A" 1000 "基金B"` (缺少最后一个金额)
- **THEN** 系统报错退出，提示参数必须成对出现

### Requirement: Failure Reporting
批量导入完成后，系统必须展示结构化的报告。

#### Scenario: Mixed success and failures
- **WHEN** 导入过程中有成功项和失败项
- **THEN** 系统在结束时打印成功数量，并以表格形式展示失败项的行号、输入内容和失败原因
