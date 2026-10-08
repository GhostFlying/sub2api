package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type claudeWeeklyAccountRepo struct {
	AccountRepository
	account *Account
}

func (r *claudeWeeklyAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *claudeWeeklyAccountRepo) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	for _, id := range ids {
		if r.account != nil && r.account.ID == id {
			return []*Account{r.account}, nil
		}
	}
	return nil, nil
}

type claudeWeeklyLogRepo struct {
	UsageLogRepository
	starts     []time.Time
	accountIDs []int64
	logs       []UsageLog
	failStart  *time.Time
}

func (r *claudeWeeklyLogRepo) GetAccountWindowStats(_ context.Context, accountID int64, start time.Time) (*usagestats.AccountStats, error) {
	r.starts = append(r.starts, start)
	r.accountIDs = append(r.accountIDs, accountID)
	if r.failStart != nil && start.Equal(*r.failStart) {
		return nil, errors.New("database unavailable")
	}
	stats := &usagestats.AccountStats{}
	for _, log := range r.logs {
		if log.AccountID != accountID || log.CreatedAt.Before(start) {
			continue
		}
		stats.Requests++
		stats.Tokens += int64(log.InputTokens + log.OutputTokens + log.CacheCreationTokens + log.CacheReadTokens)
		stats.Cost += log.TotalCost
		stats.StandardCost += log.TotalCost
		stats.UserCost += log.ActualCost
	}
	return stats, nil
}

type claudeUnexpectedFetcher struct{ ClaudeUsageFetcher }

func (claudeUnexpectedFetcher) FetchUsageWithOptions(context.Context, *ClaudeUsageFetchOptions) (*ClaudeUsageResponse, error) {
	panic("passive local statistics must not fetch upstream usage")
}

func TestClaudeSevenDayStatsStart(t *testing.T) {
	now := time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		delta  time.Duration
		period string
	}{
		{"cycle", 2 * 24 * time.Hour, "cycle"},
		{"seven day boundary", 7 * 24 * time.Hour, "cycle"},
		{"expired", -time.Hour, "last_7_days"},
		{"reset now", 0, "last_7_days"},
		{"too far", 7*24*time.Hour + time.Second, "last_7_days"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset := now.Add(tc.delta)
			start, period := claudeSevenDayStatsStart(&UsageProgress{ResetsAt: &reset}, now)
			require.Equal(t, tc.period, period)
			expected := now.Add(-7 * 24 * time.Hour)
			if period == "cycle" {
				expected = reset.Add(-7 * 24 * time.Hour)
			}
			require.Equal(t, expected, start)
		})
	}
	start, period := claudeSevenDayStatsStart(nil, now)
	require.Equal(t, "last_7_days", period)
	require.Equal(t, now.Add(-7*24*time.Hour), start)
}

func TestClaudeWeeklyLogsAndCache(t *testing.T) {
	now := time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)
	reset := now.Add(2 * 24 * time.Hour)
	start := reset.Add(-7 * 24 * time.Hour)
	fiveStart, fiveEnd := now.Add(-time.Hour), now.Add(4*time.Hour)
	account := &Account{ID: 42, SessionWindowStart: &fiveStart, SessionWindowEnd: &fiveEnd}
	repo := &claudeWeeklyLogRepo{logs: []UsageLog{
		{AccountID: 42, CreatedAt: start.Add(-time.Second), InputTokens: 999},
		{AccountID: 42, CreatedAt: start, InputTokens: 10, CacheCreationTokens: 20, TotalCost: 1, ActualCost: 0.5},
		{AccountID: 42, CreatedAt: now.Add(-time.Minute), OutputTokens: 30, CacheReadTokens: 40, TotalCost: 2, ActualCost: 1},
		{AccountID: 43, CreatedAt: now, InputTokens: 999},
	}}
	svc := &AccountUsageService{usageLogRepo: repo, cache: NewUsageCache()}
	available := true
	build := func() *UsageInfo {
		return &UsageInfo{FiveHour: &UsageProgress{}, SevenDay: &UsageProgress{ResetsAt: &reset, QuotaAvailable: &available}}
	}
	usage := build()
	svc.addWindowStatsAt(t.Context(), account, usage, now)
	require.Equal(t, []time.Time{fiveStart, start}, repo.starts)
	require.Equal(t, int64(2), usage.SevenDay.WindowStats.Requests)
	require.Equal(t, int64(100), usage.SevenDay.WindowStats.Tokens)
	require.Equal(t, 3.0, usage.SevenDay.WindowStats.Cost)
	require.Equal(t, 1.5, usage.SevenDay.WindowStats.UserCost)
	require.Equal(t, int64(70), usage.FiveHour.WindowStats.Tokens)
	svc.addWindowStatsAt(t.Context(), account, build(), now.Add(30*time.Second))
	require.Len(t, repo.starts, 2)
	// Changing the weekly cycle invalidates only the weekly slot.
	reset = reset.Add(time.Hour)
	svc.addWindowStatsAt(t.Context(), account, build(), now.Add(31*time.Second))
	require.Len(t, repo.starts, 3)
	require.Equal(t, reset.Add(-7*24*time.Hour), repo.starts[2])
	// Expiration invalidates both slots.
	svc.addWindowStatsAt(t.Context(), account, build(), now.Add(91*time.Second))
	require.Len(t, repo.starts, 5)
	// Cycle-to-rolling transition invalidates immediately.
	reset = now
	svc.addWindowStatsAt(t.Context(), account, build(), now.Add(92*time.Second))
	require.Len(t, repo.starts, 6)
	require.Equal(t, now.Add(92*time.Second).Add(-7*24*time.Hour), repo.starts[5])
	svc.addWindowStatsAt(t.Context(), account, build(), now.Add(120*time.Second))
	require.Len(t, repo.starts, 6, "rolling cache must survive a moving now")
	reset = now.Add(3 * 24 * time.Hour)
	svc.addWindowStatsAt(t.Context(), account, build(), now.Add(121*time.Second))
	require.Len(t, repo.starts, 7)
	// Account IDs never share statistics.
	other := *account
	other.ID = 43
	svc.addWindowStatsAt(t.Context(), &other, build(), now.Add(122*time.Second))
	require.Len(t, repo.starts, 9)
	require.Equal(t, int64(43), repo.accountIDs[8])
}

func TestClaudeWeeklyQueryFailureIsolation(t *testing.T) {
	now := time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)
	fiveStart, fiveEnd := now.Add(-time.Hour), now.Add(4*time.Hour)
	weeklyStart := now.Add(-7 * 24 * time.Hour)
	account := &Account{ID: 42, SessionWindowStart: &fiveStart, SessionWindowEnd: &fiveEnd}
	for _, fail := range []string{"5h", "7d"} {
		t.Run(fail, func(t *testing.T) {
			failureStart := fiveStart
			if fail == "7d" {
				failureStart = weeklyStart
			}
			repo := &claudeWeeklyLogRepo{failStart: &failureStart}
			svc := &AccountUsageService{usageLogRepo: repo, cache: NewUsageCache()}
			usage := &UsageInfo{FiveHour: &UsageProgress{Utilization: 25}}
			svc.addWindowStatsAt(t.Context(), account, usage, now)
			require.Equal(t, 25.0, usage.FiveHour.Utilization)
			if fail == "5h" {
				require.Nil(t, usage.FiveHour.WindowStats)
				require.NotNil(t, usage.SevenDay.WindowStats)
			} else {
				require.Nil(t, usage.SevenDay.WindowStats)
				require.NotNil(t, usage.FiveHour.WindowStats)
			}
			repo.failStart = nil
			usage = &UsageInfo{FiveHour: &UsageProgress{}}
			svc.addWindowStatsAt(t.Context(), account, usage, now.Add(time.Second))
			require.Len(t, repo.starts, 3, "only the failed query should retry")
			require.NotNil(t, usage.FiveHour.WindowStats)
			require.NotNil(t, usage.SevenDay.WindowStats)
		})
	}
}

func TestClaudePassiveWeeklyNoUpstream(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, known := range []bool{false, true} {
			t.Run(accountType+map[bool]string{false: "/missing", true: "/zero"}[known], func(t *testing.T) {
				extra := map[string]any{}
				if known {
					extra["passive_usage_7d_utilization"] = 0.0
					extra["passive_usage_7d_reset"] = time.Now().Add(48 * time.Hour).Unix()
				}
				account := &Account{ID: 42, Platform: PlatformAnthropic, Type: accountType, Extra: extra}
				repo := &claudeWeeklyLogRepo{logs: []UsageLog{{AccountID: 42, CreatedAt: time.Now().Add(-time.Minute), InputTokens: 100}}}
				svc := &AccountUsageService{accountRepo: &claudeWeeklyAccountRepo{account: account}, usageLogRepo: repo, cache: NewUsageCache(), usageFetcher: claudeUnexpectedFetcher{}}
				usage, err := svc.GetPassiveUsage(t.Context(), 42)
				require.NoError(t, err)
				require.Equal(t, "passive", usage.Source)
				require.Equal(t, known, *usage.SevenDay.QuotaAvailable)
				require.Equal(t, int64(100), usage.SevenDay.WindowStats.Tokens)
				if known {
					require.Equal(t, "cycle", usage.SevenDay.WindowStatsPeriod)
				} else {
					require.Equal(t, "last_7_days", usage.SevenDay.WindowStatsPeriod)
				}
			})
		}
	}
}

func TestClaudeActiveWeeklyPresenceAndFallback(t *testing.T) {
	now := time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, payload, period string
		available             bool
	}{
		{"zero", `{"seven_day":{"utilization":0,"resets_at":"2030-01-12T12:00:00Z"}}`, "cycle", true},
		{"missing utilization", `{"seven_day":{"resets_at":"2030-01-12T12:00:00Z"}}`, "cycle", false},
		{"absent window", `{}`, "last_7_days", false},
		{"null utilization", `{"seven_day":{"utilization":null}}`, "last_7_days", false},
		{"missing reset", `{"seven_day":{"utilization":40}}`, "last_7_days", true},
		{"invalid reset", `{"seven_day":{"utilization":40,"resets_at":"invalid"}}`, "last_7_days", true},
		{"expired reset", `{"seven_day":{"utilization":40,"resets_at":"2030-01-09T12:00:00Z"}}`, "last_7_days", false},
		{"too far", `{"seven_day":{"utilization":40,"resets_at":"2030-02-09T12:00:00Z"}}`, "last_7_days", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resp ClaudeUsageResponse
			require.NoError(t, json.Unmarshal([]byte(tc.payload), &resp))
			svc := &AccountUsageService{usageLogRepo: &claudeWeeklyLogRepo{}, cache: NewUsageCache()}
			usage := svc.buildUsageInfo(&resp, &now)
			svc.addWindowStatsAt(t.Context(), &Account{ID: 42}, usage, now)
			require.Equal(t, tc.period, usage.SevenDay.WindowStatsPeriod)
			require.Equal(t, tc.available, *usage.SevenDay.QuotaAvailable)
			require.NotNil(t, usage.SevenDay.WindowStats)
		})
	}
}

func TestClaudeUnknownWeeklyDoesNotPersistFakeQuota(t *testing.T) {
	repo := &sessionWindowSyncRepo{}
	svc := &AccountUsageService{accountRepo: repo}
	available := false
	svc.syncActiveToPassive(t.Context(), 42, &UsageInfo{SevenDay: &UsageProgress{QuotaAvailable: &available}})
	require.Len(t, repo.extraUpdates, 1)
	require.Contains(t, repo.extraUpdates[0], "passive_usage_7d_utilization")
	require.Nil(t, repo.extraUpdates[0]["passive_usage_7d_utilization"])
	require.Nil(t, repo.extraUpdates[0]["passive_usage_7d_reset"])
}

func TestClaudeActiveWeeklyReplacesOldPassiveSample(t *testing.T) {
	now := time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, payload    string
		available, cycle bool
	}{
		{"absent weekly", `{}`, false, false},
		{"unknown utilization with cycle", `{"seven_day":{"resets_at":"2030-01-12T12:00:00Z"}}`, false, true},
		{"new quota without reset", `{"seven_day":{"utilization":40}}`, true, false},
		{"expired quota", `{"seven_day":{"utilization":40,"resets_at":"2030-01-09T12:00:00Z"}}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response ClaudeUsageResponse
			require.NoError(t, json.Unmarshal([]byte(tc.payload), &response))
			repo := &sessionWindowSyncRepo{}
			svc := &AccountUsageService{accountRepo: repo, usageLogRepo: &claudeWeeklyLogRepo{}}
			usage := svc.buildUsageInfo(&response, &now)
			svc.addWindowStatsAt(t.Context(), &Account{ID: 42}, usage, now)
			svc.syncActiveToPassive(t.Context(), 42, usage)
			require.Len(t, repo.extraUpdates, 1)
			// Simulate the repository's JSONB merge into an existing sample.
			extra := map[string]any{"passive_usage_7d_utilization": 0.9, "passive_usage_7d_reset": now.Add(time.Hour).Unix()}
			for key, value := range repo.extraUpdates[0] {
				extra[key] = value
			}
			passive := &UsageInfo{SevenDay: buildClaudePassiveSevenDay(extra)}
			svc.addWindowStatsAt(t.Context(), &Account{ID: 42}, passive, now)
			require.Equal(t, tc.available, *passive.SevenDay.QuotaAvailable)
			require.NotNil(t, passive.SevenDay.WindowStats)
			if tc.cycle {
				require.Equal(t, "cycle", passive.SevenDay.WindowStatsPeriod)
				require.Equal(t, now.Add(48*time.Hour).Unix(), extra["passive_usage_7d_reset"])
			} else {
				require.Nil(t, passive.SevenDay.ResetsAt)
				require.Equal(t, "last_7_days", passive.SevenDay.WindowStatsPeriod)
			}
			if tc.available {
				require.Equal(t, 40.0, passive.SevenDay.Utilization)
			} else {
				require.Nil(t, extra["passive_usage_7d_utilization"])
			}
		})
	}
}

func TestClaudeWeeklyBatchRemainsPassive(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run(accountType, func(t *testing.T) {
			account := &Account{ID: 42, Platform: PlatformAnthropic, Type: accountType}
			repo := &claudeWeeklyLogRepo{logs: []UsageLog{{AccountID: 42, CreatedAt: time.Now().Add(-time.Minute), InputTokens: 100}}}
			svc := &AccountUsageService{accountRepo: &claudeWeeklyAccountRepo{account: account}, usageLogRepo: repo, cache: NewUsageCache(), usageFetcher: claudeUnexpectedFetcher{}}
			usage, failures, err := svc.GetUsageBatch(t.Context(), []int64{42, 42}, true)
			require.NoError(t, err)
			require.Empty(t, failures)
			require.Len(t, usage, 1)
			require.Equal(t, "passive", usage[42].Source)
			require.False(t, *usage[42].SevenDay.QuotaAvailable)
			require.Equal(t, int64(100), usage[42].SevenDay.WindowStats.Tokens)
		})
	}
}
