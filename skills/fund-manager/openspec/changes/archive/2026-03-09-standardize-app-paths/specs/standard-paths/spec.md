## MODIFIED Requirements

### Requirement: Standard Unix Config Path
在 macOS 和 Linux 上，系统必须优先使用 `~/.config/fund-manager` 作为配置目录。

#### Scenario: App dir on Unix
- **GIVEN** 系统为 macOS 或 Linux
- **WHEN** 调用 `get_app_dir`
- **THEN** 返回的路径应以 `.config/fund-manager` 结尾。

### Requirement: Windows Compatibility
在 Windows 上，系统必须保持使用 `%APPDATA%\fund-manager`。

#### Scenario: App dir on Windows
- **GIVEN** 系统为 Windows
- **WHEN** 调用 `get_app_dir`
- **THEN** 返回的路径应符合 Windows AppData 规范。
