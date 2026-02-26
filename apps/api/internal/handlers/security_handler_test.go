package handlers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// --- mock security service ---

type mockSecurityService struct {
	createSecurityFn    func(symbol, name string, assetType models.AssetType, currency, exchange string, extraFields map[string]interface{}) (*models.Security, error)
	getSecurityByIDFn   func(id string) (*models.Security, error)
	listSecuritiesFn    func(search string, page pagination.PageRequest) (*pagination.PageResponse[models.Security], error)
	listAllSecuritiesFn func() ([]models.Security, error)
	recordPricesFn      func(prices []services.SecurityPriceInput) (int, error)
	getPriceHistoryFn   func(securityID string, from, to time.Time, page pagination.PageRequest) (*pagination.PageResponse[models.SecurityPrice], error)
}

var _ services.SecurityServicer = (*mockSecurityService)(nil)

func (m *mockSecurityService) CreateSecurity(symbol, name string, assetType models.AssetType, currency, exchange string, extraFields map[string]interface{}) (*models.Security, error) {
	if m.createSecurityFn != nil {
		return m.createSecurityFn(symbol, name, assetType, currency, exchange, extraFields)
	}
	return &models.Security{}, nil
}

func (m *mockSecurityService) GetSecurityByID(id string) (*models.Security, error) {
	if m.getSecurityByIDFn != nil {
		return m.getSecurityByIDFn(id)
	}
	return &models.Security{}, nil
}

func (m *mockSecurityService) ListAllSecurities() ([]models.Security, error) {
	if m.listAllSecuritiesFn != nil {
		return m.listAllSecuritiesFn()
	}
	return []models.Security{}, nil
}

func (m *mockSecurityService) ListSecurities(search string, page pagination.PageRequest) (*pagination.PageResponse[models.Security], error) {
	if m.listSecuritiesFn != nil {
		return m.listSecuritiesFn(search, page)
	}
	resp := pagination.NewPageResponse([]models.Security{}, 1, 20, 0)
	return &resp, nil
}

func (m *mockSecurityService) RecordPrices(prices []services.SecurityPriceInput) (int, error) {
	if m.recordPricesFn != nil {
		return m.recordPricesFn(prices)
	}
	return 0, nil
}

func (m *mockSecurityService) GetPriceHistory(securityID string, from, to time.Time, page pagination.PageRequest) (*pagination.PageResponse[models.SecurityPrice], error) {
	if m.getPriceHistoryFn != nil {
		return m.getPriceHistoryFn(securityID, from, to, page)
	}
	resp := pagination.NewPageResponse([]models.SecurityPrice{}, 1, 20, 0)
	return &resp, nil
}

// --- router setup ---

func setupSecurityRouter(handler *SecurityHandler) *gin.Engine {
	r := gin.New()
	// Pipeline routes (no auth needed for handler tests)
	r.GET("/pipeline/securities", handler.ListAllSecurities)
	r.POST("/pipeline/securities", handler.CreateSecurity)
	r.POST("/pipeline/securities/prices", handler.RecordPrices)
	// User routes (with auth)
	auth := r.Group("", injectUserID("test-user-1"))
	auth.GET("/securities", handler.ListSecurities)
	auth.GET("/securities/:id", handler.GetSecurity)
	auth.GET("/securities/:id/prices", handler.GetPriceHistory)
	return r
}

// --- tests ---

func TestSecurityHandler_CreateSecurity(t *testing.T) {
	t.Run("returns_201_on_success", func(t *testing.T) {
		svc := &mockSecurityService{
			createSecurityFn: func(symbol, name string, assetType models.AssetType, currency, exchange string, _ map[string]interface{}) (*models.Security, error) {
				return &models.Security{
					Base:      models.Base{ID: "sec-1"},
					Symbol:    symbol,
					Name:      name,
					AssetType: assetType,
					Currency:  currency,
					Exchange:  exchange,
				}, nil
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/securities",
			`{"symbol":"AAPL","name":"Apple Inc.","asset_type":"stock","currency":"USD","exchange":"NASDAQ"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		sec := result["security"].(map[string]interface{})
		if sec["symbol"] != "AAPL" {
			t.Errorf("expected symbol=AAPL, got %v", sec["symbol"])
		}
		if sec["asset_type"] != "stock" {
			t.Errorf("expected asset_type=stock, got %v", sec["asset_type"])
		}
	})

	t.Run("returns_400_missing_symbol", func(t *testing.T) {
		handler := NewSecurityHandler(&mockSecurityService{}, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/securities",
			`{"name":"Apple Inc.","asset_type":"stock"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_400_missing_name", func(t *testing.T) {
		handler := NewSecurityHandler(&mockSecurityService{}, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/securities",
			`{"symbol":"AAPL","asset_type":"stock"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_400_invalid_asset_type", func(t *testing.T) {
		handler := NewSecurityHandler(&mockSecurityService{}, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/securities",
			`{"symbol":"AAPL","name":"Apple Inc.","asset_type":"invalid"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_201_with_provider_symbol", func(t *testing.T) {
		var capturedExtraFields map[string]interface{}
		svc := &mockSecurityService{
			createSecurityFn: func(symbol, name string, assetType models.AssetType, currency, exchange string, extraFields map[string]interface{}) (*models.Security, error) {
				capturedExtraFields = extraFields
				return &models.Security{
					Base:           models.Base{ID: "sec-1"},
					Symbol:         symbol,
					Name:           name,
					AssetType:      assetType,
					Currency:       currency,
					Exchange:       exchange,
					ProviderSymbol: "1023.KL",
				}, nil
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/securities",
			`{"symbol":"CIMB","name":"CIMB Group","asset_type":"stock","currency":"MYR","exchange":"BURSA","provider_symbol":"1023.KL"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedExtraFields["provider_symbol"] != "1023.KL" {
			t.Errorf("expected provider_symbol=1023.KL in extraFields, got %v", capturedExtraFields["provider_symbol"])
		}
		result := parseJSON(t, rec)
		sec := result["security"].(map[string]interface{})
		if sec["provider_symbol"] != "1023.KL" {
			t.Errorf("expected provider_symbol=1023.KL in response, got %v", sec["provider_symbol"])
		}
	})

	t.Run("returns_409_duplicate", func(t *testing.T) {
		svc := &mockSecurityService{
			createSecurityFn: func(_, _ string, _ models.AssetType, _, _ string, _ map[string]interface{}) (*models.Security, error) {
				return nil, apperrors.ErrDuplicateSecurity
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/securities",
			`{"symbol":"AAPL","name":"Apple Inc.","asset_type":"stock"}`)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "DUPLICATE_SECURITY")
	})
}

func TestSecurityHandler_ListAllSecurities(t *testing.T) {
	t.Run("returns_200_with_securities", func(t *testing.T) {
		svc := &mockSecurityService{
			listAllSecuritiesFn: func() ([]models.Security, error) {
				return []models.Security{
					{Base: models.Base{ID: "sec-1"}, Symbol: "AAPL", Name: "Apple Inc.", AssetType: models.AssetTypeStock, Currency: "USD", Exchange: "NASDAQ"},
					{Base: models.Base{ID: "sec-7"}, Symbol: "BTC", Name: "Bitcoin", AssetType: models.AssetTypeCrypto, Currency: "USD", Network: "bitcoin"},
				}, nil
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/pipeline/securities", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		securities := result["securities"].([]interface{})
		if len(securities) != 2 {
			t.Errorf("expected 2 securities, got %d", len(securities))
		}
		first := securities[0].(map[string]interface{})
		if first["symbol"] != "AAPL" {
			t.Errorf("expected first symbol AAPL, got %v", first["symbol"])
		}
		second := securities[1].(map[string]interface{})
		if second["symbol"] != "BTC" {
			t.Errorf("expected second symbol BTC, got %v", second["symbol"])
		}
	})

	t.Run("returns_200_empty_list", func(t *testing.T) {
		svc := &mockSecurityService{
			listAllSecuritiesFn: func() ([]models.Security, error) {
				return []models.Security{}, nil
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/pipeline/securities", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		securities := result["securities"].([]interface{})
		if len(securities) != 0 {
			t.Errorf("expected empty securities array, got %d items", len(securities))
		}
	})

	t.Run("returns_500_on_service_error", func(t *testing.T) {
		svc := &mockSecurityService{
			listAllSecuritiesFn: func() ([]models.Security, error) {
				return nil, fmt.Errorf("database error")
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/pipeline/securities", "")

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestSecurityHandler_ListSecurities(t *testing.T) {
	t.Run("returns_200_with_data", func(t *testing.T) {
		svc := &mockSecurityService{
			listSecuritiesFn: func(_ string, _ pagination.PageRequest) (*pagination.PageResponse[models.Security], error) {
				resp := pagination.NewPageResponse([]models.Security{
					{Base: models.Base{ID: "sec-1"}, Symbol: "AAPL", Name: "Apple Inc.", AssetType: models.AssetTypeStock},
					{Base: models.Base{ID: "sec-2"}, Symbol: "GOOGL", Name: "Alphabet Inc.", AssetType: models.AssetTypeStock},
				}, 1, 20, 2)
				return &resp, nil
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/securities", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("expected 2 securities, got %d", len(data))
		}
		if result["total_items"].(float64) != 2 {
			t.Errorf("expected total_items=2, got %v", result["total_items"])
		}
	})

	t.Run("returns_200_with_pagination_params", func(t *testing.T) {
		var capturedPage pagination.PageRequest
		svc := &mockSecurityService{
			listSecuritiesFn: func(_ string, page pagination.PageRequest) (*pagination.PageResponse[models.Security], error) {
				capturedPage = page
				resp := pagination.NewPageResponse([]models.Security{}, 2, 5, 10)
				return &resp, nil
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/securities?page=2&page_size=5", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedPage.Page != 2 {
			t.Errorf("expected page=2, got %d", capturedPage.Page)
		}
		if capturedPage.PageSize != 5 {
			t.Errorf("expected page_size=5, got %d", capturedPage.PageSize)
		}
	})

	t.Run("passes_search_to_service", func(t *testing.T) {
		var capturedSearch string
		svc := &mockSecurityService{
			listSecuritiesFn: func(search string, _ pagination.PageRequest) (*pagination.PageResponse[models.Security], error) {
				capturedSearch = search
				resp := pagination.NewPageResponse([]models.Security{}, 1, 20, 0)
				return &resp, nil
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/securities?search=aapl", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedSearch != "aapl" {
			t.Errorf("expected search='aapl', got '%s'", capturedSearch)
		}
	})
}

func TestSecurityHandler_GetSecurity(t *testing.T) {
	t.Run("returns_200_on_success", func(t *testing.T) {
		svc := &mockSecurityService{
			getSecurityByIDFn: func(id string) (*models.Security, error) {
				return &models.Security{
					Base:      models.Base{ID: id},
					Symbol:    "AAPL",
					Name:      "Apple Inc.",
					AssetType: models.AssetTypeStock,
					Currency:  "USD",
					Exchange:  "NASDAQ",
				}, nil
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/securities/00000000-0000-0000-0000-000000000001", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		sec := result["security"].(map[string]interface{})
		if sec["symbol"] != "AAPL" {
			t.Errorf("expected symbol=AAPL, got %v", sec["symbol"])
		}
		if sec["exchange"] != "NASDAQ" {
			t.Errorf("expected exchange=NASDAQ, got %v", sec["exchange"])
		}
	})

	t.Run("returns_404_not_found", func(t *testing.T) {
		svc := &mockSecurityService{
			getSecurityByIDFn: func(_ string) (*models.Security, error) {
				return nil, apperrors.ErrSecurityNotFound
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/securities/00000000-0000-0000-0000-000000000999", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "SECURITY_NOT_FOUND")
	})

	t.Run("returns_400_invalid_id", func(t *testing.T) {
		handler := NewSecurityHandler(&mockSecurityService{}, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/securities/abc", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestSecurityHandler_RecordPrices(t *testing.T) {
	t.Run("returns_200_on_success", func(t *testing.T) {
		svc := &mockSecurityService{
			recordPricesFn: func(prices []services.SecurityPriceInput) (int, error) {
				return len(prices), nil
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/securities/prices",
			`{"prices":[{"security_id":"00000000-0000-0000-0000-000000000001","price":17500,"recorded_at":"2026-02-09T12:00:00Z"},{"security_id":"00000000-0000-0000-0000-000000000002","price":4200,"recorded_at":"2026-02-09T12:00:00Z"}]}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["prices_recorded"].(float64) != 2 {
			t.Errorf("expected prices_recorded=2, got %v", result["prices_recorded"])
		}
	})

	t.Run("returns_400_empty_prices", func(t *testing.T) {
		handler := NewSecurityHandler(&mockSecurityService{}, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/securities/prices",
			`{"prices":[]}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_400_invalid_price", func(t *testing.T) {
		handler := NewSecurityHandler(&mockSecurityService{}, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/securities/prices",
			`{"prices":[{"security_id":"00000000-0000-0000-0000-000000000001","price":0,"recorded_at":"2026-02-09T12:00:00Z"}]}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_400_missing_security_id", func(t *testing.T) {
		handler := NewSecurityHandler(&mockSecurityService{}, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/securities/prices",
			`{"prices":[{"price":17500,"recorded_at":"2026-02-09T12:00:00Z"}]}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_500_on_service_error", func(t *testing.T) {
		svc := &mockSecurityService{
			recordPricesFn: func(_ []services.SecurityPriceInput) (int, error) {
				return 0, fmt.Errorf("database error")
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/securities/prices",
			`{"prices":[{"security_id":"00000000-0000-0000-0000-000000000001","price":17500,"recorded_at":"2026-02-09T12:00:00Z"}]}`)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestSecurityHandler_GetPriceHistory(t *testing.T) {
	t.Run("returns_200_with_data", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		svc := &mockSecurityService{
			getPriceHistoryFn: func(_ string, _, _ time.Time, _ pagination.PageRequest) (*pagination.PageResponse[models.SecurityPrice], error) {
				resp := pagination.NewPageResponse([]models.SecurityPrice{
					{ID: "price-1", SecurityID: "sec-1", Price: 17500, RecordedAt: now},
					{ID: "price-2", SecurityID: "sec-1", Price: 17600, RecordedAt: now.Add(-time.Hour)},
				}, 1, 20, 2)
				return &resp, nil
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/securities/00000000-0000-0000-0000-000000000001/prices?from_date=2026-01-01&to_date=2026-12-31", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("expected 2 prices, got %d", len(data))
		}
		if result["total_items"].(float64) != 2 {
			t.Errorf("expected total_items=2, got %v", result["total_items"])
		}
	})

	t.Run("returns_400_missing_from_date", func(t *testing.T) {
		handler := NewSecurityHandler(&mockSecurityService{}, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/securities/00000000-0000-0000-0000-000000000001/prices?to_date=2026-12-31", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_400_missing_to_date", func(t *testing.T) {
		handler := NewSecurityHandler(&mockSecurityService{}, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/securities/00000000-0000-0000-0000-000000000001/prices?from_date=2026-01-01", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_400_invalid_id", func(t *testing.T) {
		handler := NewSecurityHandler(&mockSecurityService{}, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/securities/abc/prices?from_date=2026-01-01&to_date=2026-12-31", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("passes_date_range_and_pagination_to_service", func(t *testing.T) {
		var capturedSecID string
		var capturedPage pagination.PageRequest
		svc := &mockSecurityService{
			getPriceHistoryFn: func(securityID string, _, _ time.Time, page pagination.PageRequest) (*pagination.PageResponse[models.SecurityPrice], error) {
				capturedSecID = securityID
				capturedPage = page
				resp := pagination.NewPageResponse([]models.SecurityPrice{}, 3, 10, 25)
				return &resp, nil
			},
		}
		handler := NewSecurityHandler(svc, &mockAuditService{})
		r := setupSecurityRouter(handler)

		rec := doRequest(r, "GET", "/securities/00000000-0000-0000-0000-000000000005/prices?from_date=2026-01-01&to_date=2026-12-31&page=3&page_size=10", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedSecID != "00000000-0000-0000-0000-000000000005" {
			t.Errorf("expected securityID=00000000-0000-0000-0000-000000000005, got %s", capturedSecID)
		}
		if capturedPage.Page != 3 {
			t.Errorf("expected page=3, got %d", capturedPage.Page)
		}
		if capturedPage.PageSize != 10 {
			t.Errorf("expected page_size=10, got %d", capturedPage.PageSize)
		}
	})
}
