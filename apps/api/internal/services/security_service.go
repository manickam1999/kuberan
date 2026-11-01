package services

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
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
// recorded_at). It runs batched INSERT ... ON CONFLICT DO NOTHING statements in
// one DB transaction and returns the number of rows actually inserted.
func (s *securityService) RecordPrices(prices []SecurityPriceInput) (int, error) {
	if len(prices) == 0 {
		return 0, apperrors.WithMessage(apperrors.ErrInvalidInput, "Prices array is empty")
	}

	rows := make([]models.SecurityPrice, len(prices))
	for i, p := range prices {
		rows[i] = models.SecurityPrice{
			SecurityID: p.SecurityID,
			Price:      p.Price,
			RecordedAt: p.RecordedAt,
		}
	}

	var inserted int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(rows); start += priceInsertBatchSize {
			batch := rows[start:min(start+priceInsertBatchSize, len(rows))]
			result := tx.Omit(clause.Associations).
				Clauses(clause.OnConflict{DoNothing: true}).
				Create(&batch)
			if result.Error != nil {
				return result.Error
			}
			inserted += result.RowsAffected
		}
		return nil
	})
	if err != nil {
		return 0, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return int(inserted), nil
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
