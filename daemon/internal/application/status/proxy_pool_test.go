package status

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubProxies reports one fixed summary so every branch of the proxy doctor
// check can be measured without a database.
type stubProxies struct {
	summary ProxyPoolSummary
	err     error
}

func (s stubProxies) ProxyPoolSummary(context.Context) (ProxyPoolSummary, error) {
	return s.summary, s.err
}

func proxyCheck(t *testing.T, pool ProxyPool) DoctorCheck {
	t.Helper()
	return New(Options{Proxies: pool}).checkProxyPool(context.Background())
}

// TestCheckProxyPoolReportsWhatThePoolHolds covers the five answers the check
// gives: unconfigured, unreadable, empty, all disabled, partly cooling, and
// every enabled proxy cooling.
func TestCheckProxyPoolReportsWhatThePoolHolds(t *testing.T) {
	if check := proxyCheck(t, nil); check.Status != CheckWarn || !strings.Contains(check.Detail, "not configured") {
		t.Fatalf("unconfigured = %+v, want a warning", check)
	}
	broken := stubProxies{err: errors.New("the pool is unreadable")}
	if check := proxyCheck(t, broken); check.Status != CheckFail || check.Detail != "the pool is unreadable" {
		t.Fatalf("unreadable = %+v, want a failure naming the cause", check)
	}
	empty := stubProxies{}
	if check := proxyCheck(t, empty); check.Status != CheckWarn || !strings.Contains(check.Detail, "no proxies are configured") {
		t.Fatalf("empty = %+v, want a warning", check)
	}
	disabled := stubProxies{summary: ProxyPoolSummary{Total: 2}}
	if check := proxyCheck(t, disabled); check.Status != CheckWarn || !strings.Contains(check.Detail, "every proxy is disabled") {
		t.Fatalf("disabled = %+v, want a warning", check)
	}
	healthy := stubProxies{summary: ProxyPoolSummary{Total: 3, Enabled: 2}}
	if check := proxyCheck(t, healthy); check.Status != CheckPass || check.Detail != "2 of 3 enabled" {
		t.Fatalf("healthy = %+v, want a pass with the counts", check)
	}
	cooling := stubProxies{summary: ProxyPoolSummary{Total: 3, Enabled: 2, Cooling: 1}}
	if check := proxyCheck(t, cooling); check.Status != CheckPass || !strings.Contains(check.Detail, "1 cooling") {
		t.Fatalf("cooling = %+v, want a pass naming the cooling proxy", check)
	}
	allCooling := stubProxies{summary: ProxyPoolSummary{Total: 1, Enabled: 1, Cooling: 1, AllCooling: true}}
	if check := proxyCheck(t, allCooling); check.Status != CheckWarn || !strings.Contains(check.Detail, "every enabled proxy is cooling") {
		t.Fatalf("all cooling = %+v, want a warning", check)
	}
}
