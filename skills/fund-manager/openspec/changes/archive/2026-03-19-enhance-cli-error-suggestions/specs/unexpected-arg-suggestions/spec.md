## ADDED Requirements

### Requirement: 未知参数智能意图识别 (Unknown Argument Intent Recognition)
当用户在子命令后输入了未定义的参数时，系统 SHALL 尝试匹配当前子命令层级的其他有效子命令或参数名称。

#### Scenario: 识别错位的子命令
- **WHEN** 用户执行 `fund wallet list use`
- **THEN** 系统 SHALL 识别出 `use` 是 `wallet` 的有效子命令，但在 `list` 下无效
- **AND** 系统 SHALL 提示：`💡 Hint: 你是不是想找：'fund-manager wallet use'？`

### Requirement: 解析层级 Did you mean? 建议
对于无法识别的参数，如果其 Levenshtein 距离与现有有效子命令/参数非常接近（距离 <= 2），系统 SHALL 提供拼写纠正建议。

#### Scenario: 提示拼写相近的子命令
- **WHEN** 用户输入 `fund walllet list` (多了一个 l)
- **THEN** 系统 SHALL 识别出 `walllet` 与 `wallet` 接近
- **AND** 系统 SHALL 提示：`❓ 未识别的子命令 'walllet'。你是不是想找：'wallet'？`

### Requirement: 解析错误统一格式化
所有来自 clap 的解析阶段错误，SHALL 按照项目统一的友好格式进行重写输出。

#### Scenario: 格式化必填参数缺失错误
- **WHEN** 用户执行 `fund buy` 且未提供任何参数
- **THEN** 系统 SHALL 输出：`❌ 缺少基金标识符参数`
- **AND** 系统 SHALL 提供具体的用法示例：`   用法示例：fund buy 000300 --money 5000`
