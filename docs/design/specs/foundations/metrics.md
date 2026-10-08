---
name: metrics-contract
description: What every metric means on every surface — dashboard, usage page, tray rows.
---

# Metrics contract

One number means one thing wherever it is shown. The dashboard, the usage
page, and the tray's rows read the same rollup and count it the same way.
Where a surface covers a different window, a chip names it.

## Definitions

| Metric           | Rule                                                                                                                                |
|------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| Requests, errors | every logged request; an error is a status of 400 or more                                                                           |
| Tokens           | `input + output + cache_read + cache_write` — the whole traffic, cache included                                                     |
| Spend            | pay-as-you-go connections only; a plan-covered connection is plan usage, a local engine is neither, and both stay out of the figure |
| Success rate     | `(requests − errors) / requests`, and "—" when there is no traffic — never a perfect score                                          |
| Latency          | `duration / requests` over the window                                                                                               |
| Trend latency    | the request-weighted mean of the days' averages, never their sum                                                                    |
| Delta            | the previous window of equal length, rounded half away from zero                                                                    |
| Days             | UTC calendar days: a chart's bars and its labels share one calendar                                                                 |

## Windows

| Group                       | Window                           | Chip                |
|-----------------------------|----------------------------------|---------------------|
| KPI tiles (dashboard, tray) | rolling 24 h                     | `24 h`              |
| KPI delta                   | the 24 h before it               | the caption says so |
| Trend (dashboard)           | 7 UTC days                       | `7 days`            |
| Spend board                 | 24 h rolling, or 7 / 30 UTC days | the period          |
| Usage cards and summary     | 7 / 30 days, all time            | the range           |
| Usage trend                 | the range, capped at 30 days     | the cap             |

## Tests

`daemon/internal/desktop/metrics_test.go`
(`TestMetricsContractMatchesTheConsole`) and
`console/src/lib/metric-contract.test.ts` assert one fixture and the same
figures. Whichever side drifts, the two tests name the number that has to
mean the same thing everywhere.
