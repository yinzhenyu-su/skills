# API接口

## 获取基金基本信息(json)

请求：
GET `https://fundmobapi.eastmoney.com/FundMNewApi/FundMNDetailInformation?FCODE=020988&deviceid=123456&plat=Android&product=EFund&version=6.5.5`
响应：

```json
{
    "Datas": {
        "FCODE": "020988",
        "FEATURE": "055,080,311,318,319",
        "CYCLE": "--",
        "WEBBACKCODE": "--",
        "SHORTNAME": "南方恒生科技ETF发起联接(QDII)A",
        "FULLNAME": "南方恒生科技交易型开放式指数证券投资基金发起式联接基金(QDII)",
        "FTYPE": "指数型-海外股票",
        "ESTABDATE": "2024-05-21",
        "ENDNAV": "602991325.45",
        "FEGMRQ": "2025-12-31",
        "RLEVEL_SZ": "--",
        "RISKLEVEL": "4",
        "JJGS": "南方基金",
        "TGYH": "交通银行",
        "JJGSID": "80000220",
        "JJJL": "张其思",
        "NETNAV": "16156114.62",
        "BENCH": "经汇率调整后的恒生科技指数收益率*95%+银行人民币活期存款利率(税后)*5%",
        "INDEXCODE": "HSTECH",
        "INDEXNAME": "恒生科技指数",
        "PRSVPERIOD": "--",
        "PRSVDATE": "--",
        "PRSVTYPE": "--",
        "BUYTIME": "--",
        "MGREXP": "0.15%",
        "TRUSTEXP": "0.05%",
        "SALESEXP": "0.00%",
        "PERFCMP": "经汇率调整后的恒生科技指数收益率*95%+银行人民币活期存款利率(税后)*5%",
        "INVTGT": "本基金通过投资于目标ETF,紧密跟踪标的指数,追求与业绩比较基准相似的回报。",
        "INVSTRA": "本基金为完全被动式指数基金,主要投资于目标ETF。本基金并不参与目标ETF的管理。在正常市场情况下,本基金力争净值增长率与业绩比较基准之间的日均跟踪偏离度不超过0.35%,年跟踪误差不超过4%。\n资产配置策略\r\n为实现紧密跟踪标的指数的投资目标,本基金将以不低于基金资产净值90%的资产投资于目标ETF。为更好地实现投资目标,本基金可参与其他境外市场投资工具和境内市场投资工具投资。\n目标ETF投资策略\r\n本基金投资目标ETF的方式如下:\r\n1、申购和赎回:目标ETF开放申购赎回后,以申购/赎回对价进行申购赎回或者按照目标ETF法律文件的约定以其他方式申赎目标ETF。\r\n2、二级市场方式:目标ETF上市交易后,在二级市场进行目标ETF基金份额的交易。\r\n当目标ETF申购、赎回或交易模式进行了变更或调整,本基金也将作相应的变更或调整,无须召开基金份额持有人大会。\r\n成份股、备选成份股投资策略\r\n本基金对可投资于成份股、备选成份股的资金头寸,主要采取完全复制法,即按照标的指数的成份股组成及其权重构建基金股票投资组合,并根据标的指数成份股及其权重的变动而进行相应调整。但在因特殊情况(如流动性不足等)导致无法获得足够数量的股票时,基金管理人将搭配使用其他合理方法进行适当的替代。\r\n境外金融衍生品投资策略\r\n为更好地实现本基金的投资目标,本基金将在条件允许的情况下投资于远期合约、互换及经中国证监会认可的境外交易所上市交易的权证、期权、期货等金融衍生产品,以期降低跟踪误差水平。本基金将根据风险管理的原则,以套期保值为目的,主要选择流动性好、交易活跃的金融衍生品进行交易。\r\n为了更好地跟踪标的指数,本基金还可通过投资外汇期货、外汇远期、外汇互换等来管理基金投资以及应对申购赎回过程中的汇率风险。\r\n境内金融衍生品投资策略\r\n为更好地实现投资目标,本基金可投资股指期货、股票期权和其他经中国证监会允许的境内衍生金融产品,如与标的指数或标的指数成份股、备选成份股相关的金融衍生品。本基金将根据风险管理的原则,主要选择流动性好、交易活跃的衍生品合约进行交易。\r\n存托凭证投资策略\r\n本基金在综合考虑预期收益、风险、流动性等因素的基础上,根据审慎原则合理参与存托凭证的投资,以更好地跟踪标的指数,追求与业绩比较基准相似的回报。\r\n境外市场基金投资策略\r\n为实现流动性管理和更好地追踪标的指数等目标,本基金可参与投资与本基金具有相似投资目标、投资策略或追踪与本基金标的指数相同的公募基金(含ETF),本基金还可参与其他境外市场基金投资。境外回购交易及证券借贷交易策略\r\n为更好地实现投资目标,增加基金收益,弥补基金运作成本,本基金在控制风险的前提下还将参与境外回购交易、境外证券借贷交易等投资。\r\n境外回购交易及证券借贷交易策略\r\n为更好地实现投资目标,增加基金收益,弥补基金运作成本,本基金在控制风险的前提下还将参与境外回购交易、境外证券借贷交易等投资。\r\n境内融资及转融通证券出借业务投资策略\r\n为更好地实现投资目标,在加强风险防范并遵守审慎原则的前提下,本基金可在法律法规允许范围内根据投资管理的需要参与境内融资及转融通证券出借业务。本基金将在分析市场情况、投资者类型与结构、基金历史申赎情况、出借证券流动性情况等因素的基础上,合理确定出借证券的范围、期限和比例。\r\n今后,随着证券市场的发展、金融工具的丰富和交易方式的创新等,基金还将积极寻求其他投资机会,如法律法规或监管机构以后允许基金投资其他品种,本基金将在履行适当程序后,将其纳入投资范围以丰富组合投资策略。\n资产支持证券投资策略\r\n资产支持证券投资关键在于对基础资产质量及未来现金流的分析,本基金将采用基本面分析和数量化模型相结合,对个券进行风险分析和价值评估后进行投资。本基金将严格控制资产支持证券的总体投资规模并进行分散投资,以降低流动性风险。\n股指期货投资策略\r\n本基金投资股指期货,将根据风险管理的原则,以套期保值为目的,主要选择流动性好、交易活跃的股指期货合约。本基金力争利用股指期货的杠杆作用,降低股票仓位频繁调整的交易成本和跟踪误差,达到有效跟踪标的指数的目的。\n股票期权投资策略\r\n本基金投资股票期权,将根据风险管理的原则,以套期保值为目的,充分考虑股票期权的流动性和风险收益特征,在风险可控的前提下,采取备兑开仓、delta中性等策略适度参与股票期权投资。\n可转换债券及可交换债券投资策略\r\n本基金在可转换债券和可交换债券投资中预期运用仓位策略、类属策略、个券挖掘策略、条款价值策略、行权套利策略等多种策略,构建风险收益比佳、基本面稳健的组合。其中仓位策略和类属策略,将根据基本面环境、股债性价比,确定整体仓位水平、偏股型和偏债型可转债占比。个券挖掘策略,从基本面视角来挖掘可转债的机会,选择基本面景气向上、估值不高的偏股型转债作为获取超额收益的品种。条款价值策略,即深入分析修正转股价条款、回售条款和赎回条款等对转债价值的影响,挖掘各条款对应的投资机会。行权套利策略,可转债具备按照约定的价格转换为标的股票的权利,本基金将综合分析正股的基本面和估值水平、可转债的转股溢价率水平、可转债和标的股票的流动性、可转债转股对标的股票稀释和抛售压力等因素,确定是否行使将可转债转换为标的股票的权利,以及转股的时机和转股后标的股票的持有时间。\n债券投资策略\r\n本基金将结合对未来市场利率预期运用久期调整策略、收益率曲线配置策略、债券类属配置策略、利差轮动策略等多种积极管理策略,通过严谨的研究发现价值被低估的债券和市场投资机会,构建收益稳定、流动性良好的债券组合。"
    },
    "ErrCode": 0,
    "Success": true,
    "ErrMsg": null,
    "Message": null,
    "ErrorCode": "0",
    "ErrorMessage": null,
    "ErrorMsgLst": null,
    "TotalCount": 1,
    "Expansion": null
}
```

## 获取基金持仓前十的股票图片

GET `https://j6.dfcfw.com/charts/StockPos/016453.png?rt=NaN`
响应：图片或 404 错误

## 获取大盘指数数据(jsonp)

请求：
GET `https://push2.eastmoney.com/api/qt/ulist.np/get?fltt=2&secids=1.000001,0.399001&invt=2&fields=f2,f3,f4,f6,f12,f14,f104,f105,f106&ut=267f9ad526dbe6b0262ab19316f5a25b&cb=jQuery183038316905989944383_1773111142939&_=1773111142943`

secids 对应关系：1.000001 上证指数，0.399001 深证成指，100.DJIA 道琼斯指数，100.NDX 纳斯达克指数，100.SPX 标普500指数

响应：

```javascript
jQuery183038316905989944383_1773111142939({
    "rc": 0,
    "rt": 11,
    "svr": 175643426,
    "lt": 1,
    "full": 1,
    "dlmkts": "",
    "data": {
        "total": 2,
        "diff": [{
            "f2": 4105.8, //指数点位
            "f3": 0.22, //指数涨跌幅
            "f4": 9.2, //指数涨跌点数
            "f6": 596841992448.6,//指数成交金额
            "f12": "000001", //指数代码
            "f14": "上证指数", //指数名称
            "f104": 1758, //涨家数
            "f105": 527, //跌家数
            "f106": 58 // 平家数
        }, {
            "f2": 14273.43,
            "f3": 1.46,
            "f4": 205.93,
            "f6": 793263247583.6062,
            "f12": "399001",
            "f14": "深证成指",
            "f104": 2237,
            "f105": 599,
            "f106": 80
        }]
    }
});
```

## 获取基金持仓前十的股票数据(jsonp)

请求：GET `https://fundf10.eastmoney.com/FundArchivesDatas.aspx?type=jjcc&code=020989&topline=10&year=&month=&rt=0.6779877730132542`
响应：
从响应中提取出 `content` 字段的 HTML 内容中提取 document.querySelector('.box .boxitem.w790 .hide') 可以获取到基金持仓前 10 的股票代码列表如下（需要处理尾随逗号）
116.03690,116.00981,116.00700,116.01810,116.09999,116.01211,116.09988,116.09618,116.01024,116.09888,

```javascript
var apidata = {
    content: "<div class='box'><div class='boxitem w790'><h4 class='t'><label class='left'><a  title='南方恒生科技ETF发起联接(QDII)C' href='http://fund.eastmoney.com/020989.html'>南方恒生科技ETF发起联接(QDII)C</a>&nbsp;&nbsp;2025年4季度股票投资明细</label><label class='right lab2 xq505'>&nbsp;&nbsp;&nbsp;&nbsp;来源：天天基金&nbsp;&nbsp;&nbsp;&nbsp;截止至：<font class='px12'>2025-12-31</font></label></h4><div class='space0'></div><table class='w782 comm tzxq'><thead><tr><th class='first' style='width:34px'>序号</th><th>股票代码</th><th>股票名称</th><th style='width:75px'>最新价</th><th style='width:75px'>涨跌幅</th><th style='width: 110px;'>相关资讯</th><th>占净值比例</th><th class='cgs'>持股数<br />（万股）</th><th class='last ccs'>持仓市值<br />（万元人民币）</th></tr></thead><tbody><tr><td>1</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.03690' >03690</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.03690'>美团-W</a></td><td class='toc' ><span data-id='dq03690'>--</span></td><td class='toc' ><span data-id='zd03690'>--</span></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.03690' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.03690' >行情</a></td><td class='toc'>1.12%</td><td class='toc'>27.58</td><td class='toc'>2,573.29</td></tr><tr><td>2</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.00981' >00981</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.00981'>中芯国际</a></td><td class='toc' ><span data-id='dq00981'>--</span></td><td class='toc' ><span data-id='zd00981'>--</span></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.00981' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.00981' >行情</a></td><td class='toc'>1.08%</td><td class='toc'>38.35</td><td class='toc'>2,474.92</td></tr><tr><td>3</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.00700' >00700</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.00700'>腾讯控股</a></td><td class='toc' ><span data-id='dq00700'>--</span></td><td class='toc' ><span data-id='zd00700'>--</span></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.00700' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.00700' >行情</a></td><td class='toc'>1.01%</td><td class='toc'>4.31</td><td class='toc'>2,331.83</td></tr><tr><td>4</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.01810' >01810</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.01810'>小米集团-W</a></td><td class='toc' ><span data-id='dq01810'>--</span></td><td class='toc' ><span data-id='zd01810'>--</span></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.01810' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.01810' >行情</a></td><td class='toc'>1.01%</td><td class='toc'>65.40</td><td class='toc'>2,321.47</td></tr><tr><td>5</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09999' >09999</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09999'>网易-S</a></td><td class='toc' ><span data-id='dq09999'>--</span></td><td class='toc' ><span data-id='zd09999'>--</span></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09999' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09999' >行情</a></td><td class='toc'>1.00%</td><td class='toc'>11.86</td><td class='toc'>2,298.84</td></tr><tr><td>6</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.01211' >01211</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.01211'>比亚迪股份</a></td><td class='toc' ><span data-id='dq01211'>--</span></td><td class='toc' ><span data-id='zd01211'>--</span></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.01211' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.01211' >行情</a></td><td class='toc'>1.00%</td><td class='toc'>26.59</td><td class='toc'>2,289.98</td></tr><tr><td>7</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09988' >09988</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09988'>阿里巴巴-W</a></td><td class='toc' ><span data-id='dq09988'>--</span></td><td class='toc' ><span data-id='zd09988'>--</span></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09988' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09988' >行情</a></td><td class='toc'>0.95%</td><td class='toc'>16.95</td><td class='toc'>2,186.21</td></tr><tr><td>8</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09618' >09618</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09618'>京东集团-SW</a></td><td class='toc' ><span data-id='dq09618'>--</span></td><td class='toc' ><span data-id='zd09618'>--</span></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09618' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09618' >行情</a></td><td class='toc'>0.66%</td><td class='toc'>14.94</td><td class='toc'>1,505.44</td></tr><tr><td>9</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.01024' >01024</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.01024'>快手-W</a></td><td class='toc' ><span data-id='dq01024'>--</span></td><td class='toc' ><span data-id='zd01024'>--</span></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.01024' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.01024' >行情</a></td><td class='toc'>0.65%</td><td class='toc'>25.77</td><td class='toc'>1,488.50</td></tr><tr><td>10</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09888' >09888</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09888'>百度集团-SW</a></td><td class='toc' ><span data-id='dq09888'>--</span></td><td class='toc' ><span data-id='zd09888'>--</span></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09888' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09888' >行情</a></td><td class='toc'>0.54%</td><td class='toc'>10.55</td><td class='toc'>1,252.47</td></tr></tbody></table><div class='hide' id='gpdmList'>116.03690,116.00981,116.00700,116.01810,116.09999,116.01211,116.09988,116.09618,116.01024,116.09888,</div></div></div><div class='box'><div class='boxitem w790'><h4 class='t'><label class='left'><a  title='南方恒生科技ETF发起联接(QDII)C' href='http://fund.eastmoney.com/020989.html'>南方恒生科技ETF发起联接(QDII)C</a>&nbsp;&nbsp;2025年3季度股票投资明细</label><label class='right lab2 xq505'>&nbsp;&nbsp;&nbsp;&nbsp;来源：天天基金&nbsp;&nbsp;&nbsp;&nbsp;截止至：<font class='px12'>2025-09-30</font></label></h4><div class='space0'></div><table class='w782 comm tzxq'><thead><tr><th class='first'style='width:34px'>序号</th><th>股票代码</th><th>股票名称</th><th style='width: 110px;'>相关资讯</th><th>占净值比例</th><th class='cgs'>持股数<br />（万股）</th><th class='last ccs'>持仓市值<br />（万元人民币）</th></tr></thead><tbody><tr><td>1</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09988' >09988</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09988'>阿里巴巴-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09988' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09988' >行情</a></td><td class='toc'>8.62%</td><td class='toc'>111.13</td><td class='toc'>17,958.33</td></tr><tr><td>2</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.00981' >00981</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.00981'>中芯国际</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.00981' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.00981' >行情</a></td><td class='toc'>7.56%</td><td class='toc'>216.90</td><td class='toc'>15,752.92</td></tr><tr><td>3</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.00700' >00700</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.00700'>腾讯控股</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.00700' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.00700' >行情</a></td><td class='toc'>7.24%</td><td class='toc'>24.93</td><td class='toc'>15,090.27</td></tr><tr><td>4</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09999' >09999</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09999'>网易-S</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09999' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09999' >行情</a></td><td class='toc'>6.91%</td><td class='toc'>66.64</td><td class='toc'>14,407.15</td></tr><tr><td>5</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.03690' >03690</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.03690'>美团-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.03690' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.03690' >行情</a></td><td class='toc'>6.78%</td><td class='toc'>148.06</td><td class='toc'>14,125.87</td></tr><tr><td>6</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.01211' >01211</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.01211'>比亚迪股份</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.01211' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.01211' >行情</a></td><td class='toc'>6.52%</td><td class='toc'>135.10</td><td class='toc'>13,592.46</td></tr><tr><td>7</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.01810' >01810</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.01810'>小米集团-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.01810' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.01810' >行情</a></td><td class='toc'>6.34%</td><td class='toc'>268.02</td><td class='toc'>13,213.63</td></tr><tr><td>8</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.01024' >01024</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.01024'>快手-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.01024' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.01024' >行情</a></td><td class='toc'>5.36%</td><td class='toc'>144.68</td><td class='toc'>11,174.81</td></tr><tr><td>9</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09618' >09618</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09618'>京东集团-SW</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09618' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09618' >行情</a></td><td class='toc'>5.12%</td><td class='toc'>84.33</td><td class='toc'>10,663.34</td></tr><tr><td>10</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09888' >09888</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09888'>百度集团-SW</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09888' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09888' >行情</a></td><td class='toc'>3.58%</td><td class='toc'>61.39</td><td class='toc'>7,464.96</td></tr></tbody></table><div class='hide' id='gpdmList'></div></div></div><div class='box'><div class='boxitem w790'><h4 class='t'><label class='left'><a  title='南方恒生科技ETF发起联接(QDII)C' href='http://fund.eastmoney.com/020989.html'>南方恒生科技ETF发起联接(QDII)C</a>&nbsp;&nbsp;2025年2季度股票投资明细</label><label class='right lab2 xq505'>&nbsp;&nbsp;&nbsp;&nbsp;来源：天天基金&nbsp;&nbsp;&nbsp;&nbsp;截止至：<font class='px12'>2025-06-30</font></label></h4><div class='space0'></div><table class='w782 comm tzxq'><thead><tr><th class='first'style='width:34px'>序号</th><th>股票代码</th><th>股票名称</th><th style='width: 110px;'>相关资讯</th><th>占净值比例</th><th class='cgs'>持股数<br />（万股）</th><th class='last ccs'>持仓市值<br />（万元人民币）</th></tr></thead><tbody><tr><td>1</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.01810' >01810</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.01810'>小米集团-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.01810' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.01810' >行情</a></td><td class='toc'>8.23%</td><td class='toc'>102.72</td><td class='toc'>5,615.85</td></tr><tr><td>2</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09999' >09999</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09999'>网易-S</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09999' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09999' >行情</a></td><td class='toc'>7.81%</td><td class='toc'>27.70</td><td class='toc'>5,330.07</td></tr><tr><td>3</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.00700' >00700</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.00700'>腾讯控股</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.00700' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.00700' >行情</a></td><td class='toc'>7.27%</td><td class='toc'>10.82</td><td class='toc'>4,963.25</td></tr><tr><td>4</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09988' >09988</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09988'>阿里巴巴-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09988' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09988' >行情</a></td><td class='toc'>7.04%</td><td class='toc'>47.98</td><td class='toc'>4,804.34</td></tr><tr><td>5</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.03690' >03690</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.03690'>美团-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.03690' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.03690' >行情</a></td><td class='toc'>6.70%</td><td class='toc'>40.00</td><td class='toc'>4,570.69</td></tr><tr><td>6</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.01211' >01211</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.01211'>比亚迪股份</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.01211' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.01211' >行情</a></td><td class='toc'>6.69%</td><td class='toc'>40.90</td><td class='toc'>4,569.10</td></tr><tr><td>7</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09618' >09618</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09618'>京东集团-SW</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09618' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09618' >行情</a></td><td class='toc'>6.45%</td><td class='toc'>37.76</td><td class='toc'>4,403.68</td></tr><tr><td>8</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.00981' >00981</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.00981'>中芯国际</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.00981' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.00981' >行情</a></td><td class='toc'>5.80%</td><td class='toc'>97.05</td><td class='toc'>3,956.16</td></tr><tr><td>9</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.01024' >01024</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.01024'>快手-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.01024' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.01024' >行情</a></td><td class='toc'>5.51%</td><td class='toc'>65.11</td><td class='toc'>3,758.57</td></tr><tr><td>10</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.02015' >02015</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.02015'>理想汽车-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.02015' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.02015' >行情</a></td><td class='toc'>4.29%</td><td class='toc'>30.04</td><td class='toc'>2,931.26</td></tr></tbody></table><div class='hide' id='gpdmList'></div><div class='tfoot'><font class='px12'><a style='cursor:pointer;' onclick='LoadMore(this,6,LoadStockPos)'>显示全部持仓明细>></a></font></div></div></div></div><div class='box'><div class='boxitem w790'><h4 class='t'><label class='left'><a  title='南方恒生科技ETF发起联接(QDII)C' href='http://fund.eastmoney.com/020989.html'>南方恒生科技ETF发起联接(QDII)C</a>&nbsp;&nbsp;2025年1季度股票投资明细</label><label class='right lab2 xq505'>&nbsp;&nbsp;&nbsp;&nbsp;来源：天天基金&nbsp;&nbsp;&nbsp;&nbsp;截止至：<font class='px12'>2025-03-31</font></label></h4><div class='space0'></div><table class='w782 comm tzxq'><thead><tr><th class='first'style='width:34px'>序号</th><th>股票代码</th><th>股票名称</th><th style='width: 110px;'>相关资讯</th><th>占净值比例</th><th class='cgs'>持股数<br />（万股）</th><th class='last ccs'>持仓市值<br />（万元人民币）</th></tr></thead><tbody><tr><td>1</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.00700' >00700</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.00700'>腾讯控股</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.00700' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.00700' >行情</a></td><td class='toc'>7.70%</td><td class='toc'>7.22</td><td class='toc'>3,311.43</td></tr><tr><td>2</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09618' >09618</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09618'>京东集团-SW</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09618' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09618' >行情</a></td><td class='toc'>7.63%</td><td class='toc'>22.12</td><td class='toc'>3,281.67</td></tr><tr><td>3</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09988' >09988</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09988'>阿里巴巴-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09988' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09988' >行情</a></td><td class='toc'>7.61%</td><td class='toc'>27.72</td><td class='toc'>3,274.35</td></tr><tr><td>4</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.01810' >01810</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.01810'>小米集团-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.01810' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.01810' >行情</a></td><td class='toc'>7.40%</td><td class='toc'>70.10</td><td class='toc'>3,182.77</td></tr><tr><td>5</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.03690' >03690</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.03690'>美团-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.03690' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.03690' >行情</a></td><td class='toc'>7.34%</td><td class='toc'>21.98</td><td class='toc'>3,160.22</td></tr><tr><td>6</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.00981' >00981</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.00981'>中芯国际</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.00981' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.00981' >行情</a></td><td class='toc'>6.80%</td><td class='toc'>68.75</td><td class='toc'>2,924.79</td></tr><tr><td>7</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.01024' >01024</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.01024'>快手-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.01024' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.01024' >行情</a></td><td class='toc'>6.09%</td><td class='toc'>52.20</td><td class='toc'>2,618.13</td></tr><tr><td>8</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.02015' >02015</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.02015'>理想汽车-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.02015' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.02015' >行情</a></td><td class='toc'>5.09%</td><td class='toc'>23.95</td><td class='toc'>2,189.18</td></tr><tr><td>9</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09868' >09868</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09868'>小鹏汽车-W</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09868' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09868' >行情</a></td><td class='toc'>4.74%</td><td class='toc'>28.05</td><td class='toc'>2,039.77</td></tr><tr><td>10</td><td class='toc'><a href='//quote.eastmoney.com/unify/r/116.09999' >09999</a></td><td class='toc' style='line-height:18px'><a href='//quote.eastmoney.com/unify/r/116.09999'>网易-S</a></td><td class='xglj'><a href='//guba.eastmoney.com/interface/GetList.aspx?code=116.09999' >股吧</a><a href='//quote.eastmoney.com/unify/r/116.09999' >行情</a></td><td class='toc'>4.07%</td><td class='toc'>12.00</td><td class='toc'>1,749.69</td></tr></tbody></table><div class='hide' id='gpdmList'></div></div></div>",
    arryear: [2025, 2024],
    curyear: 2025
};

```

## 根据股票 id 查询最新股价，涨跌幅，动态市盈率(jsonp)

请求： GET `
https://push2.eastmoney.com/api/qt/ulist.np/get?fltt=2&invt=2&fields=f2,f3,f12,f14,f9&cb=jQuery183019830549511523854_1773128744941&ut=267f9ad526dbe6b0262ab19316f5a25b&secids=116.09926,1.600276,1.688235,116.01801,0.002755,116.06990,1.688506,1.688331,1.688192,116.01530,&_=1773128745080
`
响应：

```javascript
jQuery183019830549511523854_1773128744941({
    "rc": 0,
    "rt": 11,
    "svr": 175643880,
    "lt": 1,
    "full": 1,
    "dlmkts": "",
    "data": {
        "total": 10,
        "diff": [{
            "f2": 115.1,
            "f3": 10.99,
            "f9": -115.33,
            "f12": "09926",
            "f14": "康方生物"
        }, {
            "f2": 56.55,
            "f3": 2.93,
            "f9": 48.95,
            "f12": "600276",
            "f14": "恒瑞医药"
        }, {
            "f2": 245.2,
            "f3": 5.29,
            "f9": 265.63,
            "f12": "688235",
            "f14": "百济神州-U"
        }, {
            "f2": 85.7,
            "f3": 4.83,
            "f9": 119.65,
            "f12": "01801",
            "f14": "信达生物"
        }, {
            "f2": 15.0,
            "f3": 3.31,
            "f9": 46.76,
            "f12": "002755",
            "f14": "奥赛康"
        }, {
            "f2": 414.0,
            "f3": 8.95,
            "f9": -122.64,
            "f12": "06990",
            "f14": "科伦博泰生物-B"
        }, {
            "f2": 281.0,
            "f3": 10.17,
            "f9": -110.42,
            "f12": "688506",
            "f14": "百利天恒"
        }, {
            "f2": 118.35,
            "f3": 10.27,
            "f9": 94.29,
            "f12": "688331",
            "f14": "荣昌生物"
        }, {
            "f2": 52.58,
            "f3": 5.37,
            "f9": -32.01,
            "f12": "688192",
            "f14": "迪哲医药-U"
        }, {
            "f2": 21.82,
            "f3": 6.75,
            "f9": 21.7,
            "f12": "01530",
            "f14": "三生制药"
        }]
    }
});
```

## 根据输入文本获取匹配基金

请求：GET `https://news.10jqka.com.cn/public/index_keyboard.php?type=fund&search-text=015&jsoncallback=jQuery1830027070694497917103_1773124646433`
响应：

注意：部分基金可能搜索不到，比如160119

```bash
curl 'https://news.10jqka.com.cn/public/index_keyboard.php?type=fund&search-text=015&jsoncallback=jQuery1830027070694497917103_1773124646433' \
  -H 'Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7' \
  -H 'Accept-Language: zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7' \
  -H 'Cache-Control: max-age=0' \
  -H 'Connection: keep-alive' \
  -H 'DNT: 1' \
  -H 'Sec-Fetch-Dest: document' \
  -H 'Sec-Fetch-Mode: navigate' \
  -H 'Sec-Fetch-Site: none' \
  -H 'Sec-Fetch-User: ?1' \
  -H 'Upgrade-Insecure-Requests: 1' \
  -H 'User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36' \
  -H 'sec-ch-ua: "Chromium";v="146", "Not-A.Brand";v="24", "Google Chrome";v="146"' \
  -H 'sec-ch-ua-mobile: ?0' \
  -H 'sec-ch-ua-platform: "macOS"'
```

```javascript
jQuery1830027070694497917103_1773124646433(["0||015000 \u534e\u6cf0\u4fdd\u5174\u5409\u5e74\u76c8\u6df7\u5408C \u57fa\u91d1", "0||015001 \u5de5\u94f6\u7269\u6d41\u4ea7\u4e1a\u80a1\u7968C \u57fa\u91d1", "0||015002 \u5de5\u94f6\u751f\u6001\u73af\u5883\u80a1\u7968C \u57fa\u91d1", "0||015003 \u4e2d\u90ae\u5c0a\u4f51\u4e00\u5e74\u5b9a\u5f00\u503a\u5238 \u57fa\u91d1", "0||015004 \u4e2d\u90ae\u80fd\u6e90\u9769\u65b0\u6df7\u5408\u53d1\u8d77\u5f0fA \u57fa\u91d1"]);
```

## 获取基金累计收益率走势图数据(jsonp)

请求：GET `https://api.fund.eastmoney.com/f10/FundLJSYLZS/?bzdm=000001&dt=month&callback=jQuery18302776145446923678_1773131650567&_=1773132154305`

dt参数说明：month 本月 threemonth 三个月 sixmonth 六个月 year 一年 threeyear 三年 fiveyear 五年 今年来 thisyear 去年 成立以来 all
Data数据格式: 日期_累计收益率_沪深300累计收益率_上证指数累计收益率|

响应：

```javascript
jQuery18302776145446923678_1773131650567({
    "Data": "2026/02/09_0.00_0.00_0.00|2026/02/10_0.17_0.11_0.13|2026/02/11_-0.87_-0.11_0.22|2026/02/12_0.26_0.01_0.27|2026/02/13_-0.35_-1.24_-0.99|2026/02/24_-0.35_-0.24_-0.14|2026/02/25_0.78_0.36_0.59|2026/02/26_1.39_0.17_0.57|2026/02/27_0.35_-0.18_0.97|2026/03/02_0.00_0.20_1.44|2026/03/03_-4.36_-1.34_-0.01|2026/03/04_-4.80_-2.47_-0.99|2026/03/05_-3.23_-1.51_-0.35|2026/03/06_-2.96_-1.24_0.03|2026/03/09_-4.53_-2.20_-0.64",
    "ErrCode": 0,
    "ErrMsg": null,
    "TotalCount": 0,
    "Expansion": null,
    "PageSize": 0,
    "PageIndex": 0
})
```

## 获取基金净值

接口：<https://api.fund.eastmoney.com/f10/lsjz?callback=jQuery18306819045531849094_1773656476226&fundCode=160119&pageIndex=1&pageSize=20&startDate=&endDate=&_=1773656638251>

根据开始和结束日期获取基金净值数据，日期格式为：2020-01-01，不传则以当前日期作为截止日期

返回值：

```javascript
jQuery18306819045531849094_1773656476226({
    "Data": {
        "LSJZList": [
            {
                "FSRQ": "2026-03-13",
                "DWJZ": "2.3026",
                "LJJZ": "2.4026",
                "JZZZL": "-1.35",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-03-12",
                "DWJZ": "2.3342",
                "LJJZ": "2.4342",
                "JZZZL": "-0.52",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-03-11",
                "DWJZ": "2.3465",
                "LJJZ": "2.4465",
                "JZZZL": "-0.08",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-03-10",
                "DWJZ": "2.3483",
                "LJJZ": "2.4483",
                "JZZZL": "1.52",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-03-09",
                "DWJZ": "2.3131",
                "LJJZ": "2.4131",
                "JZZZL": "-0.92",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-03-06",
                "DWJZ": "2.3345",
                "LJJZ": "2.4345",
                "JZZZL": "0.60",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-03-05",
                "DWJZ": "2.3205",
                "LJJZ": "2.4205",
                "JZZZL": "0.66",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-03-04",
                "DWJZ": "2.3052",
                "LJJZ": "2.4052",
                "JZZZL": "-0.41",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-03-03",
                "DWJZ": "2.3147",
                "LJJZ": "2.4147",
                "JZZZL": "-4.16",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-03-02",
                "DWJZ": "2.4151",
                "LJJZ": "2.5151",
                "JZZZL": "-0.02",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-02-27",
                "DWJZ": "2.4155",
                "LJJZ": "2.5155",
                "JZZZL": "1.12",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-02-26",
                "DWJZ": "2.3888",
                "LJJZ": "2.4888",
                "JZZZL": "0.33",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-02-25",
                "DWJZ": "2.3809",
                "LJJZ": "2.4809",
                "JZZZL": "1.52",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-02-24",
                "DWJZ": "2.3452",
                "LJJZ": "2.4452",
                "JZZZL": "1.06",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-02-13",
                "DWJZ": "2.3207",
                "LJJZ": "2.4207",
                "JZZZL": "-1.41",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-02-12",
                "DWJZ": "2.3539",
                "LJJZ": "2.4539",
                "JZZZL": "1.13",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-02-11",
                "DWJZ": "2.3277",
                "LJJZ": "2.4277",
                "JZZZL": "0.24",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-02-10",
                "DWJZ": "2.3221",
                "LJJZ": "2.4221",
                "JZZZL": "-0.04",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-02-09",
                "DWJZ": "2.3231",
                "LJJZ": "2.4231",
                "JZZZL": "1.92",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            },
            {
                "FSRQ": "2026-02-06",
                "DWJZ": "2.2794",
                "LJJZ": "2.3794",
                "JZZZL": "0.00",
                "SGZT": "开放申购",
                "SHZT": "开放赎回",
                "FHFCZ": "",
                "FHFCBZ": "",
                "NAVTYPE": "1",
                "SDATE": null,
                "ACTUALSYI": "",
                "DTYPE": null,
                "FHSP": ""
            }
        ],
        "FundType": "001",
        "SYType": null,
        "isNewType": false,
        "Feature": "020,030,031,050,051,053"
    },
    "ErrCode": 0,
    "ErrMsg": null,
    "TotalCount": 3989,
    "Expansion": null,
    "PageSize": 20,
    "PageIndex": 1
})
```

## 获取基金分段费率

请求接口 GET <https://www.morningstar.cn/cn-api/v2/funds/000300/fees>
