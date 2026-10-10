package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// AccountModelWindowStats is a local, read-only snapshot of one account window.
type AccountModelWindowStats struct {
	Window      string                 `json:"window"`
	Period      string                 `json:"period"`
	StartAt     time.Time              `json:"start_at"`
	EndAt       time.Time              `json:"end_at"`
	ModelSource string                 `json:"model_source"`
	Models      []usagestats.ModelStat `json:"models"`
	Totals      ModelWindowTotals      `json:"totals"`
}

type ModelWindowTotals struct {
	WindowStats
	InputTokens         int64 `json:"input_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
}

type modelWindowStatsCache struct {
	snapshot  *AccountModelWindowStats
	timestamp time.Time
}

type accountModelWindowReader interface {
	GetAccountModelWindowStats(context.Context, int64, time.Time, time.Time) ([]usagestats.ModelStat, error)
}

type accountWindowRangeReader interface {
	GetAccountWindowStatsInRange(context.Context, int64, time.Time, time.Time) (*usagestats.AccountStats, error)
}

// resolveAccountUsageWindow is shared by compact totals and model details.
// progress is already persisted/sampled quota state; resolving it never probes.
func resolveAccountUsageWindow(account *Account, window string, progress *UsageProgress, now time.Time) (time.Time, string) {
	duration, fallback := 5*time.Hour, "last_5_hours"
	if window == "7d" {
		duration, fallback = 7*24*time.Hour, "last_7_days"
	}
	if window == "5h" && account != nil && account.Platform != PlatformOpenAI {
		start, end := account.SessionWindowStart, account.SessionWindowEnd
		if start != nil && end != nil && !start.After(now) && now.Before(*end) && end.After(*start) && end.Sub(*start) <= duration && !end.After(now.Add(duration)) {
			return *start, "cycle"
		}
		// The displayed Claude progress can contain a predicted reset; only use
		// a persisted passive sample when no valid session boundary is available.
		progress = &UsageProgress{ResetsAt: account.SessionWindowEnd}
	}
	if account != nil && account.Platform == PlatformOpenAI {
		// A known boundary is useful even when no official percentage exists.
		// Relative reset headers require their persisted sampling timestamp;
		// anchoring them to every page read would slide the cycle indefinitely.
		resetAt, hasReset := account.Extra["codex_"+window+"_reset_at"]
		resetAfter, hasRelative := account.Extra["codex_"+window+"_reset_after_seconds"]
		if hasReset || hasRelative {
			progress = nil
			if reset, err := parseTime(fmt.Sprint(resetAt)); hasReset && err == nil {
				progress = &UsageProgress{ResetsAt: &reset}
			} else if sampledAt, err := parseTime(fmt.Sprint(account.Extra["codex_usage_updated_at"])); err == nil && !sampledAt.After(now) {
				if seconds := parseExtraInt(resetAfter); seconds > 0 && seconds <= int(duration/time.Second) {
					reset := sampledAt.Add(time.Duration(seconds) * time.Second)
					progress = &UsageProgress{ResetsAt: &reset}
				}
			}
		}
	}
	return usageWindowStart(progress, duration, fallback, now)
}

func usageWindowStart(progress *UsageProgress, duration time.Duration, fallback string, now time.Time) (time.Time, string) {
	if progress != nil && progress.ResetsAt != nil && now.Before(*progress.ResetsAt) && !progress.ResetsAt.After(now.Add(duration)) {
		return progress.ResetsAt.Add(-duration), "cycle"
	}
	return now.Add(-duration), fallback
}

func localUsageProgress(account *Account, now time.Time) *UsageInfo {
	if account.IsOpenAIOAuth() {
		usage := &UsageInfo{}
		applyExtraToUsage(usage, account.Extra, now)
		return usage
	}
	return &UsageInfo{SevenDay: buildClaudePassiveSevenDay(account.Extra)}
}

// GetAccountModelWindowStats reads only account state and retained usage logs.
func (s *AccountUsageService) GetAccountModelWindowStats(ctx context.Context, accountID int64, window string) (*AccountModelWindowStats, error) {
	if window != "5h" && window != "7d" {
		return nil, infraerrors.BadRequest("INVALID_USAGE_WINDOW", "window must be 5h or 7d")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if !supportsAnthropicPassiveUsage(account) && !account.IsOpenAIOAuth() {
		return nil, infraerrors.BadRequest("UNSUPPORTED_USAGE_ACCOUNT", "model window statistics require Claude OAuth/Setup Token or Codex OAuth")
	}
	now := time.Now().UTC()
	usage := localUsageProgress(account, now)
	progress := usage.FiveHour
	if window == "7d" {
		progress = usage.SevenDay
	}
	start, period := resolveAccountUsageWindow(account, window, progress, now)
	key := windowStatsCacheKey{accountID: accountID, window: window}
	cached := func() *AccountModelWindowStats {
		if s.cache == nil {
			return nil
		}
		if value, ok := s.cache.modelWindowStatsCache.Load(key); ok {
			entry, valid := value.(*modelWindowStatsCache)
			if !valid || entry == nil || entry.snapshot == nil {
				return nil
			}
			if now.Sub(entry.timestamp) < windowStatsCacheTTL && entry.snapshot.Period == period && (period != "cycle" || entry.snapshot.StartAt.Equal(start)) {
				return entry.snapshot
			}
		}
		return nil
	}
	load := func(queryCtx context.Context) (*AccountModelWindowStats, error) {
		if snapshot := cached(); snapshot != nil {
			return snapshot, nil
		}
		reader, ok := s.usageLogRepo.(accountModelWindowReader)
		if !ok {
			return nil, fmt.Errorf("model window statistics repository unavailable")
		}
		models, err := reader.GetAccountModelWindowStats(queryCtx, accountID, start, now)
		if err != nil {
			return nil, err
		}
		if models == nil {
			models = []usagestats.ModelStat{}
		}
		sort.Slice(models, func(i, j int) bool {
			if models[i].TotalTokens == models[j].TotalTokens {
				return models[i].Model < models[j].Model
			}
			return models[i].TotalTokens > models[j].TotalTokens
		})
		snapshot := &AccountModelWindowStats{Window: window, Period: period, StartAt: start, EndAt: now, ModelSource: usagestats.ModelSourceUpstream, Models: models}
		for _, model := range models {
			snapshot.Totals.Requests += model.Requests
			snapshot.Totals.InputTokens += model.InputTokens
			snapshot.Totals.CacheReadTokens += model.CacheReadTokens
			snapshot.Totals.CacheCreationTokens += model.CacheCreationTokens
			snapshot.Totals.OutputTokens += model.OutputTokens
			snapshot.Totals.Tokens += model.TotalTokens
			snapshot.Totals.Cost += model.AccountCost
			snapshot.Totals.StandardCost += model.Cost
			snapshot.Totals.UserCost += model.ActualCost
		}
		snapshot.Totals.TotalTokens = snapshot.Totals.Tokens
		if s.cache != nil {
			s.cache.modelWindowStatsCache.Store(key, &modelWindowStatsCache{snapshot: snapshot, timestamp: now})
			// Keep subsequent compact reads aligned with the grouped snapshot.
			totals := snapshot.Totals.WindowStats
			s.cache.windowStatsCache.Store(key, &windowStatsCache{stats: &totals, timestamp: now, period: period, startTime: start})
		}
		return snapshot, nil
	}
	if snapshot := cached(); snapshot != nil {
		return snapshot, nil
	}
	if s.cache == nil {
		return load(ctx)
	}
	flightKey := fmt.Sprintf("%d:%s:%s", accountID, window, period)
	if period == "cycle" {
		flightKey += ":" + start.Format(time.RFC3339Nano)
	}
	result := s.cache.modelWindowFlight.DoChan(flightKey, func() (any, error) {
		queryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return load(queryCtx)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-result:
		if result.Err != nil {
			return nil, result.Err
		}
		snapshot, ok := result.Val.(*AccountModelWindowStats)
		if !ok || snapshot == nil {
			return nil, fmt.Errorf("invalid model window statistics result")
		}
		return snapshot, nil
	}
}
