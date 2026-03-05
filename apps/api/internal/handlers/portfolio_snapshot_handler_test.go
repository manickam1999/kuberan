package handlers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// --- mock portfolio snapshot service ---

type mockPortfolioSnapshotService struct {
	computeAndRecordSnapshotsFn func(recordedAt time.Time) (int, error)
	getSnapshotsFn              func(userID string, from, to time.Time, page pagination.PageRequest) (*pagination.PageResponse[models.PortfolioSnapshot], error)
	getGroupedSnapshotsFn       func(userID string, from, to time.Time, groupBy string) ([]models.PortfolioSnapshot, error)
}

var _ services.PortfolioSnapshotServicer = (*mockPortfolioSnapshotService)(nil)

func (m *mockPortfolioSnapshotService) ComputeAndRecordSnapshots(recordedAt time.Time) (int, error) {
	if m.computeAndRecordSnapshotsFn != nil {
		return m.computeAndRecordSnapshotsFn(recordedAt)
	}
	return 0, nil
}

func (m *mockPortfolioSnapshotService) GetSnapshots(userID string, from, to time.Time, page pagination.PageRequest) (*pagination.PageResponse[models.PortfolioSnapshot], error) {
	if m.getSnapshotsFn != nil {
		return m.getSnapshotsFn(userID, from, to, page)
	}
	resp := pagination.NewPageResponse([]models.PortfolioSnapshot{}, 1, 20, 0)
	return &resp, nil
}

func (m *mockPortfolioSnapshotService) GetGroupedSnapshots(userID string, from, to time.Time, groupBy string) ([]models.PortfolioSnapshot, error) {
	if m.getGroupedSnapshotsFn != nil {
		return m.getGroupedSnapshotsFn(userID, from, to, groupBy)
	}
	return []models.PortfolioSnapshot{}, nil
}

// --- router setup ---

func setupSnapshotRouter(handler *PortfolioSnapshotHandler) *gin.Engine {
	r := gin.New()
	// Pipeline route (no user auth)
	r.POST("/pipeline/snapshots/compute", handler.ComputeSnapshots)
	// User route (with auth)
	auth := r.Group("", injectUserID("test-user-1"))
	auth.GET("/portfolio/snapshots", handler.GetSnapshots)
	return r
}

// --- tests ---

func TestPortfolioSnapshotHandler_ComputeSnapshots(t *testing.T) {
	t.Run("returns_200_on_success", func(t *testing.T) {
		svc := &mockPortfolioSnapshotService{
			computeAndRecordSnapshotsFn: func(_ time.Time) (int, error) {
				return 3, nil
			},
		}
		handler := NewPortfolioSnapshotHandler(svc, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/snapshots/compute",
			`{"recorded_at":"2026-02-09T12:00:00Z"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["snapshots_recorded"].(float64) != 3 {
			t.Errorf("expected snapshots_recorded=3, got %v", result["snapshots_recorded"])
		}
	})

	t.Run("returns_400_missing_recorded_at", func(t *testing.T) {
		handler := NewPortfolioSnapshotHandler(&mockPortfolioSnapshotService{}, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/snapshots/compute", `{}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_500_on_service_error", func(t *testing.T) {
		svc := &mockPortfolioSnapshotService{
			computeAndRecordSnapshotsFn: func(_ time.Time) (int, error) {
				return 0, fmt.Errorf("database error")
			},
		}
		handler := NewPortfolioSnapshotHandler(svc, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "POST", "/pipeline/snapshots/compute",
			`{"recorded_at":"2026-02-09T12:00:00Z"}`)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestPortfolioSnapshotHandler_GetSnapshots(t *testing.T) {
	t.Run("returns_200_with_data", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		svc := &mockPortfolioSnapshotService{
			getSnapshotsFn: func(_ string, _, _ time.Time, _ pagination.PageRequest) (*pagination.PageResponse[models.PortfolioSnapshot], error) {
				resp := pagination.NewPageResponse([]models.PortfolioSnapshot{
					{ID: "snap-1", UserID: "test-user-1", RecordedAt: now, TotalNetWorth: 15500000, CashBalance: 5000000, InvestmentValue: 11000000, DebtBalance: 500000},
				}, 1, 20, 1)
				return &resp, nil
			},
		}
		handler := NewPortfolioSnapshotHandler(svc, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "GET", "/portfolio/snapshots?from_date=2026-01-01&to_date=2026-12-31", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 1 {
			t.Errorf("expected 1 snapshot, got %d", len(data))
		}
		snap := data[0].(map[string]interface{})
		if snap["total_net_worth"].(float64) != 15500000 {
			t.Errorf("expected total_net_worth=15500000, got %v", snap["total_net_worth"])
		}
	})

	t.Run("returns_400_missing_from_date", func(t *testing.T) {
		handler := NewPortfolioSnapshotHandler(&mockPortfolioSnapshotService{}, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "GET", "/portfolio/snapshots?to_date=2026-12-31", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_400_missing_to_date", func(t *testing.T) {
		handler := NewPortfolioSnapshotHandler(&mockPortfolioSnapshotService{}, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "GET", "/portfolio/snapshots?from_date=2026-01-01", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_200_empty_data", func(t *testing.T) {
		svc := &mockPortfolioSnapshotService{
			getSnapshotsFn: func(_ string, _, _ time.Time, _ pagination.PageRequest) (*pagination.PageResponse[models.PortfolioSnapshot], error) {
				resp := pagination.NewPageResponse([]models.PortfolioSnapshot{}, 1, 20, 0)
				return &resp, nil
			},
		}
		handler := NewPortfolioSnapshotHandler(svc, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "GET", "/portfolio/snapshots?from_date=2026-01-01&to_date=2026-12-31", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 0 {
			t.Errorf("expected 0 snapshots, got %d", len(data))
		}
		if result["total_items"].(float64) != 0 {
			t.Errorf("expected total_items=0, got %v", result["total_items"])
		}
	})

	t.Run("returns_401_without_auth", func(t *testing.T) {
		handler := NewPortfolioSnapshotHandler(&mockPortfolioSnapshotService{}, &mockAuditService{})
		r := gin.New()
		r.GET("/portfolio/snapshots", handler.GetSnapshots)

		rec := doRequest(r, "GET", "/portfolio/snapshots?from_date=2026-01-01&to_date=2026-12-31", "")

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("passes_user_id_and_pagination_to_service", func(t *testing.T) {
		var capturedUserID string
		var capturedPage pagination.PageRequest
		svc := &mockPortfolioSnapshotService{
			getSnapshotsFn: func(userID string, _, _ time.Time, page pagination.PageRequest) (*pagination.PageResponse[models.PortfolioSnapshot], error) {
				capturedUserID = userID
				capturedPage = page
				resp := pagination.NewPageResponse([]models.PortfolioSnapshot{}, 2, 5, 10)
				return &resp, nil
			},
		}
		handler := NewPortfolioSnapshotHandler(svc, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "GET", "/portfolio/snapshots?from_date=2026-01-01&to_date=2026-12-31&page=2&page_size=5", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedUserID != "test-user-1" {
			t.Errorf("expected userID=test-user-1, got %s", capturedUserID)
		}
		if capturedPage.Page != 2 {
			t.Errorf("expected page=2, got %d", capturedPage.Page)
		}
		if capturedPage.PageSize != 5 {
			t.Errorf("expected page_size=5, got %d", capturedPage.PageSize)
		}
	})

	t.Run("group_by_day_returns_200_with_data", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		svc := &mockPortfolioSnapshotService{
			getGroupedSnapshotsFn: func(_ string, _, _ time.Time, groupBy string) ([]models.PortfolioSnapshot, error) {
				if groupBy != "day" {
					t.Errorf("expected groupBy=day, got %s", groupBy)
				}
				return []models.PortfolioSnapshot{
					{ID: "snap-1", UserID: "test-user-1", RecordedAt: now, TotalNetWorth: 15500000, CashBalance: 5000000, InvestmentValue: 11000000, DebtBalance: 500000},
				}, nil
			},
		}
		handler := NewPortfolioSnapshotHandler(svc, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "GET", "/portfolio/snapshots?from_date=2026-01-01&to_date=2026-12-31&group_by=day", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 1 {
			t.Errorf("expected 1 snapshot, got %d", len(data))
		}
		// Should not have pagination fields
		if _, ok := result["total_items"]; ok {
			t.Error("expected no total_items field for grouped response")
		}
	})

	t.Run("group_by_hour_returns_200", func(t *testing.T) {
		svc := &mockPortfolioSnapshotService{
			getGroupedSnapshotsFn: func(_ string, _, _ time.Time, groupBy string) ([]models.PortfolioSnapshot, error) {
				if groupBy != "hour" {
					t.Errorf("expected groupBy=hour, got %s", groupBy)
				}
				return []models.PortfolioSnapshot{}, nil
			},
		}
		handler := NewPortfolioSnapshotHandler(svc, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "GET", "/portfolio/snapshots?from_date=2026-01-01&to_date=2026-12-31&group_by=hour", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("group_by_invalid_returns_400", func(t *testing.T) {
		handler := NewPortfolioSnapshotHandler(&mockPortfolioSnapshotService{}, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "GET", "/portfolio/snapshots?from_date=2026-01-01&to_date=2026-12-31&group_by=week", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("without_group_by_returns_paginated", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		svc := &mockPortfolioSnapshotService{
			getSnapshotsFn: func(_ string, _, _ time.Time, _ pagination.PageRequest) (*pagination.PageResponse[models.PortfolioSnapshot], error) {
				resp := pagination.NewPageResponse([]models.PortfolioSnapshot{
					{ID: "snap-1", UserID: "test-user-1", RecordedAt: now, TotalNetWorth: 15500000},
				}, 1, 20, 1)
				return &resp, nil
			},
		}
		handler := NewPortfolioSnapshotHandler(svc, &mockAuditService{})
		r := setupSnapshotRouter(handler)

		rec := doRequest(r, "GET", "/portfolio/snapshots?from_date=2026-01-01&to_date=2026-12-31", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		// Should have pagination fields
		if _, ok := result["total_items"]; !ok {
			t.Error("expected total_items field for paginated response")
		}
	})
}
