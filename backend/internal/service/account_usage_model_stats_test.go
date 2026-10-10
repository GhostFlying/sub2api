package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type modelWindowRepo struct {
	UsageLogRepository
	calls  atomic.Int32
	logs   []UsageLog
	fail   bool
	gate   chan struct{}
	starts []time.Time
	mu     sync.Mutex
}

func (r *modelWindowRepo) GetAccountModelWindowStats(ctx context.Context, accountID int64, start, end time.Time) ([]usagestats.ModelStat, error) {
	r.calls.Add(1)
	if r.gate != nil {
		select {
		case <-r.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts = append(r.starts, start)
	if r.fail {
		return nil, errors.New("database unavailable")
	}
	grouped := map[string]usagestats.ModelStat{}
	for _, log := range r.logs {
		if log.AccountID != accountID || log.CreatedAt.Before(start) || !log.CreatedAt.Before(end) {
			continue
		}
		name := log.Model
		if log.UpstreamModel != nil && *log.UpstreamModel != "" {
			name = *log.UpstreamModel
		}
		m := grouped[name]
		m.Model = name
		m.Requests++
		m.InputTokens += int64(log.InputTokens)
		m.OutputTokens += int64(log.OutputTokens)
		m.CacheReadTokens += int64(log.CacheReadTokens)
		m.CacheCreationTokens += int64(log.CacheCreationTokens)
		m.TotalTokens += int64(log.InputTokens + log.OutputTokens + log.CacheReadTokens + log.CacheCreationTokens)
		m.Cost += log.TotalCost
		m.ActualCost += log.ActualCost
		m.AccountCost += log.TotalCost * 2
		grouped[name] = m
	}
	result := []usagestats.ModelStat{}
	for _, m := range grouped {
		result = append(result, m)
	}
	return result, nil
}

func TestResolveAccountUsageWindow(t *testing.T) {
	now := time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)
	start, end := now.Add(-time.Hour), now.Add(4*time.Hour)
	account := &Account{Platform: PlatformAnthropic, SessionWindowStart: &start, SessionWindowEnd: &end}
	got, period := resolveAccountUsageWindow(account, "5h", nil, now)
	require.Equal(t, start, got)
	require.Equal(t, "cycle", period)
	// Reset-only persisted sample (no valid session start).
	account.SessionWindowStart = nil
	got, period = resolveAccountUsageWindow(account, "5h", nil, now)
	require.Equal(t, end.Add(-5*time.Hour), got)
	require.Equal(t, "cycle", period)
	for _, window := range []string{"5h", "7d"} {
		duration, fallback := 5*time.Hour, "last_5_hours"
		if window == "7d" {
			duration, fallback = 7*24*time.Hour, "last_7_days"
		}
		for _, delta := range []time.Duration{-time.Second, 0, duration + time.Second} {
			reset := now.Add(delta)
			a := &Account{Platform: PlatformOpenAI}
			got, period := resolveAccountUsageWindow(a, window, &UsageProgress{ResetsAt: &reset}, now)
			require.Equal(t, now.Add(-duration), got)
			require.Equal(t, fallback, period)
		}
		reset := now.Add(duration / 2)
		got, period := resolveAccountUsageWindow(&Account{Platform: PlatformOpenAI}, window, &UsageProgress{ResetsAt: &reset}, now)
		require.Equal(t, reset.Add(-duration), got)
		require.Equal(t, "cycle", period)
	}
	// An impossible future start cannot produce an empty "current cycle".
	future := now.Add(time.Hour)
	account.SessionWindowStart = &future
	account.SessionWindowEnd = nil
	got, period = resolveAccountUsageWindow(account, "5h", nil, now)
	require.Equal(t, now.Add(-5*time.Hour), got)
	require.Equal(t, "last_5_hours", period)
}

func TestAccountModelWindowStatsPassiveTotalsAndCache(t *testing.T) {
	for _, account := range []*Account{
		{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
		{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeSetupToken},
		{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: func() *int64 { id := int64(41); return &id }()},
	} {
		t.Run(account.Platform+account.Type+func() string {
			if account.ParentAccountID != nil {
				return "shadow"
			}
			return ""
		}(), func(t *testing.T) {
			now := time.Now()
			mapped := "upstream-model"
			repo := &modelWindowRepo{logs: []UsageLog{
				{AccountID: 42, CreatedAt: now.Add(-time.Minute), Model: "client-alias", UpstreamModel: &mapped, InputTokens: 10, OutputTokens: 20, CacheReadTokens: 30, CacheCreationTokens: 40, TotalCost: 2, ActualCost: 0.5},
				{AccountID: 42, CreatedAt: now.Add(-2 * time.Minute), Model: "a-model", InputTokens: 100, TotalCost: 1, ActualCost: 0.25},
				{AccountID: 43, CreatedAt: now.Add(-time.Minute), Model: "other-account", InputTokens: 999},
				{AccountID: 42, CreatedAt: now.Add(-8 * 24 * time.Hour), Model: "old", InputTokens: 999},
				{AccountID: 42, CreatedAt: now.Add(time.Hour), Model: "future", InputTokens: 999},
			}}
			// Embedded repository methods and fetcher panic if any write/probe is called.
			svc := &AccountUsageService{accountRepo: &claudeWeeklyAccountRepo{account: account}, usageLogRepo: repo, usageFetcher: claudeUnexpectedFetcher{}, cache: NewUsageCache()}
			for _, window := range []string{"5h", "7d"} {
				stats, err := svc.GetAccountModelWindowStats(context.Background(), 42, window)
				require.NoError(t, err)
				require.Len(t, stats.Models, 2)
				require.Equal(t, "a-model", stats.Models[0].Model)
				require.Equal(t, "upstream-model", stats.Models[1].Model)
				require.Equal(t, int64(200), stats.Totals.Tokens)
				require.Equal(t, int64(110), stats.Totals.InputTokens)
				require.Equal(t, int64(30), stats.Totals.CacheReadTokens)
				require.Equal(t, int64(40), stats.Totals.CacheCreationTokens)
				require.Equal(t, int64(20), stats.Totals.OutputTokens)
				require.Equal(t, 6.0, stats.Totals.Cost)
				require.Equal(t, 3.0, stats.Totals.StandardCost)
				require.Equal(t, 0.75, stats.Totals.UserCost)
				cached, err := svc.GetAccountModelWindowStats(context.Background(), 42, window)
				require.NoError(t, err)
				require.Equal(t, stats, cached)
			}
			require.Equal(t, int32(2), repo.calls.Load())
		})
	}
}

func TestAccountModelWindowStatsErrorsAndInvalidation(t *testing.T) {
	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	repo := &modelWindowRepo{fail: true}
	svc := &AccountUsageService{accountRepo: &claudeWeeklyAccountRepo{account: account}, usageLogRepo: repo, cache: NewUsageCache()}
	_, err := svc.GetAccountModelWindowStats(context.Background(), 42, "bogus")
	require.Error(t, err)
	require.Zero(t, repo.calls.Load())
	account.Type = AccountTypeAPIKey
	_, err = svc.GetAccountModelWindowStats(context.Background(), 42, "5h")
	require.Error(t, err)
	require.Zero(t, repo.calls.Load())
	account.Type = AccountTypeOAuth
	for i := 0; i < 2; i++ {
		_, err = svc.GetAccountModelWindowStats(context.Background(), 42, "7d")
		require.Error(t, err)
	}
	require.Equal(t, int32(2), repo.calls.Load())
	repo.fail = false
	empty, err := svc.GetAccountModelWindowStats(context.Background(), 42, "7d")
	require.NoError(t, err)
	require.NotNil(t, empty.Models)
	require.Empty(t, empty.Models)
	require.Zero(t, empty.Totals.Tokens)
	account.Extra = map[string]any{"passive_usage_7d_reset": time.Now().Add(time.Hour).Unix()}
	cycle, err := svc.GetAccountModelWindowStats(context.Background(), 42, "7d")
	require.NoError(t, err)
	require.Equal(t, "cycle", cycle.Period)
	account.Extra["passive_usage_7d_reset"] = time.Now().Add(2 * time.Hour).Unix()
	next, err := svc.GetAccountModelWindowStats(context.Background(), 42, "7d")
	require.NoError(t, err)
	require.NotEqual(t, cycle.StartAt, next.StartAt)
	account.Extra["passive_usage_7d_reset"] = time.Now().Add(-time.Hour).Unix()
	rolling, err := svc.GetAccountModelWindowStats(context.Background(), 42, "7d")
	require.NoError(t, err)
	require.Equal(t, "last_7_days", rolling.Period)
	entry, loaded := svc.cache.modelWindowStatsCache.Load(windowStatsCacheKey{accountID: 42, window: "7d"})
	require.True(t, loaded)
	cached, ok := entry.(*modelWindowStatsCache)
	require.True(t, ok)
	cached.timestamp = time.Now().Add(-2 * time.Minute)
	_, err = svc.GetAccountModelWindowStats(context.Background(), 42, "7d")
	require.NoError(t, err)
	require.Equal(t, int32(7), repo.calls.Load())
}

func TestAccountModelWindowStatsSingleflight(t *testing.T) {
	repo := &modelWindowRepo{gate: make(chan struct{})}
	svc := &AccountUsageService{accountRepo: &claudeWeeklyAccountRepo{account: &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}}, usageLogRepo: repo, cache: NewUsageCache()}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.GetAccountModelWindowStats(context.Background(), 42, "5h")
			require.NoError(t, err)
		}()
	}
	require.Eventually(t, func() bool { return repo.calls.Load() > 0 }, time.Second, time.Millisecond)
	close(repo.gate)
	wg.Wait()
	require.Equal(t, int32(1), repo.calls.Load())
}

func TestAccountModelWindowStatsCancelledWaiter(t *testing.T) {
	repo := &modelWindowRepo{gate: make(chan struct{})}
	svc := &AccountUsageService{accountRepo: &claudeWeeklyAccountRepo{account: &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}}, usageLogRepo: repo, cache: NewUsageCache()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := svc.GetAccountModelWindowStats(ctx, 42, "5h"); done <- err }()
	require.Eventually(t, func() bool { return repo.calls.Load() == 1 }, time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	// Shared work remains alive for other waiters and fills the successful cache.
	close(repo.gate)
	snapshot, err := svc.GetAccountModelWindowStats(context.Background(), 42, "5h")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Equal(t, int32(1), repo.calls.Load())
}

func TestResolveCodexResetWithoutPercentage(t *testing.T) {
	now := time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)
	for _, window := range []string{"5h", "7d"} {
		duration := 5 * time.Hour
		if window == "7d" {
			duration = 7 * 24 * time.Hour
		}
		reset := now.Add(time.Hour)
		account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{"codex_" + window + "_reset_at": reset.Format(time.RFC3339)}}
		start, period := resolveAccountUsageWindow(account, window, nil, now)
		require.Equal(t, "cycle", period)
		require.Equal(t, reset.Add(-duration), start)
		account.Extra = map[string]any{"codex_" + window + "_reset_after_seconds": 3600}
		_, period = resolveAccountUsageWindow(account, window, &UsageProgress{ResetsAt: &reset}, now)
		require.NotEqual(t, "cycle", period, "relative reset without a sampling timestamp is not a cycle")
		account.Extra["codex_usage_updated_at"] = now.Add(-time.Minute).Format(time.RFC3339)
		start, period = resolveAccountUsageWindow(account, window, nil, now)
		require.Equal(t, "cycle", period)
		require.Equal(t, now.Add(-time.Minute).Add(time.Hour).Add(-duration), start)
	}
}
