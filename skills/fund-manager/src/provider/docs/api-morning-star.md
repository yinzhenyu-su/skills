# 获取晨星基金数据的接口

## 获取晨星基金的 performance 数据

请求参数：

```bash
curl '<https://www.morningstar.cn/cn-api/v2/funds/000513/performance>' \
  -H 'accept: application/json, text/plain, */*' \
  -H 'accept-language: zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7' \
  -H 'referer: <https://www.morningstar.cn/>' \
  -H 'user-agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36'
```

响应（JSON）：

```json
{
    "_meta": {
        "response_status": "200011",
        "response_hint": "数据查询成功"
    },
    "data": {
        "csdcc": "000513",
        "secId": "F00000TGW4",
        "categoryId": "CHCA000051",
        "categoryName": "大盘成长股票",
        "benchmarkId": "F00001LXGI",
        "benchmarkName": "沪深300相对全收益成长指数",
        "dayEnd": {
            "returns": {
                "returnDate": "2026-03-16",
                "D1": -0.83893,
                "YTD": 10.44149,
                "W1": -0.16892,
                "M1": 2.62644,
                "M3": 14.39632,
                "M6": 17.55346,
                "Y1": 54.81336,
                "Y3": 53.85617,
                "Y5": 38.0035,
                "Y7": 202.68886,
                "Y10": 282.21504,
                "sinceInception": 372.8
            },
            "categoryReturns": {
                "returnDate": "2026-03-16",
                "D1": 0.30098,
                "YTD": 4.43166,
                "W1": 1.69869,
                "M1": 0.19031,
                "M3": 9.3901,
                "M6": 8.69917,
                "Y1": 35.85453,
                "Y3": 31.23522,
                "Y5": 18.75509,
                "Y7": 113.38678,
                "Y10": 146.39056,
                "sinceInception": 244.34587
            },
            "benchmarkReturns": {
                "returnDate": "2026-03-16",
                "D1": 0.0,
                "YTD": 0.46588,
                "W1": 1.16591,
                "M1": -1.60912,
                "M3": 4.71512,
                "M6": 3.38372,
                "Y1": 27.8707,
                "Y3": 19.69418,
                "Y5": -8.80776,
                "Y7": 46.88267,
                "Y10": 71.75173,
                "sinceInception": 132.48634
            },
            "returnRanks": {
                "returnDate": "2026-03-16",
                "D1": 88,
                "YTD": 8,
                "W1": 82,
                "M1": 17,
                "M3": 12,
                "M6": 10,
                "Y1": 18,
                "Y3": 19,
                "Y5": 21,
                "Y7": 14,
                "Y10": 11,
                "sinceInception": 12
            },
            "investmentsInCategory": {
                "returnDate": "2026-03-16",
                "D1": 743,
                "YTD": 737,
                "W1": 743,
                "M1": 743,
                "M3": 729,
                "M6": 712,
                "Y1": 637,
                "Y3": 452,
                "Y5": 283,
                "Y7": 175,
                "Y10": 96,
                "sinceInception": 42
            }
        },
        "monthEnd": {
            "returns": {
                "returnDate": "2026-02-28",
                "M1": 7.30103,
                "M3": 22.19725,
                "M6": 26.49263,
                "YTD": 14.31908,
                "Y1": 61.94573,
                "Y2": 35.26022,
                "Y3": 15.01054,
                "Y5": 5.78727,
                "Y10": 15.2377
            },
            "categoryReturns": {
                "returnDate": "2026-02-28",
                "M1": 0.94199,
                "M3": 11.28909,
                "M6": 14.57626,
                "YTD": 6.24485,
                "Y1": 42.30163,
                "Y2": 27.54205,
                "Y3": 8.41649,
                "Y5": 2.48298,
                "Y10": 10.08288
            },
            "benchmarkReturns": {
                "returnDate": "2026-02-28",
                "M1": -0.83327,
                "M3": 6.85127,
                "M6": 9.34934,
                "YTD": 3.08015,
                "Y1": 35.358,
                "Y2": 23.03919,
                "Y3": 5.25659,
                "Y5": -3.01598,
                "Y10": 6.56464
            },
            "returnRanks": {
                "returnDate": "2026-02-28",
                "M1": 3,
                "M3": 9,
                "M6": 11,
                "YTD": 10,
                "Y1": 12,
                "Y2": 32,
                "Y3": 16,
                "Y5": 17,
                "Y10": 9
            },
            "investmentsInCategory": {
                "returnDate": "2026-02-28",
                "M1": 743,
                "M3": 731,
                "M6": 700,
                "YTD": 740,
                "Y1": 629,
                "Y2": 521,
                "Y3": 448,
                "Y5": 282,
                "Y10": 93
            },
            "tips": [
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null
            ]
        },
        "quarterly": {
            "returns": [
                {
                    "k": "2023-03-31",
                    "v": 9.46827
                },
                {
                    "k": "2023-06-30",
                    "v": 2.47571
                },
                {
                    "k": "2023-09-30",
                    "v": -5.07645
                },
                {
                    "k": "2023-12-31",
                    "v": -6.70103
                },
                {
                    "k": "2024-03-31",
                    "v": -5.87017
                },
                {
                    "k": "2024-06-30",
                    "v": 0.14674
                },
                {
                    "k": "2024-09-30",
                    "v": 9.12088
                },
                {
                    "k": "2024-12-31",
                    "v": -6.81437
                },
                {
                    "k": "2025-03-31",
                    "v": 5.36744
                },
                {
                    "k": "2025-06-30",
                    "v": 2.49573
                },
                {
                    "k": "2025-09-30",
                    "v": 37.95864
                },
                {
                    "k": "2025-12-31",
                    "v": 3.5058
                }
            ],
            "categoryReturns": [
                {
                    "k": "2023-03-31",
                    "v": 3.18804
                },
                {
                    "k": "2023-06-30",
                    "v": -4.57335
                },
                {
                    "k": "2023-09-30",
                    "v": -9.15781
                },
                {
                    "k": "2023-12-31",
                    "v": -5.18842
                },
                {
                    "k": "2024-03-31",
                    "v": -2.4157
                },
                {
                    "k": "2024-06-30",
                    "v": -3.37632
                },
                {
                    "k": "2024-09-30",
                    "v": 15.65191
                },
                {
                    "k": "2024-12-31",
                    "v": -1.19314
                },
                {
                    "k": "2025-03-31",
                    "v": 1.36619
                },
                {
                    "k": "2025-06-30",
                    "v": 1.46824
                },
                {
                    "k": "2025-09-30",
                    "v": 33.98151
                },
                {
                    "k": "2025-12-31",
                    "v": -0.45902
                }
            ],
            "benchmarkReturns": [
                {
                    "k": "2023-03-31",
                    "v": 4.45635
                },
                {
                    "k": "2023-06-30",
                    "v": -7.78001
                },
                {
                    "k": "2023-09-30",
                    "v": -6.86494
                },
                {
                    "k": "2023-12-31",
                    "v": -7.43837
                },
                {
                    "k": "2024-03-31",
                    "v": -1.04882
                },
                {
                    "k": "2024-06-30",
                    "v": -5.20946
                },
                {
                    "k": "2024-09-30",
                    "v": 19.51835
                },
                {
                    "k": "2024-12-31",
                    "v": -3.42089
                },
                {
                    "k": "2025-03-31",
                    "v": 0.32626
                },
                {
                    "k": "2025-06-30",
                    "v": 1.116
                },
                {
                    "k": "2025-09-30",
                    "v": 33.29891
                },
                {
                    "k": "2025-12-31",
                    "v": -2.05907
                }
            ],
            "messages": [],
            "tips": [
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null
            ]
        },
        "annual": {
            "lastCalendarYear": 2025,
            "returns": [
                {
                    "k": 2016,
                    "v": -25.69256
                },
                {
                    "k": 2017,
                    "v": 27.77778
                },
                {
                    "k": 2018,
                    "v": -31.46453
                },
                {
                    "k": 2019,
                    "v": 79.54925
                },
                {
                    "k": 2020,
                    "v": 66.0623
                },
                {
                    "k": 2021,
                    "v": 15.14558
                },
                {
                    "k": 2022,
                    "v": -29.12716
                },
                {
                    "k": 2023,
                    "v": -0.6518
                },
                {
                    "k": 2024,
                    "v": -4.14365
                },
                {
                    "k": 2025,
                    "v": 54.2147
                }
            ],
            "categoryReturns": [
                {
                    "k": 2016,
                    "v": -13.35775
                },
                {
                    "k": 2017,
                    "v": 12.14307
                },
                {
                    "k": 2018,
                    "v": -27.82696
                },
                {
                    "k": 2019,
                    "v": 47.66434
                },
                {
                    "k": 2020,
                    "v": 63.14224
                },
                {
                    "k": 2021,
                    "v": 10.89105
                },
                {
                    "k": 2022,
                    "v": -23.42928
                },
                {
                    "k": 2023,
                    "v": -15.18981
                },
                {
                    "k": 2024,
                    "v": 7.74657
                },
                {
                    "k": 2025,
                    "v": 37.17344
                }
            ],
            "benchmarkReturns": [
                {
                    "k": 2016,
                    "v": -14.9585
                },
                {
                    "k": 2017,
                    "v": 18.06731
                },
                {
                    "k": 2018,
                    "v": -29.37655
                },
                {
                    "k": 2019,
                    "v": 52.80903
                },
                {
                    "k": 2020,
                    "v": 48.11375
                },
                {
                    "k": 2021,
                    "v": -3.97729
                },
                {
                    "k": 2022,
                    "v": -26.34587
                },
                {
                    "k": 2023,
                    "v": -16.95679
                },
                {
                    "k": 2024,
                    "v": 8.26891
                },
                {
                    "k": 2025,
                    "v": 32.44187
                }
            ],
            "tips": [
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null
            ],
            "returnRanks": [
                {
                    "k": 2016,
                    "v": 89
                },
                {
                    "k": 2017,
                    "v": 17
                },
                {
                    "k": 2018,
                    "v": 68
                },
                {
                    "k": 2019,
                    "v": 2
                },
                {
                    "k": 2020,
                    "v": 16
                },
                {
                    "k": 2021,
                    "v": 32
                },
                {
                    "k": 2022,
                    "v": 86
                },
                {
                    "k": 2023,
                    "v": 4
                },
                {
                    "k": 2024,
                    "v": 89
                },
                {
                    "k": 2025,
                    "v": 22
                }
            ],
            "investmentsInCategory": [
                {
                    "k": 2016,
                    "v": 143
                },
                {
                    "k": 2017,
                    "v": 157
                },
                {
                    "k": 2018,
                    "v": 195
                },
                {
                    "k": 2019,
                    "v": 229
                },
                {
                    "k": 2020,
                    "v": 955
                },
                {
                    "k": 2021,
                    "v": 406
                },
                {
                    "k": 2022,
                    "v": 459
                },
                {
                    "k": 2023,
                    "v": 525
                },
                {
                    "k": 2024,
                    "v": 579
                },
                {
                    "k": 2025,
                    "v": 621
                }
            ]
        },
        "rating": {
            "ratingDate": "2025-12",
            "Y3": "5",
            "Y5": "4",
            "Y10": "4",
            "Y3History": [
                {
                    "k": "2017-06",
                    "v": "1"
                },
                {
                    "k": "2017-09",
                    "v": "1"
                },
                {
                    "k": "2017-12",
                    "v": "3"
                },
                {
                    "k": "2018-03",
                    "v": "2"
                },
                {
                    "k": "2018-06",
                    "v": "3"
                },
                {
                    "k": "2018-09",
                    "v": "3"
                },
                {
                    "k": "2018-12",
                    "v": "2"
                },
                {
                    "k": "2019-03",
                    "v": "3"
                },
                {
                    "k": "2019-06",
                    "v": "3"
                },
                {
                    "k": "2019-09",
                    "v": "4"
                },
                {
                    "k": "2019-12",
                    "v": "5"
                },
                {
                    "k": "2020-03",
                    "v": "5"
                },
                {
                    "k": "2020-06",
                    "v": "5"
                },
                {
                    "k": "2020-09",
                    "v": "5"
                },
                {
                    "k": "2020-10",
                    "v": "5"
                },
                {
                    "k": "2020-12",
                    "v": "4"
                },
                {
                    "k": "2021-04",
                    "v": "5"
                },
                {
                    "k": "2021-06",
                    "v": "5"
                },
                {
                    "k": "2021-09",
                    "v": "4"
                },
                {
                    "k": "2021-10",
                    "v": "4"
                },
                {
                    "k": "2021-12",
                    "v": "4"
                },
                {
                    "k": "2022-03",
                    "v": "4"
                },
                {
                    "k": "2022-06",
                    "v": "4"
                },
                {
                    "k": "2022-09",
                    "v": "3"
                },
                {
                    "k": "2022-12",
                    "v": "3"
                },
                {
                    "k": "2023-03",
                    "v": "3"
                },
                {
                    "k": "2023-06",
                    "v": "3"
                },
                {
                    "k": "2023-09",
                    "v": "4"
                },
                {
                    "k": "2023-12",
                    "v": "4"
                },
                {
                    "k": "2024-03",
                    "v": "4"
                },
                {
                    "k": "2024-06",
                    "v": "4"
                },
                {
                    "k": "2024-09",
                    "v": "4"
                },
                {
                    "k": "2024-12",
                    "v": "3"
                },
                {
                    "k": "2025-03",
                    "v": "4"
                },
                {
                    "k": "2025-06",
                    "v": "4"
                },
                {
                    "k": "2025-09",
                    "v": "4"
                },
                {
                    "k": "2025-12",
                    "v": "5"
                }
            ]
        },
        "risk": {
            "riskDate": "2026-02-28",
            "Y1": {
                "risk": {
                    "return": 61.94573,
                    "returnRankOver": 88.73,
                    "stdDev": 24.91079,
                    "stdDevRankOver": 44.44,
                    "maxDrawdown": -18.66197,
                    "maxDrawdownRankOver": 27.01,
                    "downsideDeviation": 7.61905,
                    "downsideDeviationRankOver": 49.54,
                    "morningstarRisk": 7.91983,
                    "morningstarRiskRankOver": 38.12,
                    "sharpeRatio": 2.03872,
                    "sharpeRatioRankOver": 86.11,
                    "calmarRatio": 3.31936,
                    "calmarRatioRankOver": 77.47,
                    "sortinoRatio": 6.66567,
                    "sortinoRatioRankOver": 82.66,
                    "alphaWI": 10.9595,
                    "alphaWIRankOver": 81.64,
                    "betaWI": 1.83936,
                    "betaWIRankOver": 61.73,
                    "rSquaredWI": 80.33053,
                    "rSquaredWIRankOver": 40.12,
                    "battingAverageWI": 58.33333,
                    "battingAverageWIRankOver": 82.25,
                    "upsideCaptureRatioWI": 226.12968,
                    "upsideCaptureRatioWIRankOver": 88.73,
                    "downsideCaptureRatioWI": 226.61389,
                    "downsideCaptureRatioWIRankOver": 40.12,
                    "alphaCAI": 16.8439,
                    "alphaCAIRankOver": 87.19,
                    "betaCAI": 1.08607,
                    "betaCAIRankOver": 55.25,
                    "rSquaredCAI": 74.69082,
                    "rSquaredCAIRankOver": 27.31,
                    "battingAverageCAI": 66.66667,
                    "battingAverageCAIRankOver": 89.51,
                    "upsideCaptureRatioCAI": 134.90789,
                    "upsideCaptureRatioCAIRankOver": 62.65,
                    "downsideCaptureRatioCAI": 56.04359,
                    "downsideCaptureRatioCAIRankOver": 79.94,
                    "alphaCA": 9.2237,
                    "alphaCARankOver": 83.95,
                    "betaCA": 1.13952,
                    "betaCARankOver": 58.8,
                    "rSquaredCA": 85.61362,
                    "rSquaredCARankOver": 37.04,
                    "battingAverageCA": 66.66667,
                    "battingAverageCARankOver": 95.99,
                    "upsideCaptureRatioCA": 134.87767,
                    "upsideCaptureRatioCARankOver": 82.72,
                    "downsideCaptureRatioCA": 122.64323,
                    "downsideCaptureRatioCARankOver": 47.07,
                    "alphaPB": 6.55457,
                    "alphaPBRankOver": 77.36,
                    "betaPB": 2.2314,
                    "betaPBRankOver": 94.26,
                    "rSquaredPB": 85.12058,
                    "rSquaredPBRankOver": 26.2,
                    "battingAveragePB": 58.33333,
                    "battingAveragePBRankOver": 51.47,
                    "upsideCaptureRatioPB": 247.57997,
                    "upsideCaptureRatioPBRankOver": 93.33,
                    "downsideCaptureRatioPB": 257.38206,
                    "downsideCaptureRatioPBRankOver": 5.74,
                    "excessPB": 39.47206,
                    "excessPBRankOver": 92.4,
                    "trackPB": 15.91213,
                    "trackPBRankOver": 12.56,
                    "infoPB": 2.48063,
                    "infoPBRankOver": 66.36,
                    "excessCA": 19.64411,
                    "excessCARankOver": 88.73,
                    "trackCA": 9.86095,
                    "trackCARankOver": 60.19,
                    "infoCA": 1.99211,
                    "infoCARankOver": 92.28,
                    "excessCAI": 26.58773,
                    "excessCAIRankOver": 88.73,
                    "trackCAI": 12.6478,
                    "trackCAIRankOver": 38.27,
                    "infoCAI": 2.10216,
                    "infoCAIRankOver": 89.81,
                    "excessWI": 37.46695,
                    "excessWIRankOver": 88.73,
                    "trackWI": 15.02878,
                    "trackWIRankOver": 48.61,
                    "infoWI": 2.49301,
                    "infoWIRankOver": 92.28
                },
                "categoryRisk": {
                    "return": 42.30163,
                    "stdDev": 20.2273,
                    "maxDrawdown": -14.01573,
                    "downsideDeviation": 5.79742,
                    "morningstarRisk": 4.55728,
                    "sharpeRatio": 1.80319,
                    "calmarRatio": 3.01815,
                    "sortinoRatio": 6.29136,
                    "alphaWI": 1.71866,
                    "betaWI": 1.60514,
                    "rSquaredWI": 92.78301,
                    "battingAverageWI": 58.33333,
                    "upsideCaptureRatioWI": 162.60512,
                    "downsideCaptureRatioWI": 160.16153,
                    "alphaCAI": 4.97241,
                    "betaCAI": 1.00797,
                    "rSquaredCAI": 97.57589,
                    "battingAverageCAI": 66.66667,
                    "upsideCaptureRatioCAI": 111.13576,
                    "downsideCaptureRatioCAI": 93.44582,
                    "alphaCA": 0.0,
                    "betaCA": 1.0,
                    "rSquaredCA": 100.0,
                    "battingAverageCA": 100.0,
                    "upsideCaptureRatioCA": 100.0,
                    "downsideCaptureRatioCA": 100.0,
                    "excessCA": 0.0,
                    "trackCA": 0.0,
                    "excessCAI": 6.94363,
                    "trackCAI": 3.15326,
                    "infoCAI": 2.20204,
                    "excessWI": 17.82285,
                    "trackWI": 9.13687,
                    "infoWI": 1.95065
                },
                "benchmarkRisk": {
                    "return": 35.358,
                    "stdDev": 19.82265,
                    "maxDrawdown": -11.68859,
                    "downsideDeviation": 5.41456,
                    "morningstarRisk": 4.09506,
                    "sharpeRatio": 1.57659,
                    "calmarRatio": 3.025,
                    "sortinoRatio": 5.77187,
                    "alphaWI": -2.66241,
                    "betaWI": 1.56632,
                    "rSquaredWI": 91.99395,
                    "battingAverageWI": 41.66667,
                    "upsideCaptureRatioWI": 139.20411,
                    "downsideCaptureRatioWI": 138.4254,
                    "alphaCAI": 0.0,
                    "betaCAI": 1.0,
                    "rSquaredCAI": 100.0,
                    "battingAverageCAI": 100.0,
                    "upsideCaptureRatioCAI": 100.0,
                    "downsideCaptureRatioCAI": 100.0,
                    "alphaCA": -4.05592,
                    "betaCA": 0.96804,
                    "rSquaredCA": 97.57589,
                    "battingAverageCA": 33.33333,
                    "upsideCaptureRatioCA": 86.23285,
                    "downsideCaptureRatioCA": 89.25815,
                    "excessCA": -6.94363,
                    "trackCA": 3.15326,
                    "infoCA": -21.89508,
                    "excessCAI": 0.0,
                    "trackCAI": 0.0,
                    "excessWI": 10.87922,
                    "trackWI": 8.87208,
                    "infoWI": 1.22623
                },
                "benchmarkNames": [
                    {
                        "k": "WI",
                        "v": "沪深300全收益"
                    },
                    {
                        "k": "CAI",
                        "v": "沪深300相对全收益成长指数"
                    },
                    {
                        "k": "CA",
                        "v": "同类平均"
                    },
                    {
                        "k": "PB",
                        "v": "业绩比较基准"
                    }
                ]
            },
            "Y3": {
                "risk": {
                    "return": 14.99221,
                    "returnRankOver": 84.36,
                    "stdDev": 24.85393,
                    "stdDevRankOver": 53.3,
                    "maxDrawdown": -29.70418,
                    "maxDrawdownRankOver": 86.78,
                    "downsideDeviation": 12.82183,
                    "downsideDeviationRankOver": 52.42,
                    "morningstarRisk": 6.20901,
                    "morningstarRiskRankOver": 49.12,
                    "sharpeRatio": 0.6387,
                    "sharpeRatioRankOver": 89.21,
                    "calmarRatio": 1.75495,
                    "calmarRatioRankOver": 91.63,
                    "sortinoRatio": 1.23805,
                    "sortinoRatioRankOver": 79.3,
                    "alphaWI": 6.47241,
                    "alphaWIRankOver": 88.77,
                    "betaWI": 1.16975,
                    "betaWIRankOver": 39.65,
                    "rSquaredWI": 67.97851,
                    "rSquaredWIRankOver": 37.22,
                    "battingAverageWI": 50.0,
                    "battingAverageWIRankOver": 70.26,
                    "upsideCaptureRatioWI": 142.42263,
                    "upsideCaptureRatioWIRankOver": 66.96,
                    "downsideCaptureRatioWI": 120.86186,
                    "downsideCaptureRatioWIRankOver": 65.64,
                    "alphaCAI": 9.74942,
                    "alphaCAIRankOver": 87.89,
                    "betaCAI": 0.93912,
                    "betaCAIRankOver": 40.75,
                    "rSquaredCAI": 77.41231,
                    "rSquaredCAIRankOver": 35.24,
                    "battingAverageCAI": 58.33333,
                    "battingAverageCAIRankOver": 76.43,
                    "upsideCaptureRatioCAI": 105.39156,
                    "upsideCaptureRatioCAIRankOver": 57.93,
                    "downsideCaptureRatioCAI": 69.8908,
                    "downsideCaptureRatioCAIRankOver": 77.75,
                    "alphaCA": 6.20753,
                    "alphaCARankOver": 88.55,
                    "betaCA": 1.04231,
                    "betaCARankOver": 44.71,
                    "rSquaredCA": 85.88161,
                    "rSquaredCARankOver": 43.83,
                    "battingAverageCA": 58.33333,
                    "battingAverageCARankOver": 91.85,
                    "upsideCaptureRatioCA": 124.62458,
                    "upsideCaptureRatioCARankOver": 71.81,
                    "downsideCaptureRatioCA": 104.41149,
                    "downsideCaptureRatioCARankOver": 55.07,
                    "alphaPB": 6.90016,
                    "alphaPBRankOver": 89.58,
                    "betaPB": 1.46331,
                    "betaPBRankOver": 95.12,
                    "rSquaredPB": 73.84802,
                    "rSquaredPBRankOver": 30.82,
                    "battingAveragePB": 55.55556,
                    "battingAveragePBRankOver": 52.77,
                    "upsideCaptureRatioPB": 182.06967,
                    "upsideCaptureRatioPBRankOver": 96.67,
                    "downsideCaptureRatioPB": 156.9363,
                    "downsideCaptureRatioPBRankOver": 7.32,
                    "excessPB": 8.58428,
                    "excessPBRankOver": 92.46,
                    "trackPB": 14.39703,
                    "trackPBRankOver": 21.29,
                    "infoPB": 0.59625,
                    "infoPBRankOver": 53.66,
                    "excessCA": 6.58406,
                    "excessCARankOver": 84.36,
                    "trackCA": 9.38542,
                    "trackCARankOver": 65.64,
                    "infoCA": 0.70152,
                    "infoCARankOver": 91.63,
                    "excessCAI": 9.7478,
                    "excessCAIRankOver": 84.36,
                    "trackCAI": 11.89697,
                    "trackCAIRankOver": 40.75,
                    "infoCAI": 0.81935,
                    "infoCAIRankOver": 86.56,
                    "excessWI": 6.99508,
                    "excessWIRankOver": 84.36,
                    "trackWI": 14.37517,
                    "trackWIRankOver": 55.73,
                    "infoWI": 0.48661,
                    "infoWIRankOver": 90.31
                },
                "categoryRisk": {
                    "return": 8.41649,
                    "stdDev": 22.09766,
                    "maxDrawdown": -32.28863,
                    "downsideDeviation": 10.79167,
                    "morningstarRisk": 4.43581,
                    "sharpeRatio": 0.41969,
                    "calmarRatio": 0.85075,
                    "sortinoRatio": 0.85938,
                    "alphaWI": -0.11542,
                    "betaWI": 1.16824,
                    "rSquaredWI": 85.7722,
                    "battingAverageWI": 50.0,
                    "upsideCaptureRatioWI": 118.28727,
                    "downsideCaptureRatioWI": 124.01911,
                    "alphaCAI": 3.19997,
                    "betaCAI": 0.93137,
                    "rSquaredCAI": 96.31917,
                    "battingAverageCAI": 55.55556,
                    "upsideCaptureRatioCAI": 95.23497,
                    "downsideCaptureRatioCAI": 82.31017,
                    "alphaCA": 0.0,
                    "betaCA": 1.0,
                    "rSquaredCA": 100.0,
                    "battingAverageCA": 100.0,
                    "upsideCaptureRatioCA": 100.0,
                    "downsideCaptureRatioCA": 100.0,
                    "excessCA": 0.0,
                    "trackCA": 0.0,
                    "excessCAI": 3.16374,
                    "trackCAI": 4.53071,
                    "infoCAI": 0.69829,
                    "excessWI": 0.41103,
                    "trackWI": 8.84092,
                    "infoWI": 0.04649
                },
                "benchmarkRisk": {
                    "return": 5.25659,
                    "stdDev": 23.28516,
                    "maxDrawdown": -33.09357,
                    "downsideDeviation": 11.65084,
                    "morningstarRisk": 4.65949,
                    "sharpeRatio": 0.28008,
                    "calmarRatio": 0.50263,
                    "sortinoRatio": 0.55976,
                    "alphaWI": -3.76946,
                    "betaWI": 1.28042,
                    "rSquaredWI": 92.79444,
                    "battingAverageWI": 33.33333,
                    "upsideCaptureRatioWI": 121.47008,
                    "downsideCaptureRatioWI": 147.45051,
                    "alphaCAI": 0.0,
                    "betaCAI": 1.0,
                    "rSquaredCAI": 100.0,
                    "battingAverageCAI": 100.0,
                    "upsideCaptureRatioCAI": 100.0,
                    "downsideCaptureRatioCAI": 100.0,
                    "alphaCA": -3.06924,
                    "betaCA": 1.03416,
                    "rSquaredCA": 96.31917,
                    "battingAverageCA": 44.44444,
                    "upsideCaptureRatioCA": 96.40997,
                    "downsideCaptureRatioCA": 109.36295,
                    "excessCA": -3.16374,
                    "trackCA": 4.53071,
                    "infoCA": -14.334,
                    "excessCAI": 0.0,
                    "trackCAI": 0.0,
                    "excessWI": -2.75271,
                    "trackWI": 7.9499,
                    "infoWI": -21.88381
                },
                "benchmarkNames": [
                    {
                        "k": "WI",
                        "v": "沪深300全收益"
                    },
                    {
                        "k": "CAI",
                        "v": "沪深300相对全收益成长指数"
                    },
                    {
                        "k": "CA",
                        "v": "同类平均"
                    },
                    {
                        "k": "PB",
                        "v": "业绩比较基准"
                    }
                ]
            },
            "Y5": {
                "risk": {
                    "return": 5.78483,
                    "returnRankOver": 83.62,
                    "stdDev": 23.31624,
                    "stdDevRankOver": 57.84,
                    "maxDrawdown": -44.17535,
                    "maxDrawdownRankOver": 89.9,
                    "downsideDeviation": 13.99547,
                    "downsideDeviationRankOver": 62.02,
                    "morningstarRisk": 5.24847,
                    "morningstarRiskRankOver": 52.96,
                    "sharpeRatio": 0.30725,
                    "sharpeRatioRankOver": 83.28,
                    "calmarRatio": 0.73537,
                    "calmarRatioRankOver": 84.67,
                    "sortinoRatio": 0.51187,
                    "sortinoRatioRankOver": 81.53,
                    "alphaWI": 6.68979,
                    "alphaWIRankOver": 77.35,
                    "betaWI": 1.05323,
                    "betaWIRankOver": 49.48,
                    "rSquaredWI": 65.95881,
                    "rSquaredWIRankOver": 55.05,
                    "battingAverageWI": 53.33333,
                    "battingAverageWIRankOver": 83.97,
                    "upsideCaptureRatioWI": 127.16642,
                    "upsideCaptureRatioWIRankOver": 71.43,
                    "downsideCaptureRatioWI": 100.77906,
                    "downsideCaptureRatioWIRankOver": 56.45,
                    "alphaCAI": 8.64586,
                    "alphaCAIRankOver": 72.47,
                    "betaCAI": 0.88687,
                    "betaCAIRankOver": 43.9,
                    "rSquaredCAI": 75.90746,
                    "rSquaredCAIRankOver": 49.48,
                    "battingAverageCAI": 60.0,
                    "battingAverageCAIRankOver": 83.97,
                    "upsideCaptureRatioCAI": 94.53568,
                    "upsideCaptureRatioCAIRankOver": 53.66,
                    "downsideCaptureRatioCAI": 66.96725,
                    "downsideCaptureRatioCAIRankOver": 72.47,
                    "alphaCA": 3.64995,
                    "alphaCARankOver": 80.49,
                    "betaCA": 1.01416,
                    "betaCARankOver": 47.74,
                    "rSquaredCA": 83.77764,
                    "rSquaredCARankOver": 49.83,
                    "battingAverageCA": 55.0,
                    "battingAverageCARankOver": 88.5,
                    "upsideCaptureRatioCA": 112.97696,
                    "upsideCaptureRatioCARankOver": 59.23,
                    "downsideCaptureRatioCA": 100.87173,
                    "downsideCaptureRatioCARankOver": 64.11,
                    "alphaPB": 6.14435,
                    "alphaPBRankOver": 81.18,
                    "betaPB": 1.40269,
                    "betaPBRankOver": 98.61,
                    "rSquaredPB": 74.24637,
                    "rSquaredPBRankOver": 43.21,
                    "battingAveragePB": 53.33333,
                    "battingAveragePBRankOver": 52.61,
                    "upsideCaptureRatioPB": 164.29529,
                    "upsideCaptureRatioPBRankOver": 96.17,
                    "downsideCaptureRatioPB": 136.82345,
                    "downsideCaptureRatioPBRankOver": 6.97,
                    "excessPB": 4.93921,
                    "excessPBRankOver": 82.23,
                    "trackPB": 13.16343,
                    "trackPBRankOver": 37.63,
                    "infoPB": 0.37522,
                    "infoPBRankOver": 51.22,
                    "excessCA": 3.30326,
                    "excessCARankOver": 83.62,
                    "trackCA": 9.39581,
                    "trackCARankOver": 66.2,
                    "infoCA": 0.35157,
                    "infoCARankOver": 85.02,
                    "excessCAI": 8.80447,
                    "excessCAIRankOver": 83.62,
                    "trackCAI": 11.7343,
                    "trackCAIRankOver": 52.96,
                    "infoCAI": 0.75032,
                    "infoCAIRankOver": 87.8,
                    "excessWI": 5.77224,
                    "excessWIRankOver": 83.62,
                    "trackWI": 13.63745,
                    "trackWIRankOver": 69.69,
                    "infoWI": 0.42326,
                    "infoWIRankOver": 87.8
                },
                "categoryRisk": {
                    "return": 2.48298,
                    "stdDev": 21.04333,
                    "maxDrawdown": -47.20298,
                    "downsideDeviation": 12.23398,
                    "morningstarRisk": 4.01388,
                    "sharpeRatio": 0.16465,
                    "calmarRatio": 0.27652,
                    "sortinoRatio": 0.28321,
                    "alphaWI": 3.00191,
                    "betaWI": 1.02846,
                    "rSquaredWI": 77.21289,
                    "battingAverageWI": 51.66667,
                    "upsideCaptureRatioWI": 109.25883,
                    "downsideCaptureRatioWI": 97.91761,
                    "alphaCAI": 4.92749,
                    "betaCAI": 0.87529,
                    "rSquaredCAI": 90.77345,
                    "battingAverageCAI": 60.0,
                    "upsideCaptureRatioCAI": 91.76472,
                    "downsideCaptureRatioCAI": 74.9493,
                    "alphaCA": 0.0,
                    "betaCA": 1.0,
                    "rSquaredCA": 100.0,
                    "battingAverageCA": 100.0,
                    "upsideCaptureRatioCA": 100.0,
                    "downsideCaptureRatioCA": 100.0,
                    "excessCA": 0.0,
                    "trackCA": 0.0,
                    "excessCAI": 5.50121,
                    "trackCAI": 7.00123,
                    "infoCAI": 0.78575,
                    "excessWI": 2.46898,
                    "trackWI": 10.05823,
                    "infoWI": 0.24547
                },
                "benchmarkRisk": {
                    "return": -3.01598,
                    "stdDev": 22.90564,
                    "maxDrawdown": -51.5745,
                    "downsideDeviation": 14.21868,
                    "morningstarRisk": 4.43797,
                    "sharpeRatio": -3.18968,
                    "calmarRatio": -0.27538,
                    "sortinoRatio": -0.11752,
                    "alphaWI": -2.22262,
                    "betaWI": 1.22541,
                    "rSquaredWI": 92.51697,
                    "battingAverageWI": 40.0,
                    "upsideCaptureRatioWI": 117.72292,
                    "downsideCaptureRatioWI": 130.75822,
                    "alphaCAI": 0.0,
                    "betaCAI": 1.0,
                    "rSquaredCAI": 100.0,
                    "battingAverageCAI": 100.0,
                    "upsideCaptureRatioCAI": 100.0,
                    "downsideCaptureRatioCAI": 100.0,
                    "alphaCA": -5.26432,
                    "betaCA": 1.03707,
                    "rSquaredCA": 90.77345,
                    "battingAverageCA": 40.0,
                    "upsideCaptureRatioCA": 94.20931,
                    "downsideCaptureRatioCA": 116.0093,
                    "excessCA": -5.50121,
                    "trackCA": 7.00123,
                    "infoCA": -38.51525,
                    "excessCAI": 0.0,
                    "trackCAI": 0.0,
                    "excessWI": -3.03223,
                    "trackWI": 7.46228,
                    "infoWI": -22.62736
                },
                "benchmarkNames": [
                    {
                        "k": "WI",
                        "v": "沪深300全收益"
                    },
                    {
                        "k": "CAI",
                        "v": "沪深300相对全收益成长指数"
                    },
                    {
                        "k": "CA",
                        "v": "同类平均"
                    },
                    {
                        "k": "PB",
                        "v": "业绩比较基准"
                    }
                ]
            },
            "Y10": {
                "risk": {
                    "return": 14.34613,
                    "returnRankOver": 87.63,
                    "stdDev": 22.47851,
                    "stdDevRankOver": 31.96,
                    "maxDrawdown": -44.51131,
                    "maxDrawdownRankOver": 86.6,
                    "downsideDeviation": 12.04981,
                    "downsideDeviationRankOver": 54.64,
                    "morningstarRisk": 5.25188,
                    "morningstarRiskRankOver": 28.87,
                    "sharpeRatio": 0.69413,
                    "sharpeRatioRankOver": 87.63,
                    "calmarRatio": 6.34319,
                    "calmarRatioRankOver": 87.63,
                    "sortinoRatio": 1.29488,
                    "sortinoRatioRankOver": 86.6,
                    "alphaWI": 7.61432,
                    "alphaWIRankOver": 86.6,
                    "betaWI": 1.04381,
                    "betaWIRankOver": 61.86,
                    "rSquaredWI": 65.44774,
                    "rSquaredWIRankOver": 62.89,
                    "battingAverageWI": 58.33333,
                    "battingAverageWIRankOver": 94.85,
                    "upsideCaptureRatioWI": 117.2967,
                    "upsideCaptureRatioWIRankOver": 93.81,
                    "downsideCaptureRatioWI": 86.38789,
                    "downsideCaptureRatioWIRankOver": 61.86,
                    "alphaCAI": 8.67771,
                    "alphaCAIRankOver": 85.57,
                    "betaCAI": 0.92366,
                    "betaCAIRankOver": 67.01,
                    "rSquaredCAI": 77.96404,
                    "rSquaredCAIRankOver": 56.7,
                    "battingAverageCAI": 63.33333,
                    "battingAverageCAIRankOver": 98.97,
                    "upsideCaptureRatioCAI": 98.58889,
                    "upsideCaptureRatioCAIRankOver": 83.51,
                    "downsideCaptureRatioCAI": 65.79021,
                    "downsideCaptureRatioCAIRankOver": 74.23,
                    "alphaCA": 4.63903,
                    "alphaCARankOver": 85.57,
                    "betaCA": 1.04856,
                    "betaCARankOver": 72.16,
                    "rSquaredCA": 86.25983,
                    "rSquaredCARankOver": 55.67,
                    "battingAverageCA": 59.16667,
                    "battingAverageCARankOver": 97.94,
                    "upsideCaptureRatioCA": 113.37898,
                    "upsideCaptureRatioCARankOver": 83.51,
                    "downsideCaptureRatioCA": 96.55539,
                    "downsideCaptureRatioCARankOver": 65.98,
                    "alphaPB": 8.95035,
                    "alphaPBRankOver": 83.51,
                    "betaPB": 1.39123,
                    "betaPBRankOver": 98.97,
                    "rSquaredPB": 72.90271,
                    "rSquaredPBRankOver": 45.36,
                    "battingAveragePB": 59.16667,
                    "battingAveragePBRankOver": 75.26,
                    "upsideCaptureRatioPB": 159.40768,
                    "upsideCaptureRatioPBRankOver": 97.94,
                    "downsideCaptureRatioPB": 114.09503,
                    "downsideCaptureRatioPBRankOver": 10.31,
                    "excessPB": 10.1666,
                    "excessPBRankOver": 90.72,
                    "trackPB": 12.88597,
                    "trackPBRankOver": 37.11,
                    "infoPB": 0.78897,
                    "infoPBRankOver": 76.29,
                    "excessCA": 5.15047,
                    "excessCARankOver": 90.72,
                    "trackCA": 8.38819,
                    "trackCARankOver": 62.89,
                    "infoCA": 0.61401,
                    "infoCARankOver": 96.91,
                    "excessCAI": 8.67027,
                    "excessCAIRankOver": 90.72,
                    "trackCAI": 10.67873,
                    "trackCAIRankOver": 65.98,
                    "infoCAI": 0.81192,
                    "infoCAIRankOver": 94.85,
                    "excessWI": 7.70234,
                    "excessWIRankOver": 90.72,
                    "trackWI": 13.23516,
                    "trackWIRankOver": 60.82,
                    "infoWI": 0.58196,
                    "infoWIRankOver": 92.78
                },
                "categoryRisk": {
                    "return": 9.52317,
                    "stdDev": 19.91025,
                    "maxDrawdown": -47.20298,
                    "downsideDeviation": 10.90451,
                    "morningstarRisk": 3.91912,
                    "sharpeRatio": 0.52517,
                    "calmarRatio": 3.14473,
                    "sortinoRatio": 0.95889,
                    "alphaWI": 2.80447,
                    "betaWI": 0.99978,
                    "rSquaredWI": 76.53191,
                    "battingAverageWI": 55.0,
                    "upsideCaptureRatioWI": 101.42618,
                    "downsideCaptureRatioWI": 89.44214,
                    "alphaCAI": 3.84688,
                    "betaCAI": 0.88151,
                    "rSquaredCAI": 90.51287,
                    "battingAverageCAI": 55.0,
                    "upsideCaptureRatioCAI": 90.41185,
                    "downsideCaptureRatioCAI": 74.81511,
                    "alphaCA": 0.0,
                    "betaCA": 1.0,
                    "rSquaredCA": 100.0,
                    "battingAverageCA": 100.0,
                    "upsideCaptureRatioCA": 100.0,
                    "downsideCaptureRatioCA": 100.0,
                    "excessCA": 0.0,
                    "trackCA": 0.0,
                    "excessCAI": 3.5198,
                    "trackCAI": 6.64014,
                    "infoCAI": 0.53008,
                    "excessWI": 2.55186,
                    "trackWI": 9.6453,
                    "infoWI": 0.26457
                },
                "benchmarkRisk": {
                    "return": 6.18515,
                    "stdDev": 21.4884,
                    "maxDrawdown": -56.47145,
                    "downsideDeviation": 12.48425,
                    "morningstarRisk": 4.40148,
                    "sharpeRatio": 0.34892,
                    "calmarRatio": 1.45706,
                    "sortinoRatio": 0.60058,
                    "alphaWI": -1.56861,
                    "betaWI": 1.18462,
                    "rSquaredWI": 92.24252,
                    "battingAverageWI": 41.66667,
                    "upsideCaptureRatioWI": 111.78473,
                    "downsideCaptureRatioWI": 120.50756,
                    "alphaCAI": 0.0,
                    "betaCAI": 1.0,
                    "rSquaredCAI": 100.0,
                    "battingAverageCAI": 100.0,
                    "upsideCaptureRatioCAI": 100.0,
                    "downsideCaptureRatioCAI": 100.0,
                    "alphaCA": -3.23861,
                    "betaCA": 1.02679,
                    "rSquaredCA": 90.51287,
                    "battingAverageCA": 45.0,
                    "upsideCaptureRatioCA": 94.77323,
                    "downsideCaptureRatioCA": 108.4603,
                    "excessCA": -3.5198,
                    "trackCA": 6.64014,
                    "infoCA": -23.37198,
                    "excessCAI": 0.0,
                    "trackCAI": 0.0,
                    "excessWI": -0.96794,
                    "trackWI": 6.79449,
                    "infoWI": -6.57665
                },
                "benchmarkNames": [
                    {
                        "k": "WI",
                        "v": "沪深300全收益"
                    },
                    {
                        "k": "CAI",
                        "v": "沪深300相对全收益成长指数"
                    },
                    {
                        "k": "CA",
                        "v": "同类平均"
                    },
                    {
                        "k": "PB",
                        "v": "业绩比较基准"
                    }
                ]
            }
        },
        "investorReturn": {
            "returnDate": "2025-12-31",
            "Y3": {
                "investorReturn": 11.44974,
                "return": 13.6673
            },
            "Y5": {
                "investorReturn": 0.95446,
                "return": 3.68758
            },
            "Y10": {
                "investorReturn": 7.20386,
                "return": 8.80507
            },
            "cashFlows": [
                {
                    "k": "2016-03-31",
                    "v": 1.5152718068E8
                },
                {
                    "k": "2016-06-30",
                    "v": -3.645485899E7
                },
                {
                    "k": "2016-09-30",
                    "v": -4.553257191E7
                },
                {
                    "k": "2016-12-31",
                    "v": -8.451008236E7
                },
                {
                    "k": "2017-03-31",
                    "v": -3.774271124E7
                },
                {
                    "k": "2017-06-30",
                    "v": -4.576813443E7
                },
                {
                    "k": "2017-09-30",
                    "v": -9.696051261E7
                },
                {
                    "k": "2017-12-31",
                    "v": -5.590044781E7
                },
                {
                    "k": "2018-03-31",
                    "v": -6.646994333E7
                },
                {
                    "k": "2018-06-30",
                    "v": -1.548199384E7
                },
                {
                    "k": "2018-09-30",
                    "v": -2.42756841E7
                },
                {
                    "k": "2018-12-31",
                    "v": 1445962.87
                },
                {
                    "k": "2019-03-31",
                    "v": -1.991454831E7
                },
                {
                    "k": "2019-06-30",
                    "v": -2.19270775E7
                },
                {
                    "k": "2019-09-30",
                    "v": -4.334662563E7
                },
                {
                    "k": "2019-12-31",
                    "v": 6.272885849E7
                },
                {
                    "k": "2020-03-31",
                    "v": 1.0915281477E8
                },
                {
                    "k": "2020-06-30",
                    "v": -8.523927161E7
                },
                {
                    "k": "2020-09-30",
                    "v": 1.8729552691E8
                },
                {
                    "k": "2020-12-31",
                    "v": 3.480537538E7
                },
                {
                    "k": "2021-03-31",
                    "v": -1.0371694768E8
                },
                {
                    "k": "2021-06-30",
                    "v": 1.819809361E8
                },
                {
                    "k": "2021-09-30",
                    "v": -2.9026703552E8
                },
                {
                    "k": "2021-12-31",
                    "v": -1.8078975808E8
                },
                {
                    "k": "2022-03-31",
                    "v": -2.8724724032E8
                },
                {
                    "k": "2022-06-30",
                    "v": -1.3668174936E8
                },
                {
                    "k": "2022-09-30",
                    "v": -3.014464096E7
                },
                {
                    "k": "2022-12-31",
                    "v": -1.455936136E7
                },
                {
                    "k": "2023-03-31",
                    "v": -1.100806806E7
                },
                {
                    "k": "2023-06-30",
                    "v": 3.009266813E7
                },
                {
                    "k": "2023-09-30",
                    "v": -1.079516594E7
                },
                {
                    "k": "2023-12-31",
                    "v": -6.378318374E7
                },
                {
                    "k": "2024-03-31",
                    "v": -1.46512929E7
                },
                {
                    "k": "2024-06-30",
                    "v": -2.490664649E7
                },
                {
                    "k": "2024-09-30",
                    "v": -1.848710075E7
                },
                {
                    "k": "2024-12-31",
                    "v": -1.896853962E7
                },
                {
                    "k": "2025-03-31",
                    "v": -1.957235594E7
                },
                {
                    "k": "2025-06-30",
                    "v": -1.306658158E7
                },
                {
                    "k": "2025-09-30",
                    "v": -1.0462490367E8
                },
                {
                    "k": "2025-12-31",
                    "v": -5.93217011E7
                }
            ],
            "quarterlyReturns": [
                {
                    "k": "2016-03-31",
                    "v": -22.27051
                },
                {
                    "k": "2016-06-30",
                    "v": 5.9399
                },
                {
                    "k": "2016-09-30",
                    "v": -2.37467
                },
                {
                    "k": "2016-12-31",
                    "v": -7.56757
                },
                {
                    "k": "2017-03-31",
                    "v": 4.09357
                },
                {
                    "k": "2017-06-30",
                    "v": 4.91573
                },
                {
                    "k": "2017-09-30",
                    "v": 9.70549
                },
                {
                    "k": "2017-12-31",
                    "v": 6.6504
                },
                {
                    "k": "2018-03-31",
                    "v": -2.746
                },
                {
                    "k": "2018-06-30",
                    "v": -11.52941
                },
                {
                    "k": "2018-09-30",
                    "v": -6.18351
                },
                {
                    "k": "2018-12-31",
                    "v": -15.09568
                },
                {
                    "k": "2019-03-31",
                    "v": 37.06177
                },
                {
                    "k": "2019-06-30",
                    "v": 3.10597
                },
                {
                    "k": "2019-09-30",
                    "v": 10.98641
                },
                {
                    "k": "2019-12-31",
                    "v": 14.47578
                },
                {
                    "k": "2020-03-31",
                    "v": 1.34821
                },
                {
                    "k": "2020-06-30",
                    "v": 31.00917
                },
                {
                    "k": "2020-09-30",
                    "v": 13.30532
                },
                {
                    "k": "2020-12-31",
                    "v": 10.38319
                },
                {
                    "k": "2021-03-31",
                    "v": -1.51176
                },
                {
                    "k": "2021-06-30",
                    "v": 13.55884
                },
                {
                    "k": "2021-09-30",
                    "v": -5.85732
                },
                {
                    "k": "2021-12-31",
                    "v": 9.35921
                },
                {
                    "k": "2022-03-31",
                    "v": -22.31947
                },
                {
                    "k": "2022-06-30",
                    "v": 9.70266
                },
                {
                    "k": "2022-09-30",
                    "v": -16.7475
                },
                {
                    "k": "2022-12-31",
                    "v": -0.10281
                },
                {
                    "k": "2023-03-31",
                    "v": 9.46827
                },
                {
                    "k": "2023-06-30",
                    "v": 2.47571
                },
                {
                    "k": "2023-09-30",
                    "v": -5.07645
                },
                {
                    "k": "2023-12-31",
                    "v": -6.70103
                },
                {
                    "k": "2024-03-31",
                    "v": -5.87017
                },
                {
                    "k": "2024-06-30",
                    "v": 0.14674
                },
                {
                    "k": "2024-09-30",
                    "v": 9.12088
                },
                {
                    "k": "2024-12-31",
                    "v": -6.81437
                },
                {
                    "k": "2025-03-31",
                    "v": 5.36744
                },
                {
                    "k": "2025-06-30",
                    "v": 2.49573
                },
                {
                    "k": "2025-09-30",
                    "v": 37.95864
                },
                {
                    "k": "2025-12-31",
                    "v": 3.5058
                }
            ]
        }
    }
}
```

## 获取基金基本信息的接口

请求： GET <https://www.morningstar.cn/cn-api/v2/funds/000513/common-data>
