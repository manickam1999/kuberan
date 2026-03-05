package services

import (
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

// portfolioSnapshotService handles portfolio snapshot operations.
type portfolioSnapshotService struct {
	db *gorm.DB
}

// NewPortfolioSnapshotService creates a new PortfolioSnapshotServicer.
func NewPortfolioSnapshotService(db *gorm.DB) PortfolioSnapshotServicer {
	return &portfolioSnapshotService{db: db}
}

// ComputeAndRecordSnapshots computes and stores a net worth snapshot for all active users.
func (s *portfolioSnapshotService) ComputeAndRecordSnapshots(recordedAt time.Time) (int, error) {
	// Find all distinct active user IDs
	var userIDs []string
	if err := s.db.Model(&models.Account{}).
		Where("is_active = ?", true).
		Distinct("user_id").
		Pluck("user_id", &userIDs).Error; err != nil {
		return 0, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	count := 0
	for _, userID := range userIDs {
		snapshot, err := s.computeSnapshot(userID, recordedAt)
		if err != nil {
			return count, err
		}

		// Upsert: check for existing snapshot at same user+time
		var existing models.PortfolioSnapshot
		result := s.db.Where("user_id = ? AND recorded_at = ?", userID, recordedAt).First(&existing)
		if result.Error == nil {
			// Already exists, update it
			if err := s.db.Model(&existing).Updates(map[string]interface{}{
				"total_net_worth":  snapshot.TotalNetWorth,
				"cash_balance":     snapshot.CashBalance,
				"investment_value": snapshot.InvestmentValue,
				"debt_balance":     snapshot.DebtBalance,
			}).Error; err != nil {
				return count, apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		} else {
			if err := s.db.Create(snapshot).Error; err != nil {
				return count, apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		}
		count++
	}

	return count, nil
}

// computeSnapshot calculates a user's net worth breakdown.
func (s *portfolioSnapshotService) computeSnapshot(userID string, recordedAt time.Time) (*models.PortfolioSnapshot, error) {
	// Cash balance: sum of cash account balances
	var cashBalance int64
	if err := s.db.Model(&models.Account{}).
		Where("user_id = ? AND type = ? AND is_active = ?", userID, models.AccountTypeCash, true).
		Select("COALESCE(SUM(balance), 0)").
		Scan(&cashBalance).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Investment value: sum of quantity * latest security price for all investments in active investment accounts
	var investmentValue int64
	var investments []models.Investment
	if err := s.db.Joins("JOIN accounts ON accounts.id = investments.account_id").
		Where("accounts.user_id = ? AND accounts.type = ? AND accounts.is_active = ? AND accounts.deleted_at IS NULL",
			userID, models.AccountTypeInvestment, true).
		Find(&investments).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	secIDs := make([]string, 0, len(investments))
	for i := range investments {
		secIDs = append(secIDs, investments[i].SecurityID)
	}
	prices, err := getLatestPrices(s.db, secIDs)
	if err != nil {
		return nil, err
	}
	for i := range investments {
		investmentValue += int64(investments[i].Quantity * float64(prices[investments[i].SecurityID]))
	}

	// Debt balance: sum of debt + credit_card account balances
	var debtBalance int64
	if err := s.db.Model(&models.Account{}).
		Where("user_id = ? AND type IN ? AND is_active = ?", userID, []models.AccountType{models.AccountTypeDebt, models.AccountTypeCreditCard}, true).
		Select("COALESCE(SUM(balance), 0)").
		Scan(&debtBalance).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	totalNetWorth := cashBalance + investmentValue - debtBalance

	return &models.PortfolioSnapshot{
		UserID:          userID,
		RecordedAt:      recordedAt,
		TotalNetWorth:   totalNetWorth,
		CashBalance:     cashBalance,
		InvestmentValue: investmentValue,
		DebtBalance:     debtBalance,
	}, nil
}

// GetGroupedSnapshots returns snapshots for a user within a date range, keeping
// only the last snapshot per time bucket (e.g. per day or per hour). The result
// is sorted chronologically (oldest first) and is not paginated — the caller
// should ensure the date range produces a reasonable number of buckets.
func (s *portfolioSnapshotService) GetGroupedSnapshots(
	userID string,
	from, to time.Time,
	groupBy string,
) ([]models.PortfolioSnapshot, error) {
	var snapshots []models.PortfolioSnapshot
	if err := s.db.Where("user_id = ? AND recorded_at >= ? AND recorded_at <= ?", userID, from, to).
		Order("recorded_at ASC").
		Find(&snapshots).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return groupSnapshots(snapshots, groupBy), nil
}

// groupSnapshots deduplicates snapshots by keeping the last one per time bucket.
// Since the input must be sorted by recorded_at ASC, iterating and replacing on
// the same bucket key naturally retains the latest snapshot in each bucket.
func groupSnapshots(snapshots []models.PortfolioSnapshot, groupBy string) []models.PortfolioSnapshot {
	if len(snapshots) == 0 {
		return snapshots
	}

	bucketKey := dayBucketKey
	if groupBy == "hour" {
		bucketKey = hourBucketKey
	}

	var result []models.PortfolioSnapshot
	currentKey := ""
	for i := range snapshots {
		key := bucketKey(snapshots[i].RecordedAt)
		if key != currentKey {
			currentKey = key
			result = append(result, snapshots[i])
		} else {
			// Same bucket — replace with the later snapshot.
			result[len(result)-1] = snapshots[i]
		}
	}
	return result
}

func dayBucketKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

func hourBucketKey(t time.Time) string {
	return t.UTC().Format("2006-01-02-15")
}

// GetSnapshots returns paginated snapshots for a user within a date range.
func (s *portfolioSnapshotService) GetSnapshots(
	userID string,
	from, to time.Time,
	page pagination.PageRequest,
) (*pagination.PageResponse[models.PortfolioSnapshot], error) {
	page.Defaults()

	var totalItems int64
	base := s.db.Model(&models.PortfolioSnapshot{}).
		Where("user_id = ? AND recorded_at >= ? AND recorded_at <= ?", userID, from, to)
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var snapshots []models.PortfolioSnapshot
	if err := base.Order("recorded_at DESC").Scopes(pagination.Paginate(page)).Find(&snapshots).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result := pagination.NewPageResponse(snapshots, page.Page, page.PageSize, totalItems)
	return &result, nil
}
