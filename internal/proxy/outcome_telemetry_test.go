package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/proxy"
	"weave-os/router/internal/proxy/usage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBypassTelemetryRecordsOutcomeAndIncomingTools(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rejected"}[rejected], func(t *testing.T) {
			telem := newCaptureTelemetry()
			provider := &fakeProvider{}
			if rejected {
				provider.proxyErr = &providers.UpstreamErrorResponse{Status: http.StatusBadRequest, Body: []byte(`{"error":{"message":"bad request"}}`)}
			}
			observer := usage.NewObserver([]byte("salt"), time.Minute, time.Now)
			svc := proxy.NewService(&fakeRouter{}, map[string]providers.Client{providers.ProviderAnthropic: provider}, nil, false, nil, nil, false, providers.ProviderAnthropic, bypassRequestedMdl, telem).WithUsageObserver(observer)
			ctx := context.WithValue(bypassCtx(0.8), proxy.InstallationIDContextKey{}, "55555555-5555-5555-5555-555555555555")
			body := []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"check"},{"role":"assistant","content":[{"type":"tool_use","id":"call-1","name":"Bash","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","is_error":true,"content":"failed"}]}]}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
			require.NoError(t, svc.ProxyMessages(ctx, body, httptest.NewRecorder(), req))
			row := telem.firstRow(t)
			require.NotNil(t, row.UserPrompt)
			assert.False(t, *row.UserPrompt)
			require.JSONEq(t, `{"Bash":{"calls":1,"errors":1}}`, string(row.LatestToolCallCounts))
			if rejected {
				assert.Equal(t, proxy.TurnErrorInvalidRequest, row.ErrorClass)
				assert.Equal(t, int32(http.StatusBadRequest), row.UpstreamStatusCode)
			} else {
				assert.Empty(t, row.ErrorClass)
			}
		})
	}
}
