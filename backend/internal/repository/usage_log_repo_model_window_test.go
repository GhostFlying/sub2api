package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestAccountModelWindowSQL(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	start := time.Now().Add(-5 * time.Hour)
	end := time.Now()
	modelExpr := "COALESCE(NULLIF(TRIM(upstream_model), ''), model)"
	mock.ExpectQuery(`(?s)SELECT\s+`+regexp.QuoteMeta(modelExpr)+` as model,.*SUM\(input_tokens\).*SUM\(output_tokens\).*SUM\(cache_creation_tokens\).*SUM\(cache_read_tokens\).*SUM\(actual_cost\).*WHERE created_at >= \$1 AND created_at < \$2.*AND account_id = \$3.*GROUP BY `+regexp.QuoteMeta(modelExpr)).WithArgs(start, end, int64(42)).WillReturnRows(sqlmock.NewRows([]string{"model", "requests", "input", "output", "write", "read", "total", "cost", "user_cost", "account_cost"}).AddRow("gpt-model", 2, 10, 20, 30, 40, 100, 2.0, 0.5, 4.0))
	stats, err := repo.GetAccountModelWindowStats(context.Background(), 42, start, end)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, int64(100), stats[0].TotalTokens)
	require.Equal(t, 0.5, stats[0].ActualCost)
	require.Equal(t, 4.0, stats[0].AccountCost)
	mock.ExpectQuery(`(?s)WHERE account_id = \$1 AND created_at >= \$2.*AND created_at < \$3`).WithArgs(int64(42), start, end).WillReturnRows(sqlmock.NewRows([]string{"requests", "tokens", "cost", "standard_cost", "user_cost"}).AddRow(2, 100, 4.0, 2.0, 0.5))
	totals, err := repo.GetAccountWindowStatsInRange(context.Background(), 42, start, end)
	require.NoError(t, err)
	require.Equal(t, int64(100), totals.Tokens)
	require.NoError(t, mock.ExpectationsWereMet())
}
