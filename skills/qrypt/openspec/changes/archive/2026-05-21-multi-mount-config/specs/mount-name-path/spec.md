## ADDED Requirements

### Requirement: mount_name:path 语法

CLI 命令 SHALL 支持 `mount_name:path` 格式指定目标网盘和路径。

#### Scenario: 完整语法
- **WHEN** 用户输入 `personal:/Documents/file.txt`
- **THEN** 解析为 mount_name=`personal`，path=`/Documents/file.txt`

#### Scenario: 单 mount 模式兼容
- **WHEN** 配置只有 1 个 mount instance
- **AND** 用户输入 `/Documents/file.txt`（无前缀）
- **THEN** 系统自动匹配到唯一的 mount instance

#### Scenario: 相对路径不被误解析
- **WHEN** 用户输入 `./local/file.txt`
- **THEN** 系统当作纯路径处理（不以冒号开始）

### Requirement: 消歧规则

系统 SHALL 符合明确的消歧规则判断字符串是否为 `mount_name:path` 语法。

#### Scenario: `:/` 触发 mount 解析
- **WHEN** 字符串匹配 `^[a-z0-9-]{1,32}:/`
- **THEN** 解析为 mount_name + path

#### Scenario: Windows 盘符排除
- **WHEN** 字符串为 `C:/Users/file.txt`
- **THEN** `C` 含大写字母，不匹配 mount name 规范
- **AND** 整个字符串当作纯路径
