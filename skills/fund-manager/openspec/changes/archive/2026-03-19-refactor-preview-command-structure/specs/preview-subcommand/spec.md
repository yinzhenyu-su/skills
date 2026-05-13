## ADDED Requirements

### Requirement: preview 命令支持 buy 和 sell 子命令

`preview` 命令 SHALL 作为父命令，接受 `buy` 和 `sell` 两个子命令，分别对应买入预览和卖出预览。`preview-sell` 顶层命令 SHALL 被移除。

#### Scenario: preview buy 调用买入预览

- **WHEN** 用户运行 `fund-manager preview buy <fund> --money <amount>`
- **THEN** 系统执行买入预览逻辑，展示预期费用和份额，不写入数据库

#### Scenario: preview sell 调用卖出预览

- **WHEN** 用户运行 `fund-manager preview sell <fund> --shares <amount>`
- **THEN** 系统执行卖出预览逻辑，展示预期收益和手续费，不写入数据库

#### Scenario: preview --help 显示子命令列表

- **WHEN** 用户运行 `fund-manager preview --help`
- **THEN** 帮助文本展示 `buy` 和 `sell` 两个子命令

#### Scenario: preview-sell 命令不再可用

- **WHEN** 用户运行 `fund-manager preview-sell <fund>`
- **THEN** clap 返回 "unrecognized subcommand" 错误

### Requirement: 帮助文本示例与实际调用方式一致

帮助文本中的示例 SHALL 使用正确的命令形式，二进制名为 `fund-manager`，不包含无效的 `buy` 位置参数。

#### Scenario: preview buy --help 展示正确示例

- **WHEN** 用户运行 `fund-manager preview buy --help`
- **THEN** 帮助文本中的示例格式为 `fund-manager preview buy 000312 --money 5000`

#### Scenario: preview sell --help 展示正确示例

- **WHEN** 用户运行 `fund-manager preview sell --help`
- **THEN** 帮助文本中的示例格式为 `fund-manager preview sell 000312 --shares 500`

### Requirement: 缺少 `<FUND>` 参数时提供友好引导

省略 `<FUND>` 参数时，系统 SHALL 打印当前钱包中已追踪的基金列表（含名称和代码）作为提示，并以非零状态码退出，而非抛出 clap 原始错误。

#### Scenario: preview buy 省略 fund 且钱包有基金

- **WHEN** 用户运行 `fund-manager preview buy --money 5000`（未提供基金参数）
- **THEN** 系统打印 `❌ 缺少参数 <FUND>` 错误，列出当前追踪的基金（每行格式 `基金名 (代码)`），并附上用法示例 `fund-manager preview buy <first_code> --money 5000`，最后以退出码 1 退出

#### Scenario: preview sell 省略 fund 且钱包有基金

- **WHEN** 用户运行 `fund-manager preview sell` （未提供基金参数）
- **THEN** 系统输出同样的引导信息，用法示例改为 `fund-manager preview sell <first_code> --money 5000`

#### Scenario: 省略 fund 且钱包中无基金

- **WHEN** 用户运行 `fund-manager preview buy`，且当前钱包未追踪任何基金
- **THEN** 系统打印 `❌ 缺少参数 <FUND>` 以及提示"请先用 \`fund fund add\` 添加基金"，以退出码 1 退出

### Requirement: 缺少 `--money` 和 `--shares` 时提供分类说明

`preview buy` 和 `preview sell` 在两个参数均缺失时，SHALL 输出多行说明，列举所有可用选项和具体示例，而非仅输出单行错误。

#### Scenario: preview buy 缺少 --money 和 --shares

- **WHEN** 用户运行 `fund-manager preview buy 000312`（未提供 --money 或 --shares）
- **THEN** 系统打印：
 	- `❌ 缺少参数：请提供 --money 或 --shares 之一`
 	- `--money <金额>   按投入金额买入，例如：--money 5000`
 	- `--shares <份额>  按指定份额买入，例如：--shares 4538.65`
 	- 以退出码 1 退出

#### Scenario: preview sell 缺少 --money 和 --shares

- **WHEN** 用户运行 `fund-manager preview sell 000312`（未提供 --money 或 --shares）
- **THEN** 系统打印：
 	- `❌ 缺少参数：请提供 --money 或 --shares 之一`
 	- `--shares <份额>  按指定份额卖出，例如：--shares 500`
 	- `--shares all     全部卖出`
 	- `--shares 1/2     卖出一半份额`
 	- `--money <金额>   按预期收回金额卖出，例如：--money 5000`
 	- 以退出码 1 退出

### Requirement: NAV 不可用时向后回退查找

当指定日期净值在本地数据库和远端 API 均不存在时（如 QDII 基金 T+1/T+2 延迟），系统 SHALL 优先向后查找最近已发布的净值数据，而非直接报错。

#### Scenario: 今日净值未发布（QDII 场景）

- **GIVEN** 某 QDII 基金今日净值尚未入库，但前一交易日净值存在
- **WHEN** 用户运行 `fund-manager preview buy <fund> --money 5000`
- **THEN** 系统自动使用最近一个交易日（最多向后30天）的净值完成预览计算

#### Scenario: 近期无任何净值数据

- **GIVEN** 某基金在请求日期前30天和后20天均无净值记录
- **WHEN** 用户运行 `fund-manager preview buy <fund> --money 5000`
- **THEN** 系统报告净值不可用错误
