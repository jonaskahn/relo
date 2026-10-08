package quota_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
)

type balanceConstructor func(quota.ProberOptions) activity.QuotaProber

func TestBalanceProbers(t *testing.T) {
	tests := []struct {
		name       string
		newProber  balanceConstructor
		responses  map[string]string
		baseURL    string
		amounts    []float64
		currencies []string
	}{
		{
			name: "DeepSeek", newProber: func(options quota.ProberOptions) activity.QuotaProber {
				return quota.NewDeepSeekProber(options)
			},
			responses: map[string]string{"/probe": `{"balance_infos":[{"currency":"USD","total_balance":"1.25"},{"currency":"CNY","total_balance":"8.50"}]}`},
			amounts:   []float64{1.25, 8.5}, currencies: []string{"USD", "CNY"},
		},
		{
			name: "TeamoRouter", newProber: func(options quota.ProberOptions) activity.QuotaProber {
				return quota.NewTeamoRouterProber(options)
			},
			responses: map[string]string{"/probe": `{"balance":{"value":"50.09","currency":"USD"}}`},
			amounts:   []float64{50.09}, currencies: []string{"USD"},
		},
		{
			name: "Vercel", newProber: func(options quota.ProberOptions) activity.QuotaProber {
				return quota.NewVercelGatewayProber(options)
			},
			responses: map[string]string{"/probe": `{"balance":"95.50","total_used":"4.50"}`},
			amounts:   []float64{95.5, 4.5}, currencies: []string{"USD", "USD"},
		},
		{
			name: "OrcaRouter", newProber: func(options quota.ProberOptions) activity.QuotaProber {
				return quota.NewOrcaRouterProber(options)
			},
			responses: map[string]string{
				"/v1/dashboard/billing/usage":        `{"total_usage":275}`,
				"/v1/dashboard/billing/subscription": `{"hard_limit_usd":1000}`,
			},
			amounts: []float64{2.75, 7.25}, currencies: []string{"USD", "USD"},
		},
		{
			name: "Moonshot", newProber: func(options quota.ProberOptions) activity.QuotaProber {
				return quota.NewMoonshotProber(options)
			},
			responses: map[string]string{"/probe": `{"code":0,"data":{"available_balance":49.58894},"status":true}`},
			baseURL:   "https://api.moonshot.cn/v1", amounts: []float64{49.58894}, currencies: []string{"CNY"},
		},
		{
			name: "SiliconFlow", newProber: func(options quota.ProberOptions) activity.QuotaProber {
				return quota.NewSiliconFlowProber(options)
			},
			responses: map[string]string{"/probe": `{"code":20000,"status":true,"data":{"balance":"0.88"}}`},
			baseURL:   "https://api.siliconflow.com/v1", amounts: []float64{0.88}, currencies: []string{"USD"},
		},
		{
			name: "Novita", newProber: func(options quota.ProberOptions) activity.QuotaProber {
				return quota.NewNovitaProber(options)
			},
			responses: map[string]string{"/probe": `{"availableBalance":"1000000"}`},
			amounts:   []float64{100}, currencies: []string{"USD"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := balanceServer(t, test.responses, http.StatusOK)
			endpoint := server.URL + "/probe"
			if test.name == "OrcaRouter" {
				endpoint = server.URL
			}
			prober := test.newProber(quota.ProberOptions{Endpoint: endpoint})
			samples, err := prober.Probe(context.Background(), activity.Credential{
				ID: "key-1", AccessToken: "secret", BaseURL: test.baseURL,
			})
			if err != nil {
				t.Fatalf("Probe() error = %v", err)
			}
			if len(samples) != len(test.amounts) {
				t.Fatalf("samples = %+v, want %d readings", samples, len(test.amounts))
			}
			for index, sample := range samples {
				if sample.Amount == nil || *sample.Amount != test.amounts[index] || sample.Currency != test.currencies[index] {
					t.Fatalf("sample %d = %+v, want %v %s", index, sample, test.amounts[index], test.currencies[index])
				}
			}
		})
	}
}

func TestBalanceProbersRejectAndValidateResponses(t *testing.T) {
	constructors := map[string]balanceConstructor{
		"DeepSeek": func(options quota.ProberOptions) activity.QuotaProber { return quota.NewDeepSeekProber(options) },
		"TeamoRouter": func(options quota.ProberOptions) activity.QuotaProber {
			return quota.NewTeamoRouterProber(options)
		},
		"Vercel":     func(options quota.ProberOptions) activity.QuotaProber { return quota.NewVercelGatewayProber(options) },
		"OrcaRouter": func(options quota.ProberOptions) activity.QuotaProber { return quota.NewOrcaRouterProber(options) },
		"Moonshot":   func(options quota.ProberOptions) activity.QuotaProber { return quota.NewMoonshotProber(options) },
		"SiliconFlow": func(options quota.ProberOptions) activity.QuotaProber {
			return quota.NewSiliconFlowProber(options)
		},
		"Novita": func(options quota.ProberOptions) activity.QuotaProber { return quota.NewNovitaProber(options) },
	}
	for name, constructor := range constructors {
		t.Run(name+" rejects the key", func(t *testing.T) {
			responses := map[string]string{"/probe": `{}`}
			if name == "OrcaRouter" {
				responses = map[string]string{"/v1/dashboard/billing/usage": `{}`}
			}
			server := balanceServer(t, responses, http.StatusUnauthorized)
			endpoint := server.URL + "/probe"
			if name == "OrcaRouter" {
				endpoint = server.URL
			}
			_, err := constructor(quota.ProberOptions{Endpoint: endpoint}).Probe(
				context.Background(), activity.Credential{ID: "key-1", AccessToken: "bad"},
			)
			if !errors.Is(err, activity.ErrProbeRejected) {
				t.Fatalf("Probe() error = %v, want ErrProbeRejected", err)
			}
		})
		t.Run(name+" validates the response", func(t *testing.T) {
			responses := map[string]string{"/probe": `{}`}
			endpointPath := "/probe"
			if name == "OrcaRouter" {
				responses = map[string]string{"/v1/dashboard/billing/usage": `{}`}
				endpointPath = ""
			}
			server := balanceServer(t, responses, http.StatusOK)
			_, err := constructor(quota.ProberOptions{Endpoint: server.URL + endpointPath}).Probe(
				context.Background(), activity.Credential{ID: "key-1", AccessToken: "secret"},
			)
			if !errors.Is(err, activity.ErrProbeResponse) {
				t.Fatalf("Probe() error = %v, want ErrProbeResponse", err)
			}
		})
	}
}

func balanceServer(t *testing.T, responses map[string]string, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" && status == http.StatusOK {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		body, found := responses[request.URL.Path]
		if !found {
			http.NotFound(w, request)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}
