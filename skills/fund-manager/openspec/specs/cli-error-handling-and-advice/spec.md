## ADDED Requirements

### Requirement: 智能参数反转检测 (Smart Param Swap Detection)
在处理 `buy`、`sell`、`import` 等带有“基金标识符”和“数值（金额/份额）”参数的命令时，如果解析基金失败，系统 SHALL 自动检查后续数值参数是否更像是一个有效的 6 位基金代码。

#### Scenario: 检测到 Buy 命令参数反转
- **WHEN** 用户执行 `fund buy 1000 --money 520570`，且本地/远程未找到名为 "1000" 的基金
- **THEN** 系统 SHALL 在错误信息中包含：`💡 Hint: 你是不是把[基金代码]和[金额]写反了？`

### Requirement: 基金名称拼写建议 (Did you mean?)
当用户输入的基金名称/代码在本地和远程均未找到精确匹配时，系统 SHALL 搜索本地已持有的基金列表，并返回 Levenshtein 距离最近的前 1-2 个建议。

#### Scenario: 提供拼写建议
- **WHEN** 用户输入 `fund status 沪深30`，且库中只有 "沪深300"
- **THEN** 系统 SHALL 提示：`❓ 未找到基金 '沪深30'。你是不是想找：'沪深300' (000300)？`

### Requirement: 上下文感知错误增强
所有涉及钱包操作的错误信息，SHALL 包含当前活跃钱包的名称，以帮助用户确认操作环境。

#### Scenario: 报错包含钱包上下文
- **WHEN** 在钱包 "MyFund" 下执行操作失败
- **THEN** 错误输出的末尾 SHALL 包含一行：`当前激活钱包: [MyFund]`

### Requirement: 解析层错误智能分流 (Parsing Error Smart Dispatch)
系统 SHALL 拦截 clap 解析阶段的所有原始错误，并根据错误类型（ErrorKind）分流到相应的智能建议引擎。

#### Scenario: 成功拦截 UnknownArgument 错误
- **WHEN** 用户执行了带有未知参数的命令
- **THEN** 系统 SHALL 自动调用意图识别逻辑
- **AND** 系统 SHALL 输出包含纠正建议的友好中文报错
