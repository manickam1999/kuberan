package handlers

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
	"kuberan/internal/uuid"
)

const (
	// ImportIBKRRoute is the Gin route pattern of the IBKR import. It reads up to
	// importMaxFiles uploads, so it enforces its own body cap instead of the
	// global JSON body limit.
	ImportIBKRRoute = "/api/v1/imports/ibkr"

	importMaxFiles = 10
	// importFieldMaxBytes bounds each non-file form field.
	importFieldMaxBytes = 1024
	// importBodySlack covers multipart framing and the small form fields.
	importBodySlack = 64 * 1024
)

// ImportHandler handles statement imports.
type ImportHandler struct {
	importService  services.ImportServicer
	auditService   services.AuditServicer
	maxUploadBytes int64
}

// NewImportHandler creates a new ImportHandler. maxUploadBytes caps each
// uploaded statement file.
func NewImportHandler(importService services.ImportServicer, auditService services.AuditServicer, maxUploadBytes int64) *ImportHandler {
	return &ImportHandler{importService: importService, auditService: auditService, maxUploadBytes: maxUploadBytes}
}

// ImportIBKR imports Interactive Brokers Activity Statements.
// @Summary     Import IBKR Activity Statements
// @Description Upload 1-10 contiguous Activity Statement CSVs. A dry run (default) returns the report without writing; apply=true writes the holdings and the cash ledger in one transaction and is refused (400 IMPORT_INVALID, report in details) when the report has errors or a quantity checkpoint fails. Uploads are processed in memory only.
// @Tags        imports
// @Accept      multipart/form-data
// @Produce     json
// @Security    BearerAuth
// @Param       files                        formData file    true  "Activity Statement CSVs (1-10)"
// @Param       investment_account_id        formData string  true  "Investment account of the holdings"
// @Param       cash_account_id              formData string  true  "Cash account (statement base currency) for the cash ledger"
// @Param       cash_start_date              formData string  true  "YYYY-MM-DD; cash legs start on this date"
// @Param       legacy_cash_account_id       formData string  false "Old IBKR cash account whose incoming transfers are re-pointed"
// @Param       deactivate_legacy_cash_account formData bool  false "Deactivate the legacy account after apply"
// @Param       apply                        formData bool    false "Write the changes (default: dry run)"
// @Success     200 {object} services.IBKRImportReport
// @Failure     400 {object} ErrorResponse "Invalid input or IMPORT_INVALID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Account not found"
// @Failure     409 {object} ErrorResponse "Another import is being applied"
// @Failure     413 {object} ErrorResponse "File too large"
// @Router      /imports/ibkr [post]
func (h *ImportHandler) ImportIBKR(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes*importMaxFiles+importBodySlack)
	params, files, err := h.readForm(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	report, err := h.importService.ImportIBKR(userID, params, files)
	if err != nil {
		respondWithError(c, err)
		return
	}

	if report.Applied {
		newTx, legs := 0, report.Cash.Legs+report.Cash.Dividends+report.Cash.Fees
		for _, s := range report.Securities {
			newTx += s.NewTransactions
		}
		h.auditService.Log(userID, "IMPORT_IBKR", "import", "", c.ClientIP(), map[string]interface{}{
			"files": len(files), "securities": len(report.Securities), "investment_transactions": newTx,
			"cash_rows": legs, "deposits_matched": len(report.Cash.DepositsMatched), "warnings": len(report.Warnings),
		})
	}
	c.JSON(http.StatusOK, report)
}

// readForm streams the multipart body part by part, so no upload is spooled to
// disk, and returns the import parameters and the files held in memory.
func (h *ImportHandler) readForm(c *gin.Context) (services.IBKRImportParams, []io.Reader, error) {
	var p services.IBKRImportParams
	invalid := func(msg string) error { return apperrors.WithMessage(apperrors.ErrInvalidInput, msg) }

	mt, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || !strings.EqualFold(mt, "multipart/form-data") {
		return p, nil, invalid("expected a multipart/form-data body")
	}
	mr, err := c.Request.MultipartReader()
	if err != nil {
		return p, nil, invalid("invalid multipart body")
	}

	var files []io.Reader
	fields := map[string]string{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return p, nil, bodyReadError(err)
		}
		name := part.FormName()
		if name == "files" {
			if len(files) >= importMaxFiles {
				return p, nil, invalid("upload at most 10 statement files")
			}
			data, err := io.ReadAll(io.LimitReader(part, h.maxUploadBytes+1))
			if err != nil {
				return p, nil, bodyReadError(err)
			}
			if int64(len(data)) > h.maxUploadBytes {
				return p, nil, apperrors.ErrPayloadTooLarge
			}
			files = append(files, bytes.NewReader(data))
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, importFieldMaxBytes+1))
		if err != nil {
			return p, nil, bodyReadError(err)
		}
		if len(data) > importFieldMaxBytes {
			return p, nil, invalid("form field " + name + " is too long")
		}
		fields[name] = strings.TrimSpace(string(data))
	}

	if len(files) == 0 {
		return p, nil, invalid("at least one statement file is required in the 'files' field")
	}
	if p.InvestmentAccountID, err = requiredUUID(fields, "investment_account_id"); err != nil {
		return p, nil, err
	}
	if p.CashAccountID, err = requiredUUID(fields, "cash_account_id"); err != nil {
		return p, nil, err
	}
	if legacy := fields["legacy_cash_account_id"]; legacy != "" {
		if !uuid.IsValid(legacy) {
			return p, nil, invalid("Invalid legacy_cash_account_id format")
		}
		p.LegacyCashAccountID = legacy
	}
	start, err := time.Parse("2006-01-02", fields["cash_start_date"])
	if err != nil {
		return p, nil, invalid("cash_start_date must be a date in YYYY-MM-DD format")
	}
	p.CashStartDate = start
	if p.Apply, err = optionalBool(fields, "apply"); err != nil {
		return p, nil, err
	}
	if p.DeactivateLegacyCashAccount, err = optionalBool(fields, "deactivate_legacy_cash_account"); err != nil {
		return p, nil, err
	}
	return p, files, nil
}

func requiredUUID(fields map[string]string, name string) (string, error) {
	v := fields[name]
	if v == "" {
		return "", apperrors.WithMessage(apperrors.ErrInvalidInput, name+" is required")
	}
	if !uuid.IsValid(v) {
		return "", apperrors.WithMessage(apperrors.ErrInvalidInput, "Invalid "+name+" format")
	}
	return v, nil
}

func optionalBool(fields map[string]string, name string) (bool, error) {
	v := fields[name]
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, apperrors.WithMessage(apperrors.ErrInvalidInput, name+" must be true or false")
	}
	return b, nil
}

// bodyReadError maps a body read failure: crossing the request cap is a 413.
func bodyReadError(err error) error {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return apperrors.ErrPayloadTooLarge
	}
	return apperrors.WithMessage(apperrors.ErrInvalidInput, "could not read the upload")
}
