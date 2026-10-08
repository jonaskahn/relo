// Tray metrics: usage rows and paid-spend totals.
package desktop

import (
	"fmt"
	"strconv"
	"strings"
)

type metric uint8

const (
	metricRequests metric = iota
	metricTokens
	metricSpend
)

// UsageRow is one rollup row. Every counter is a float because rollups
// arrive from JSON numbers.
type UsageRow struct {
	Key              string  `json:"Key"`
	Requests         float64 `json:"Requests"`
	Errors           float64 `json:"Errors"`
	InputTokens      float64 `json:"InputTokens"`
	OutputTokens     float64 `json:"OutputTokens"`
	CacheReadTokens  float64 `json:"CacheReadTokens"`
	CacheWriteTokens float64 `json:"CacheWriteTokens"`
	CostMicros       float64 `json:"CostMicros"`
	DurationMs       float64 `json:"DurationMs"`
}

type usageTotals struct {
	requests float64
	errors   float64
	tokens   float64
	spend    float64
}

func withoutNonPaidSpend(rows []UsageRow, skip map[string]bool) []UsageRow {
	if len(skip) == 0 {
		return rows
	}
	out := make([]UsageRow, len(rows))
	copy(out, rows)
	for i := range out {
		if skip[out[i].Key] {
			out[i].CostMicros = 0
		}
	}
	return out
}

func totalsOf(rows []UsageRow) usageTotals {
	var total usageTotals
	for _, row := range rows {
		total.requests += row.Requests
		total.errors += row.Errors
		total.tokens += rowTokens(row)
		total.spend += row.CostMicros
	}
	return total
}

func (t usageTotals) value(m metric) float64 {
	switch m {
	case metricTokens:
		return t.tokens
	case metricSpend:
		return t.spend
	default:
		return t.requests
	}
}

func rowTokens(row UsageRow) float64 {
	return row.InputTokens + row.OutputTokens + row.CacheReadTokens + row.CacheWriteTokens
}

func formatMetric(v float64, m metric) string {
	switch m {
	case metricTokens:
		return formatTokens(v)
	case metricSpend:
		return formatMicros(v)
	default:
		return formatCount(v)
	}
}

func formatCount(v float64) string {
	negative := v < 0
	if negative {
		v = -v
	}
	digits := strconv.FormatInt(int64(v), 10)
	var grouped strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	if negative {
		return "-" + grouped.String()
	}
	return grouped.String()
}

func formatTokens(v float64) string {
	switch {
	case v < 1000:
		return strconv.FormatInt(int64(v), 10)
	case v < 1_000_000:
		if v < 10_000 {
			return fmt.Sprintf("%.1fk", v/1000)
		}
		return fmt.Sprintf("%.0fk", v/1000)
	case v < 1_000_000_000:
		if v < 10_000_000 {
			return fmt.Sprintf("%.2fM", v/1_000_000)
		}
		return fmt.Sprintf("%.1fM", v/1_000_000)
	case v < 1e12:
		return fmt.Sprintf("%.1fB", v/1e9)
	case v < 1e15:
		return fmt.Sprintf("%.1fT", v/1e12)
	case v < 1e18:
		return fmt.Sprintf("%.1fQa", v/1e15)
	default:
		return fmt.Sprintf("%.1fQi", v/1e18)
	}
}

func formatMicros(micros float64) string {
	switch {
	case micros <= 0:
		return "$0.00"
	case micros < 100:
		return "<$0.01"
	default:
		return fmt.Sprintf("$%.2f", micros/1_000_000)
	}
}
