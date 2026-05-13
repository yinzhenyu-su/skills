## ADDED Requirements

### Requirement: API 频率限制自动退避
当 API 返回 429 错误时，系统必须暂停请求并按指数退避算法等待。

#### Scenario: 触发限流
- **WHEN** 夸克 API 返回 429 响应
- **THEN** 系统等待 2s, 4s, 8s 后重试，直到成功或达到最大次数

### Requirement: 无缝 Cookie 更新
系统必须能根据 HTTP 响应头中的 `Set-Cookie` 实时更新内存中的 Cookie 状态。

#### Scenario: 保持登录
- **WHEN** 夸克在响应中提供新的 `__puus`
- **THEN** 后续所有请求自动携带该新 Cookie
