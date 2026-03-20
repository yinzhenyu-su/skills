---
name: trending
description: 获取当日中文平台热搜榜。支持微博、知乎、头条、百度、抖音等平台。使用场景：(1) 用户询问"微博热搜"、"微博热门"；(2) 用户询问"知乎热榜"、"知乎热搜"；(3) 用户询问"头条热榜"、"头条热搜"；(4) 用户询问"百度热搜"；(5) 用户询问"有什么热搜"、"全平台热搜"。
metadata:
  openclaw:
    requires:
      bins: ["curl"]
---

# 全平台热搜榜

获取当日中文平台热搜榜单，了解当前热门话题。

数据来源：[龙轩导航 API](http://api.ilxdh.com) 聚合了多个平台的热搜数据，无需登录即可查看。

## API 请求

```bash
curl -s 'http://api.ilxdh.com/navig/topnews/list' \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -H 'User-Agent: Mozilla/5.0' \
  --data-raw '{"all":1}'
```

## API 响应结构

```json
{
  "error_code": 0,
  "msg": "",
  "data": [
    {
      "id": 123,
      "type": 1,
      "top_name": "微博热搜",
      "top_content": [
        {
          "rank": 1,
          "hot": 12345678,
          "word": "话题标题",
          "desc": "话题描述",
          "url": "https://...",
          "image": ""
        }
      ]
    }
  ]
}
```

注意：`hot` 字段在微博、知乎等平台可能为 `null`；每个平台返回约 30-52 条数据。

## 支持平台

`{"all":1}` 请求会返回全部 8 个平台（**不支持在请求体中按 type 过滤**，需在响应数据里按 type 字段筛选）：

| 平台 | type值 | 关键词 |
|------|--------|--------|
| 微博热搜 | 1 | 微博、微博热搜 |
| 知乎热榜 | 2 | 知乎、知乎热榜、知乎热搜 |
| 头条热榜 | 3 | 头条、头条热榜、头条热搜 |
| 抖音热榜 | 4 | 抖音、抖音热榜 |
| 百度热搜 | 5 | 百度、百度热搜 |
| 实时销量榜 | 6 | 销量榜、热销 |
| 京东热搜商品 | 7 | 京东热搜 |
| 副业赚钱 | 8 | — |

## 使用示例

**获取微博热搜：**

```
用户: 微博热搜
助手: 调用 API，从 data 数组中找到 type=1 的条目，取其 top_content 展示

📈 微博热搜榜
1. [7年前买的泡泡玛特盲盒才发货](https://s.weibo.com/weibo?q=7年前买的泡泡玛特盲盒才发货&Refer=top)
2. ...
```

**获取知乎热榜：**

```
用户: 知乎热搜
助手: 调用 API，从 data 数组中找到 type=2 的条目

📚 知乎热榜
1. [如何看待...](https://www.zhihu.com/question/...)
2. ...
```

**获取头条热搜：**

```
用户: 头条热搜
助手: 调用 API，从 data 数组中找到 type=3 的条目

📱 头条热榜
1. [春分巧遇"龙抬头"本世纪仅有三次](https://www.toutiao.com/trending/...)
2. ...
```

**获取全平台热搜：**

```
用户: 全平台热搜 / 有什么热搜
助手: 调用 API，展示所有平台（type 1-5 的主流平台）的 top_content
```

## 输出格式

- 使用 `top_content[].word` 作为标题，`top_content[].url` 作为链接
- 所有平台的链接直接使用 API 返回的 `url` 字段，无需手动拼接或 URL 编码
- 微博 URL 格式示例：`https://s.weibo.com/weibo?q=关键词&Refer=top`
- 百度 URL 格式示例：`https://m.baidu.com/s?word=URL编码关键词`

## 注意事项

- API 返回数据可能有数分钟延迟
- 建议缓存结果避免频繁请求
