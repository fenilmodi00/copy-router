package admin

import (
	"net/http"
	"strconv"
	"time"

	"workweave/router/internal/auth"
	"workweave/router/internal/proxy"

	"github.com/gin-gonic/gin"
)

type metricsSummaryResponse struct {
	RequestCount          int64   `json:"request_count"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalRequestedCostUSD float64 `json:"total_requested_cost_usd"`
	TotalActualCostUSD    float64 `json:"total_actual_cost_usd"`
	TotalSavingsUSD       float64 `json:"total_savings_usd"`
	CacheWriteTokens      int64   `json:"cache_write_tokens"`
	CacheReadTokens       int64   `json:"cache_read_tokens"`
	CacheInputSavingsUSD  float64 `json:"cache_input_savings_usd"`
	SemanticCacheHits     int64   `json:"semantic_cache_hits"`
}

type timeseriesBucket struct {
	Bucket           string  `json:"bucket"`
	RequestedCostUSD float64 `json:"requested_cost_usd"`
	ActualCostUSD    float64 `json:"actual_cost_usd"`
	TotalTokens      int64   `json:"total_tokens"`
	RequestCount     int64   `json:"request_count"`
}

type metricsTimeseriesResponse struct {
	Buckets []timeseriesBucket `json:"buckets"`
}

// metricsScope resolves the installation whose telemetry the dashboard may read.
// Admin-cookie and account-cookie sessions resolve through resolveInstallation
// (admin singleton or signed-in account install); rk_ bearer auth uses the key's
// installation. Returns ok=false after writing 401/500.
func metricsScope(c *gin.Context, authSvc *auth.Service) (installationID string, ok bool) {
	installation, ok := resolveInstallation(c, authSvc)
	if !ok {
		return "", false
	}
	return installation.ID, true
}

func MetricsSummaryHandler(proxySvc *proxy.Service, authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		from, to := parseTimeWindow(c)

		installationID, ok := metricsScope(c, authSvc)
		if !ok {
			return
		}

		summary, err := proxySvc.MetricsSummary(c.Request.Context(), installationID, from, to)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch metrics."})
			return
		}

		c.JSON(http.StatusOK, metricsSummaryResponse{
			RequestCount:          summary.RequestCount,
			TotalTokens:           summary.TotalTokens,
			TotalRequestedCostUSD: summary.TotalRequestedCostUSD,
			TotalActualCostUSD:    summary.TotalActualCostUSD,
			TotalSavingsUSD:       summary.TotalSavingsUSD,
			CacheWriteTokens:      summary.CacheWriteTokens,
			CacheReadTokens:       summary.CacheReadTokens,
			CacheInputSavingsUSD:  summary.CacheInputSavingsUSD.Float64(),
			SemanticCacheHits:     summary.SemanticCacheHits,
		})
	}
}

func MetricsTimeseriesHandler(proxySvc *proxy.Service, authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		granularity := c.DefaultQuery("granularity", "hour")
		if granularity != "hour" && granularity != "day" && granularity != "week" {
			granularity = "hour"
		}
		from, to := parseTimeWindow(c)

		installationID, ok := metricsScope(c, authSvc)
		if !ok {
			return
		}

		buckets, err := proxySvc.MetricsTimeseries(c.Request.Context(), installationID, from, to, granularity)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch timeseries."})
			return
		}

		out := make([]timeseriesBucket, 0, len(buckets))
		for _, b := range buckets {
			out = append(out, timeseriesBucket{
				Bucket:           b.Bucket.UTC().Format(time.RFC3339),
				RequestedCostUSD: b.RequestedCostUSD,
				ActualCostUSD:    b.ActualCostUSD,
				TotalTokens:      b.TotalTokens,
				RequestCount:     b.RequestCount,
			})
		}
		c.JSON(http.StatusOK, metricsTimeseriesResponse{Buckets: out})
	}
}

type modelBreakdownBucket struct {
	Bucket        string  `json:"bucket"`
	DecisionModel string  `json:"decision_model"`
	RequestCount  int64   `json:"request_count"`
	TotalTokens   int64   `json:"total_tokens"`
	ActualCostUSD float64 `json:"actual_cost_usd"`
}

type metricsModelBreakdownResponse struct {
	Buckets []modelBreakdownBucket `json:"buckets"`
}

// MetricsModelBreakdownHandler serves per-bucket totals grouped by the model
// the router selected, powering the per-model usage and spend charts.
func MetricsModelBreakdownHandler(proxySvc *proxy.Service, authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		granularity := c.DefaultQuery("granularity", "hour")
		if granularity != "hour" && granularity != "day" && granularity != "week" {
			granularity = "hour"
		}
		from, to := parseTimeWindow(c)

		installationID, ok := metricsScope(c, authSvc)
		if !ok {
			return
		}

		buckets, err := proxySvc.MetricsModelBreakdown(c.Request.Context(), installationID, from, to, granularity)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch model breakdown."})
			return
		}

		out := make([]modelBreakdownBucket, 0, len(buckets))
		for _, b := range buckets {
			out = append(out, modelBreakdownBucket{
				Bucket:        b.Bucket.UTC().Format(time.RFC3339),
				DecisionModel: b.DecisionModel,
				RequestCount:  b.RequestCount,
				TotalTokens:   b.TotalTokens,
				ActualCostUSD: b.ActualCostUSD,
			})
		}
		c.JSON(http.StatusOK, metricsModelBreakdownResponse{Buckets: out})
	}
}

type metricsDetailRow struct {
	Timestamp           string  `json:"timestamp"`
	RequestID           string  `json:"request_id"`
	RequestedModel      string  `json:"requested_model"`
	DecisionModel       string  `json:"decision_model"`
	DecisionProvider    string  `json:"decision_provider"`
	DecisionReason      string  `json:"decision_reason"`
	StickyHit           bool    `json:"sticky_hit"`
	InputTokens         int32   `json:"input_tokens"`
	OutputTokens        int32   `json:"output_tokens"`
	CacheCreationTokens *int32  `json:"cache_creation_tokens"`
	CacheReadTokens     *int32  `json:"cache_read_tokens"`
	RequestedCostUSD    float64 `json:"requested_cost_usd"`
	ActualCostUSD       float64 `json:"actual_cost_usd"`
	TotalLatencyMs      int64   `json:"total_latency_ms"`
	UpstreamStatusCode  int32   `json:"upstream_status_code"`
	RouterUserID        string  `json:"router_user_id"`
	ClientApp           string  `json:"client_app"`
	TurnType            string  `json:"turn_type"`
	UserEmail           string  `json:"user_email"`
}

type metricsDetailsResponse struct {
	Rows []metricsDetailRow `json:"rows"`
}

func MetricsDetailsHandler(proxySvc *proxy.Service, authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		from, to := parseTimeWindow(c)
		const defaultLimit = 100
		const maxLimit = 1000
		limit := int32(defaultLimit)
		if raw := c.Query("limit"); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 32)
			if err == nil && n > 0 {
				if n > maxLimit {
					n = maxLimit
				}
				limit = int32(n)
			}
		}

		installationID, ok := metricsScope(c, authSvc)
		if !ok {
			return
		}

		rows, err := proxySvc.MetricsRows(c.Request.Context(), installationID, from, to, limit)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch details."})
			return
		}

		out := make([]metricsDetailRow, 0, len(rows))
		for _, r := range rows {
			out = append(out, metricsDetailRow{
				Timestamp:           r.Timestamp.UTC().Format(time.RFC3339Nano),
				RequestID:           r.RequestID,
				RequestedModel:      r.RequestedModel,
				DecisionModel:       r.DecisionModel,
				DecisionProvider:    r.DecisionProvider,
				DecisionReason:      r.DecisionReason,
				StickyHit:           r.StickyHit,
				InputTokens:         r.InputTokens,
				OutputTokens:        r.OutputTokens,
				CacheCreationTokens: r.CacheCreationTokens,
				CacheReadTokens:     r.CacheReadTokens,
				RequestedCostUSD:    r.RequestedCostUSD,
				ActualCostUSD:       r.ActualCostUSD,
				TotalLatencyMs:      r.TotalLatencyMs,
				UpstreamStatusCode:  r.UpstreamStatusCode,
				RouterUserID:        r.RouterUserID,
				ClientApp:           r.ClientApp,
				TurnType:            r.TurnType,
				UserEmail:           r.UserEmail,
			})
		}
		c.JSON(http.StatusOK, metricsDetailsResponse{Rows: out})
	}
}

func parseTimeWindow(c *gin.Context) (from, to time.Time) {
	to = time.Now().UTC()
	from = to.AddDate(0, 0, -7)

	if raw := c.Query("from"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			from = t.UTC()
		}
	}
	if raw := c.Query("to"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			to = t.UTC()
		}
	}
	return from, to
}
