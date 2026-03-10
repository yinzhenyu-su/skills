## ADDED Requirements

### Requirement: JS Endpoint Request
系统 SHALL 向 `https://fundgz.1234567.com.cn/js/{code}.js` 发起异步 HTTP GET 请求。

#### Scenario: Successful JS request
- **WHEN** 给定基金代码 "160119"
- **THEN** 系统发起请求并获得 HTTP 200 响应。

### Requirement: JS Content Extraction
系统 SHALL 使用正则表达式 `jsonpgz\((.*)\);` 从响应文本中提取 JSON 字符串并进行反序列化。

#### Scenario: Parsing valid JS response
- **WHEN** 响应文本为 `jsonpgz({"fundcode":"160119","name":"南方中证500ETF联接A","dwjz":"1.3705"});`
- **THEN** 系统成功解析出基金名称为 "南方中证500ETF联接A"，单位净值为 1.3705。
