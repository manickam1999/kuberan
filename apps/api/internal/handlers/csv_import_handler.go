package handlers

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
)

// ImportCSVRoute is the Gin route pattern of the CSV import. It reads a file
// upload, so it enforces its own body cap instead of the global JSON body limit.
const ImportCSVRoute = "/api/v1/imports/csv"

// CSVImportHandler handles bank-statement CSV imports.
type CSVImportHandler struct {
	importService  services.CSVImportServicer
	auditService   services.AuditServicer
	maxUploadBytes int64
}

// NewCSVImportHandler creates a CSVImportHandler. maxUploadBytes caps the file.
func NewCSVImportHandler(importService services.CSVImportServicer, auditService services.AuditServicer, maxUploadBytes int64) *CSVImportHandler {
	return &CSVImportHandler{importService: importService, auditService: auditService, maxUploadBytes: maxUploadBytes}
}

// csvMappingFields are the form fields that make up the column mapping.
var csvMappingFields = []string{
	"date_column", "description_column", "amount_column", "debit_column", "credit_column",
	"category_column", "notes_column", "date_format", "decimal_separator", "amount_sign",
}

// ImportCSV imports a bank-statement CSV into one account.
// @Summary     Import a bank-statement CSV
// @Description Upload one CSV (at most 5,000 data rows) with a header row. A dry run (default) returns the detected columns, a sample, the suggested mapping and, when a mapping is sent, what would be created or skipped; apply=true writes the new rows in one transaction with source "import". Rows repeating an earlier import (same import_ref) and rows matching an entry typed by hand are skipped; the user's rules categorise the rest. Amounts are in the target account's currency. The file is processed in memory only.
// @Tags        imports
// @Accept      multipart/form-data
// @Produce     json
// @Security    BearerAuth
// @Param       file                formData file    true  "The CSV file (UTF-8)"
// @Param       account_id          formData string  true  "Cash, credit-card or debt account the rows go into"
// @Param       date_column         formData string  false "Header of the date column (starts the mapping)"
// @Param       description_column  formData string  false "Header of the description column"
// @Param       amount_column       formData string  false "Header of a signed amount column (or use debit_column and credit_column)"
// @Param       debit_column        formData string  false "Header of the money-out column"
// @Param       credit_column       formData string  false "Header of the money-in column"
// @Param       category_column     formData string  false "Header of a category column (matched to your categories by name)"
// @Param       notes_column        formData string  false "Header of a notes column"
// @Param       date_format         formData string  false "e.g. DD/MM/YYYY (default: the suggested format)"
// @Param       decimal_separator   formData string  false "'.' or ',' (default: the suggested one)"
// @Param       amount_sign         formData string  false "negative_is_expense (default) or positive_is_expense"
// @Param       apply               formData bool    false "Write the changes (default: dry run; needs a mapping)"
// @Success     200 {object} services.CSVImportReport
// @Failure     400 {object} ErrorResponse "Invalid input, CSV_TOO_MANY_ROWS or IMPORT_INVALID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Account not found"
// @Failure     409 {object} ErrorResponse "Another import is being applied"
// @Failure     413 {object} ErrorResponse "File too large"
// @Router      /imports/csv [post]
func (h *CSVImportHandler) ImportCSV(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes+importBodySlack)
	params, file, err := h.readForm(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	report, err := h.importService.ImportCSV(userID, params, file)
	if err != nil {
		respondWithError(c, err)
		return
	}
	if report.Applied {
		changes := map[string]interface{}{"created": report.Created}
		if sm := report.Summary; sm != nil {
			changes["rows"], changes["skipped_duplicate"] = sm.Rows, sm.SkipDuplicate
			changes["skipped_manual"], changes["invalid"] = sm.SkipManual, sm.Invalid
		}
		h.auditService.Log(userID, "IMPORT_CSV", "import", params.AccountID, c.ClientIP(), changes)
	}
	c.JSON(http.StatusOK, report)
}

// readForm streams the multipart body part by part and returns the import
// parameters and the file held in memory.
func (h *CSVImportHandler) readForm(c *gin.Context) (services.CSVImportParams, io.Reader, error) {
	var p services.CSVImportParams
	invalid := func(msg string) error { return apperrors.WithMessage(apperrors.ErrInvalidInput, msg) }

	mt, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || !strings.EqualFold(mt, "multipart/form-data") {
		return p, nil, invalid("expected a multipart/form-data body")
	}
	mr, err := c.Request.MultipartReader()
	if err != nil {
		return p, nil, invalid("invalid multipart body")
	}

	var file io.Reader
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
		if name == "file" {
			if file != nil {
				return p, nil, invalid("upload one file in the 'file' field")
			}
			data, err := io.ReadAll(io.LimitReader(part, h.maxUploadBytes+1))
			if err != nil {
				return p, nil, bodyReadError(err)
			}
			if int64(len(data)) > h.maxUploadBytes {
				return p, nil, apperrors.ErrPayloadTooLarge
			}
			file = bytes.NewReader(data)
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

	if file == nil {
		return p, nil, invalid("a CSV file is required in the 'file' field")
	}
	if p.AccountID, err = requiredUUID(fields, "account_id"); err != nil {
		return p, nil, err
	}
	if p.Apply, err = optionalBool(fields, "apply"); err != nil {
		return p, nil, err
	}
	for _, name := range csvMappingFields {
		if fields[name] != "" {
			p.Mapping = &services.CSVMapping{
				DateColumn: fields["date_column"], DescriptionColumn: fields["description_column"],
				AmountColumn: fields["amount_column"], DebitColumn: fields["debit_column"], CreditColumn: fields["credit_column"],
				CategoryColumn: fields["category_column"], NotesColumn: fields["notes_column"],
				DateFormat: fields["date_format"], DecimalSeparator: fields["decimal_separator"], AmountSign: fields["amount_sign"],
			}
			break
		}
	}
	return p, file, nil
}
