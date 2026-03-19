## ADDED Requirements

### Requirement: CSV Import Format
系统必须支持解析 CSV 文件，文件应包含基金名称、持有金额、持有收益三列，不支持日期列（统一使用系统当前日期）。

#### Scenario: Valid CSV with standard columns
- **WHEN** 用户执行 `fund import-holding --file holdings.csv`，文件内容为 `基金名称,持有金额,持有收益\n中欧医疗,11000,1000`
- **THEN** 系统解析出三列数据：基金名称="中欧医疗"，持有金额=11000，持有收益=1000

#### Scenario: CSV with extra whitespace
- **WHEN** CSV 某行内容为 `  中欧医疗  ,  11000  ,  1000  `
- **THEN** 系统去除首尾空白后解析

#### Scenario: Invalid CSV missing columns
- **WHEN** CSV 某行内容为 `中欧医疗,11000`（缺少持有收益列）
- **THEN** 系统跳过该行并在报告中记录原因

#### Scenario: Invalid holding amount (non-numeric)
- **WHEN** CSV 某行内容为 `中欧医疗,abc,1000`
- **THEN** 系统跳过该行并在报告中记录原因

#### Scenario: Negative holding profit (loss)
- **WHEN** CSV 某行内容为 `中欧医疗,9000,-1000`（亏损情况）
- **THEN** 系统正常解析，持有收益=-1000，总成本=持有金额-持有收益=10000

### Requirement: Cost and Shares Calculation
系统必须根据持有金额、持有收益和当前净值反推持有份额、总成本和成本价。

#### Scenario: Normal profit calculation
- **WHEN** 持有金额=11000，持有收益=1000，当前净值=1.1
- **THEN** 系统计算：持有份额=11000/1.1=10000，总成本=11000-1000=10000，成本价=10000/10000=1.0

#### Scenario: Loss calculation
- **WHEN** 持有金额=9000，持有收益=-1000，当前净值=0.9
- **THEN** 系统计算：持有份额=9000/0.9=10000，总成本=9000-(-1000)=10000，成本价=10000/10000=1.0

#### Scenario: Zero holding profit
- **WHEN** 持有金额=10000，持有收益=0，当前净值=1.0
- **THEN** 系统计算：持有份额=10000/1.0=10000，总成本=10000-0=10000，成本价=10000/10000=1.0

### Requirement: NAV Lookup and Failure Handling
导入时必须成功获取基金当前净值，否则跳过该记录。

#### Scenario: NAV lookup successful
- **WHEN** 解析出基金"中欧医疗"后，系统调用 NAV 查询接口
- **THEN** 获取到当前净值 NAV=1.1，继续计算

#### Scenario: NAV lookup failed
- **WHEN** 解析出基金"中欧医疗"后，NAV 查询失败（网络错误或基金代码不存在）
- **THEN** 系统跳过该行并在报告中记录原因

### Requirement: Fund Name Resolution
系统必须能够将基金名称解析为基金代码。

#### Scenario: Fund name resolves to unique code
- **WHEN** 基金名称"中欧医疗健康混合A"可以唯一匹配到代码"000312"
- **THEN** 系统使用代码"000312"进行 NAV 查询

#### Scenario: Fund name not found
- **WHEN** 基金名称无法匹配任何已知基金
- **THEN** 系统跳过该行并在报告中记录原因

### Requirement: Import Modes (Merge vs Override)
系统必须支持两种导入模式处理已存在的基金。

#### Scenario: Merge mode (default) - fund not exists
- **WHEN** 用户执行 `fund import-holding --file holdings.csv`（默认 merge 模式）
- **AND** 基金"中欧医疗"在钱包中不存在
- **THEN** 系统创建新的 holdings_import 记录

#### Scenario: Merge mode - fund already exists
- **WHEN** 用户执行 `fund import-holding --file holdings.csv`（默认 merge 模式）
- **AND** 基金"中欧医疗"在钱包中已存在导入记录
- **THEN** 系统跳过该行，不更新现有记录

#### Scenario: Override mode - fund not exists
- **WHEN** 用户执行 `fund import-holding --file holdings.csv --override`
- **AND** 基金"中欧医疗"在钱包中不存在
- **THEN** 系统创建新的 transaction_log 记录（type='import'）

#### Scenario: Override mode - fund already exists
- **WHEN** 用户执行 `fund import-holding --file holdings.csv --override`
- **AND** 基金"中欧医疗"在钱包中已存在导入记录
- **THEN** 系统删除旧的 import 类型 transaction_log 记录并创建新记录

### Requirement: Wallet Specification
导入时可以指定目标钱包。

#### Scenario: Import with explicit wallet
- **WHEN** 用户执行 `fund import-holding --file holdings.csv --wallet 我的钱包`
- **THEN** 系统将导入数据写入指定钱包

#### Scenario: Import without wallet (use active)
- **WHEN** 用户执行 `fund import-holding --file holdings.csv`
- **AND** 没有指定 --wallet 参数
- **THEN** 系统使用当前活跃钱包

### Requirement: Import Report
导入完成后系统必须展示结构化报告。

#### Scenario: All imports succeeded
- **WHEN** 导入过程中所有记录都成功
- **THEN** 系统打印：`✅ 成功：3，❌ 失败：0`

#### Scenario: Partial failures
- **WHEN** 导入过程中有成功项和失败项
- **THEN** 系统打印成功数量并以表格形式展示失败项的基金名称、原因
