package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type modelStatsAccountRepo struct {
	service.AccountRepository
	account *service.Account
}

func (r *modelStatsAccountRepo) GetByID(context.Context, int64) (*service.Account, error) {
	return r.account, nil
}

type modelStatsUsageRepo struct{ service.UsageLogRepository }

func (*modelStatsUsageRepo) GetAccountModelWindowStats(context.Context, int64, time.Time, time.Time) ([]usagestats.ModelStat, error) {
	return []usagestats.ModelStat{{Model: "upstream-model", Requests: 1, InputTokens: 10, TotalTokens: 10}}, nil
}

func TestGetUsageModelStats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &service.Account{ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	svc := service.NewAccountUsageService(&modelStatsAccountRepo{account: account}, &modelStatsUsageRepo{}, nil, nil, nil, nil, nil, nil, service.NewUsageCache(), nil, nil)
	h := &AccountHandler{accountUsageService: svc}
	router := gin.New()
	router.GET("/accounts/:id/usage-model-stats", h.GetUsageModelStats)
	for _, tc := range []struct {
		url    string
		status int
	}{
		{"/accounts/no/usage-model-stats?window=5h", http.StatusBadRequest},
		{"/accounts/0/usage-model-stats?window=5h", http.StatusBadRequest},
		{"/accounts/42/usage-model-stats", http.StatusBadRequest},
		{"/accounts/42/usage-model-stats?window=1h", http.StatusBadRequest},
		{"/accounts/42/usage-model-stats?window=5h", http.StatusOK},
	} {
		t.Run(tc.url, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status == http.StatusOK {
				var response struct {
					Data service.AccountModelWindowStats `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				require.Equal(t, "upstream", response.Data.ModelSource)
				require.Equal(t, "last_5_hours", response.Data.Period)
				require.Equal(t, int64(10), response.Data.Totals.Tokens)
			}
		})
	}
	account.Type = service.AccountTypeAPIKey
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/accounts/42/usage-model-stats?window=7d", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
}
