## MODIFIED Requirements

### Requirement: 远程搜索建议功能
当本地未命中输入时，系统必须调用远程搜索接口进行匹配。该功能 SHALL 优先使用晨星官方接口提供搜索建议。

#### Scenario: 模糊搜索返回单一结果
- **WHEN** 用户输入 "南方" 且本地未找到，且晨星接口返回唯一的匹配基金 "020988"
- **THEN** 系统自动识别该输入为 "020988"，并提示 "发现匹配基金: 南方恒生科技 (020988)"

## REMOVED Requirements

### Requirement: 同花顺搜索支持
**Reason**: 同花顺接口为 JSONP 格式且解析逻辑不稳定，已被更可靠的晨星 JSON 接口取代。
**Migration**: 系统已自动切换到 `MorningstarSearchProvider`。
