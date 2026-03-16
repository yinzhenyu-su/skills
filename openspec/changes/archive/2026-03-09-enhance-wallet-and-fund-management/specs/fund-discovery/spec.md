## ADDED Requirements

### Requirement: On-demand Fund Creation
系统必须在执行买入操作时，自动处理不存在的基金代码。

#### Scenario: Buying non-existent fund by code
- **WHEN** 用户执行 `fund buy 000001` 且数据库中没有该基金
- **THEN** 系统发起抓取请求：
    - 如果成功获取到名称和费率，自动创建基金记录并继续交易。
    - 如果抓取失败，提示用户检查代码或手动添加。

#### Scenario: Buying non-existent fund by name
- **WHEN** 用户输入一个本地不存在的名称（非 6 位数字代码）
- **THEN** 系统应报错提示“基金不存在”，且不尝试自动创建。
