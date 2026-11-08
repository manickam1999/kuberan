package services

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/logger"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/uuid"
)

// securityService handles security-related business logic.
type securityService struct {
	db *gorm.DB
}

// NewSecurityService creates a new SecurityServicer.
func NewSecurityService(db *gorm.DB) SecurityServicer {
	return &securityService{db: db}
}

// CreateSecurity creates a new security record.
func (s *securityService) CreateSecurity(
	symbol, name string,
	assetType models.AssetType,
	currency, exchange string,
	extraFields map[string]interface{},
) (*models.Security, error) {
	if strings.TrimSpace(symbol) == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "Symbol is required")
	}
	if strings.TrimSpace(name) == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "Name is required")
	}
	if currency == "" {
		currency = "USD"
	}

	security := &models.Security{
		Symbol:    symbol,
		Name:      name,
		AssetType: assetType,
		Currency:  currency,
		Exchange:  exchange,
	}

	applySecurityExtraFields(security, extraFields)

	if err := s.db.Create(security).Error; err != nil {
		if isUniqueConstraintError(err) {
			return nil, apperrors.ErrDuplicateSecurity
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return security, nil
}

// GetSecurityByID returns a security by its ID.
func (s *securityService) GetSecurityByID(id string) (*models.Security, error) {
	var security models.Security
	if err := s.db.Where("id = ?", id).First(&security).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrSecurityNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &security, nil
}

// ListSecurities returns a paginated list of securities ordered by symbol.
// When search is non-empty, results are filtered by case-insensitive match on symbol or name.
func (s *securityService) ListSecurities(search string, page pagination.PageRequest) (*pagination.PageResponse[models.Security], error) {
	page.Defaults()

	var totalItems int64
	base := s.db.Model(&models.Security{})

	if search = strings.TrimSpace(search); search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		base = base.Where("LOWER(symbol) LIKE ? OR LOWER(name) LIKE ?", pattern, pattern)
	}

	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var securities []models.Security
	if err := base.Order("symbol ASC").Scopes(pagination.Paginate(page)).Find(&securities).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result := pagination.NewPageResponse(securities, page.Page, page.PageSize, totalItems)
	return &result, nil
}

// priceInsertBatchSize bounds the rows per multi-row price INSERT (4 bind
// parameters per row keeps each statement well under driver limits).
const priceInsertBatchSize = 1000

// RecordPrices bulk-inserts price entries, skipping duplicates (an existing
// row, or an earlier entry in the same request, with the same security_id and
// recorded_at). It loads the referenced securities in one query and rejects
// entries for an unknown security or whose currency differs from the
// security's; an entry without a currency is stored in the security's own
// currency. The accepted rows are inserted with batched INSERT ... ON CONFLICT
// DO NOTHING statements in one DB transaction. The result carries the number
// of rows actually inserted and the rejected entries (never nil).
func (s *securityService) RecordPrices(prices []SecurityPriceInput) (*RecordPricesResult, error) {
	if len(prices) == 0 {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "Prices array is empty")
	}

	ids := make([]string, 0, len(prices))
	for _, p := range prices {
		if uuid.IsValid(p.SecurityID) {
			ids = append(ids, p.SecurityID)
		}
	}
	currencyByID := map[string]string{}
	if ids = uniqueStrings(ids); len(ids) > 0 {
		var secs []struct {
			ID       string
			Currency string
		}
		if err := s.db.Table("securities").Select("id, currency").
			Where("id IN ? AND deleted_at IS NULL", ids).Scan(&secs).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		for _, sec := range secs {
			currencyByID[sec.ID] = normalizeCode(sec.Currency)
		}
	}

	result := &RecordPricesResult{Rejected: []RejectedPrice{}}
	rows := make([]models.SecurityPrice, 0, len(prices))
	missingCurrency := 0
	for i, p := range prices {
		expected, known := currencyByID[p.SecurityID]
		if !known {
			result.Rejected = append(result.Rejected, RejectedPrice{
				Index: i, SecurityID: p.SecurityID, Currency: normalizeCode(p.Currency),
				Reason: PriceRejectedUnknownSecurity,
			})
			continue
		}
		given := normalizeCode(p.Currency)
		switch {
		case given == "":
			missingCurrency++
		case given != expected:
			result.Rejected = append(result.Rejected, RejectedPrice{
				Index: i, SecurityID: p.SecurityID, Currency: given, ExpectedCurrency: expected,
				Reason: PriceRejectedCurrencyMismatch,
			})
			continue
		}
		rows = append(rows, models.SecurityPrice{
			SecurityID: p.SecurityID,
			Price:      p.Price,
			Currency:   expected,
			RecordedAt: p.RecordedAt,
		})
	}
	if missingCurrency > 0 {
		logger.Get().Warnw("price entries without currency stored in the security currency",
			"entries_without_currency", missingCurrency, "entries", len(prices))
	}
	if len(rows) == 0 {
		return result, nil
	}

	var inserted int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(rows); start += priceInsertBatchSize {
			batch := rows[start:min(start+priceInsertBatchSize, len(rows))]
			res := tx.Omit(clause.Associations).
				Clauses(clause.OnConflict{DoNothing: true}).
				Create(&batch)
			if res.Error != nil {
				return res.Error
			}
			inserted += res.RowsAffected
		}
		return nil
	})
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result.Recorded = int(inserted)
	return result, nil
}

// UpdateSecurity corrects a security's currency, exchange, provider symbol or
// name. Changing the currency of a security that has prices requires
// ResetPrices, which deletes them in the same transaction; ResetPrices without
// a currency change is rejected because it would only destroy data. An
// exchange change that collides with the unique (symbol, exchange) index
// returns ErrDuplicateSecurity.
func (s *securityService) UpdateSecurity(id string, update SecurityUpdate) (*SecurityUpdateResult, error) {
	if update.Currency == nil && update.Exchange == nil && update.ProviderSymbol == nil &&
		update.Name == nil && !update.ResetPrices {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "At least one field is required")
	}
	if update.Name != nil && strings.TrimSpace(*update.Name) == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "Name cannot be empty")
	}

	var result SecurityUpdateResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var sec models.Security
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&sec).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.ErrSecurityNotFound
			}
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}

		changes := map[string]interface{}{}
		logFields := map[string]interface{}{}
		currencyChanged := false
		if update.Currency != nil {
			newCur := normalizeCode(*update.Currency)
			if newCur != normalizeCode(sec.Currency) {
				currencyChanged = true
				changes["currency"] = newCur
				logFields["currency"] = sec.Currency + "->" + newCur
			}
		}
		if update.ResetPrices && !currencyChanged {
			return apperrors.WithMessage(apperrors.ErrInvalidInput,
				"reset_prices requires a currency change")
		}
		if update.Exchange != nil {
			if v := strings.TrimSpace(*update.Exchange); v != sec.Exchange {
				changes["exchange"] = v
				logFields["exchange"] = sec.Exchange + "->" + v
			}
		}
		if update.ProviderSymbol != nil {
			if v := strings.TrimSpace(*update.ProviderSymbol); v != sec.ProviderSymbol {
				changes["provider_symbol"] = v
				logFields["provider_symbol"] = sec.ProviderSymbol + "->" + v
			}
		}
		if update.Name != nil {
			if v := strings.TrimSpace(*update.Name); v != sec.Name {
				changes["name"] = v
				logFields["name"] = sec.Name + "->" + v
			}
		}

		if currencyChanged {
			var priceCount int64
			if err := tx.Model(&models.SecurityPrice{}).Where("security_id = ?", id).Count(&priceCount).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
			if priceCount > 0 && !update.ResetPrices {
				return apperrors.ErrSecurityCurrencyLocked
			}
		}

		if len(changes) > 0 {
			if err := tx.Model(&models.Security{}).Where("id = ?", id).Updates(changes).Error; err != nil {
				if isUniqueConstraintError(err) {
					return apperrors.ErrDuplicateSecurity
				}
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		}
		if update.ResetPrices {
			del := tx.Where("security_id = ?", id).Delete(&models.SecurityPrice{})
			if del.Error != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, del.Error)
			}
			result.PricesDeleted = del.RowsAffected
		}

		if err := tx.Where("id = ?", id).First(&sec).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		result.Security = &sec
		logger.Get().Infow("security updated",
			"security_id", id, "symbol", sec.Symbol, "changes", logFields,
			"prices_deleted", result.PricesDeleted)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPriceHistory returns paginated price history for a security within a date range.
func (s *securityService) GetPriceHistory(
	securityID string,
	from, to time.Time,
	page pagination.PageRequest,
) (*pagination.PageResponse[models.SecurityPrice], error) {
	page.Defaults()

	var totalItems int64
	base := s.db.Model(&models.SecurityPrice{}).
		Where("security_id = ? AND recorded_at >= ? AND recorded_at <= ?", securityID, from, to)
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var prices []models.SecurityPrice
	if err := base.Order("recorded_at DESC").Scopes(pagination.Paginate(page)).Find(&prices).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result := pagination.NewPageResponse(prices, page.Page, page.PageSize, totalItems)
	return &result, nil
}

// ListAllSecurities returns all active securities ordered by symbol.
// Intended for machine clients (e.g., the price oracle) that need the full list.
func (s *securityService) ListAllSecurities() ([]models.Security, error) {
	var securities []models.Security
	if err := s.db.Order("symbol ASC").Find(&securities).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return securities, nil
}

// applySecurityExtraFields sets asset-type-specific fields on a security from a map.
func applySecurityExtraFields(sec *models.Security, fields map[string]interface{}) {
	if fields == nil {
		return
	}
	if v, ok := fields["maturity_date"].(*time.Time); ok {
		sec.MaturityDate = v
	}
	if v, ok := fields["yield_to_maturity"].(float64); ok {
		sec.YieldToMaturity = v
	}
	if v, ok := fields["coupon_rate"].(float64); ok {
		sec.CouponRate = v
	}
	if v, ok := fields["network"].(string); ok {
		sec.Network = v
	}
	if v, ok := fields["property_type"].(string); ok {
		sec.PropertyType = v
	}
	if v, ok := fields["provider_symbol"].(string); ok {
		sec.ProviderSymbol = v
	}
}

// isUniqueConstraintError checks if a GORM error is a unique constraint violation.
func isUniqueConstraintError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || // SQLite
		strings.Contains(msg, "duplicate key value violates unique constraint") // PostgreSQL
}
