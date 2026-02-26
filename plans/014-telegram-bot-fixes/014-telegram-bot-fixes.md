# Telegram Bot Integration Fixes

## Context

The `telegram-bot` feature branch adds a Telegram bot integration across three layers: Go API endpoints, a Python bot service, and a Next.js frontend settings page. A code review identified several issues that should be fixed before merging to `main`.

### Issues Identified

1. **Migration/GORM mismatch on unique indexes** — The `TelegramLink` GORM model declares `uniqueIndex` on `UserID` and `TelegramUserID`, but migration 000018 creates regular (non-unique) indexes. Since this project uses golang-migrate (not GORM AutoMigrate), the GORM tags are cosmetic and the actual DB has no uniqueness enforcement. This allows duplicate links per user or duplicate Telegram accounts linked to different users.

2. **`GetUserWithAuthToken` returns `map[string]interface{}`** — Every other service method in the codebase returns typed structs or model pointers. This is the only one returning a map, which loses type safety and is inconsistent with established patterns.

3. **No tests for new Go code** — The telegram handler, service, and `InternalAuthMiddleware` have zero test coverage. The `TelegramLink` model is also missing from the test DB's `allModels` list, so service tests can't run against it.

4. **Bot `create_transaction` call not wrapped in try/except** — In the transaction confirm handler, the API call to create the transaction has no error handling. If the API returns an error, the user gets no feedback and the conversation state is left inconsistent.

5. **Hardcoded `'USD'` in balance total** — The `/balance` command sums all account balances and formats the total as USD regardless of the user's default currency.

6. **Missing `debt` account type in bot formatting** — The `format_account_type` map handles `cash`, `investment`, and `credit_card` but not `debt`, which is a supported account type.

7. **TypeScript `TelegramLink` uses `number` for UUID fields** — `id` and `user_id` are UUIDs (strings) from the backend but typed as `number` in the frontend interface.

8. **Hardcoded light-mode colors in Telegram settings** — The linked-status alert uses `bg-green-50`, `border-green-200`, and `text-green-900` without `dark:` variants, breaking dark mode.

### What's Intentionally Left Alone

- **Bot JWT tokens (1-year expiry)** — Accepted as-is. The bot re-resolves tokens on each request anyway.
- **Link code entropy (hex, 16^6)** — Accepted as-is. 15-minute expiry and single-use mitigate brute-force risk.
- **Blocking `requests` in async bot handlers** — Out of scope for this fix pass. Would require switching to `httpx`/`aiohttp` across all handlers.
- **Migration squash (19 into 18)** — Left as separate migrations per owner preference.

## Scope Summary

| Fix | Area | Files Changed |
|---|---|---|
| Unique indexes in migration | Backend (API) | `migrations/000018_create_telegram_links.up.sql` |
| Typed return struct | Backend (API) | `services/interfaces.go`, `services/telegram_service.go`, `handlers/telegram_handler.go` |
| Test coverage | Backend (API) | New: `services/telegram_service_test.go`, `handlers/telegram_handler_test.go`; Modified: `middleware/pipeline_auth_test.go`, `testutil/database.go`, `testutil/fixtures.go` |
| Bot error handling | Bot (Python) | `handlers/transaction_flow.py` |
| Bot balance currency fix | Bot (Python) | `handlers/balance.py` |
| Bot debt account type | Bot (Python) | `utils/formatting.py` |
| Frontend type fix | Frontend (Next.js) | `hooks/use-telegram.ts` |
| Frontend dark mode | Frontend (Next.js) | `components/settings/telegram-settings.tsx` |

---

## Phase 1: Fix Migration Indexes

### 1.1 Make Unique Indexes in Migration 000018

**File**: `apps/api/migrations/000018_create_telegram_links.up.sql`

Change the two regular indexes to unique indexes. For `telegram_user_id`, use a partial unique index to avoid conflicts when `telegram_user_id = 0` (pending/unlinked rows):

```sql
-- Change from:
CREATE INDEX IF NOT EXISTS idx_telegram_links_user_id ON telegram_links (user_id);
CREATE INDEX IF NOT EXISTS idx_telegram_links_telegram_user_id ON telegram_links (telegram_user_id);

-- Change to:
CREATE UNIQUE INDEX IF NOT EXISTS idx_telegram_links_user_id ON telegram_links (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_telegram_links_telegram_user_id ON telegram_links (telegram_user_id) WHERE telegram_user_id != 0;
```

Rationale:
- `user_id` is always set (NOT NULL, foreign key), so a plain unique index is correct — one link per user.
- `telegram_user_id` defaults to `0` for rows created via `GenerateLinkCode` before `CompleteLink` is called. Multiple pending rows would have `telegram_user_id = 0`, so the unique index must exclude those. PostgreSQL partial indexes (`WHERE telegram_user_id != 0`) handle this correctly.

**Note on GORM tag consistency**: The GORM model tags (`uniqueIndex`) are now consistent with the SQL migration for `user_id`. For `telegram_user_id`, the GORM tag says `uniqueIndex` (unconditional) while the SQL uses a partial index. Since GORM AutoMigrate is not used, the SQL is the source of truth. The GORM tag is informational — it communicates the intent (unique per linked account) even though it can't express the partial condition.

### 1.2 Verification

Run `go build ./...` from `apps/api/` to ensure no issues.

---

## Phase 2: Typed Return Struct for GetUserWithAuthToken

### 2.1 Define TelegramUserAuth Struct

**File**: `apps/api/internal/services/interfaces.go`

Add a new struct near the other result types:

```go
// TelegramUserAuth holds the resolved user info and auth token for bot service communication.
type TelegramUserAuth struct {
	UserID          string `json:"user_id"`
	Email           string `json:"email"`
	AuthToken       string `json:"auth_token"`
	DefaultCurrency string `json:"default_currency"`
}
```

### 2.2 Update Interface

**File**: `apps/api/internal/services/interfaces.go`

Change the `TelegramServicer` interface method signature:

```go
// Change from:
GetUserWithAuthToken(telegramUserID int64) (map[string]interface{}, error)

// Change to:
GetUserWithAuthToken(telegramUserID int64) (*TelegramUserAuth, error)
```

### 2.3 Update Service Implementation

**File**: `apps/api/internal/services/telegram_service.go`

Update `GetUserWithAuthToken` to return `*TelegramUserAuth`:

```go
func (s *telegramService) GetUserWithAuthToken(telegramUserID int64) (*TelegramUserAuth, error) {
	// ... existing link + user lookup logic unchanged ...

	token, err := middleware.GenerateBotToken(&user)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return &TelegramUserAuth{
		UserID:          user.ID,
		Email:           user.Email,
		AuthToken:       token,
		DefaultCurrency: link.DefaultCurrency,
	}, nil
}
```

### 2.4 Update Handler

**File**: `apps/api/internal/handlers/telegram_handler.go`

The `ResolveUser` handler currently does `c.JSON(http.StatusOK, userData)` where `userData` is `map[string]interface{}`. After the change, `userData` is `*TelegramUserAuth` — this serializes identically via `json` struct tags, so the handler code stays the same. Just verify the response shape doesn't change.

### 2.5 Verification

Run `go build ./...` from `apps/api/`.

---

## Phase 3: Add Test Coverage

### 3.1 Add TelegramLink to Test DB

**File**: `apps/api/internal/testutil/database.go`

Add `&models.TelegramLink{}` to the `allModels` slice so that `SetupTestDB` creates the `telegram_links` table in the test SQLite database.

### 3.2 Add Test Fixtures

**File**: `apps/api/internal/testutil/fixtures.go`

Add a `CreateTestTelegramLink` helper:

```go
// CreateTestTelegramLink creates a linked TelegramLink for testing.
func CreateTestTelegramLink(t *testing.T, db *gorm.DB, userID string, telegramUserID int64) *models.TelegramLink {
	t.Helper()
	link := &models.TelegramLink{
		UserID:            userID,
		TelegramUserID:    telegramUserID,
		TelegramUsername:  "testuser",
		TelegramFirstName: "Test",
		DefaultCurrency:   "MYR",
		IsActive:          true,
	}
	if err := db.Create(link).Error; err != nil {
		t.Fatalf("failed to create test telegram link: %v", err)
	}
	return link
}
```

### 3.3 Service Tests

**New file**: `apps/api/internal/services/telegram_service_test.go`

Follow existing service test patterns: each subtest creates its own `SetupTestDB` + `TeardownTestDB`, uses `testutil.CreateTestUser` for user fixtures. Test the real service implementation against SQLite.

Test cases:

**TestGenerateLinkCode**:
- `creates_new_link_for_user` — call GenerateLinkCode, verify link returned with non-empty LinkCode and LinkCodeExpiresAt ~15min from now, IsActive=false
- `updates_existing_link_code` — create link via GenerateLinkCode, call again, verify new code replaces old one, same link ID
- `preserves_existing_telegram_data` — create a fully linked record, call GenerateLinkCode, verify TelegramUserID/Username are preserved but code is updated

**TestCompleteLink**:
- `valid_code` — GenerateLinkCode then CompleteLink with valid code, verify TelegramUserID set, IsActive=true, LinkCode cleared, LinkCodeExpiresAt cleared
- `invalid_code` — CompleteLink with non-existent code, assert `ErrInvalidLinkCode`
- `expired_code` — create link, manually set LinkCodeExpiresAt to past, CompleteLink, assert `ErrLinkCodeExpired`
- `telegram_already_linked` — create two users, link first user's code, try to link second user's code with same Telegram ID, assert `ErrTelegramAlreadyLinked`
- `sets_default_currency` — CompleteLink with `defaultCurrency="JPY"`, verify it's persisted

**TestGetLinkByUserID**:
- `found` — create link, retrieve by UserID, verify fields match
- `not_found` — retrieve non-existent user, assert `ErrNotFound`

**TestGetLinkByTelegramID**:
- `found_active` — create active link, retrieve by TelegramUserID, verify match
- `not_found_inactive` — create link with IsActive=false, retrieve, assert `ErrNotFound`
- `not_found` — retrieve non-existent TelegramUserID, assert `ErrNotFound`

**TestUnlinkAccount**:
- `success` — create link, unlink, verify deleted (soft delete)
- `not_found` — unlink non-existent user, assert `ErrNotFound`

**TestRecordActivity**:
- `updates_timestamp_and_count` — create link, call RecordActivity, verify LastMessageAt is set and MessageCount=1. Call again, verify MessageCount=2.

**TestIsLinked**:
- `true_when_active` — create active link, verify IsLinked returns true
- `false_when_inactive` — create link with IsActive=false, verify returns false
- `false_when_no_link` — verify returns false for non-existent user

**TestGetUserWithAuthToken**:
- `returns_user_auth` — create user + linked TelegramLink, call GetUserWithAuthToken, verify UserID, Email, DefaultCurrency match, AuthToken is non-empty
- `not_found` — call with non-existent TelegramUserID, assert `ErrNotFound`

### 3.4 Handler Tests

**New file**: `apps/api/internal/handlers/telegram_handler_test.go`

Follow existing handler test patterns: mock service with function fields, `httptest`, `gin.TestMode`.

Define `mockTelegramService` with function fields for each `TelegramServicer` method. Verify interface compliance with `var _ services.TelegramServicer = (*mockTelegramService)(nil)`.

Define `setupTelegramRouter` that creates a Gin engine with `injectUserID` middleware and registers all telegram routes.

Define `setupInternalTelegramRouter` for internal endpoints (no JWT auth, just route directly).

Test cases:

**TestTelegramHandler_GetLink**:
- `returns_200_with_link` — mock returns link, verify JSON response shape
- `returns_404_when_not_found` — mock returns ErrNotFound, verify 404
- `returns_401_without_auth` — no injectUserID, verify 401

**TestTelegramHandler_GenerateCode**:
- `returns_200_with_code` — mock returns link with code, verify `link_code` and `expires_at` in response
- `returns_500_on_service_error` — mock returns ErrInternalServer, verify 500

**TestTelegramHandler_Unlink**:
- `returns_200_on_success` — mock returns nil, verify success message
- `returns_404_when_not_linked` — mock returns ErrNotFound, verify 404

**TestTelegramHandler_CompleteLink**:
- `returns_200_on_success` — POST valid JSON body, mock returns nil, verify success message
- `returns_400_on_invalid_body` — POST with missing required fields, verify 400
- `returns_400_on_expired_code` — mock returns ErrLinkCodeExpired, verify 400

**TestTelegramHandler_ResolveUser**:
- `returns_200_with_user_data` — mock returns TelegramUserAuth, verify JSON fields
- `returns_404_when_not_found` — mock returns ErrNotFound, verify 404
- `returns_400_on_invalid_id` — request with non-numeric path param, verify 400

**TestTelegramHandler_RecordActivity**:
- `returns_200_on_success` — mock returns nil, verify success message
- `returns_400_on_invalid_id` — non-numeric path param, verify 400

### 3.5 InternalAuthMiddleware Tests

**File**: `apps/api/internal/middleware/pipeline_auth_test.go`

Add `TestInternalAuthMiddleware` following the same table-driven pattern as the existing `TestPipelineAuthMiddleware`:

Add `setupInternalRouter(secret string) *gin.Engine` helper — creates Gin engine with `InternalAuthMiddleware(secret)` and a dummy POST `/test` handler.

Add `doInternalRequest(r *gin.Engine, secret string) *httptest.ResponseRecorder` helper — sends POST with optional `X-Internal-Secret` header.

Test cases (table-driven):

| Name | Configured Secret | Request Secret | Expected Status | Expected Error Code |
|---|---|---|---|---|
| `valid_secret` | `"test-secret"` | `"test-secret"` | 200 | — |
| `invalid_secret` | `"test-secret"` | `"wrong-secret"` | 401 | `INVALID_INTERNAL_SECRET` |
| `missing_secret` | `"test-secret"` | `""` | 401 | `INVALID_INTERNAL_SECRET` |
| `empty_configured_secret` | `""` | `"any"` | 503 | `INTERNAL_AUTH_NOT_CONFIGURED` |
| `both_empty` | `""` | `""` | 503 | `INTERNAL_AUTH_NOT_CONFIGURED` |
| `partial_match_rejected` | `"test-secret"` | `"test-secre"` | 401 | `INVALID_INTERNAL_SECRET` |

### 3.6 Verification

Run `./scripts/check-go.sh apps/api`. All 5 steps must pass.

---

## Phase 4: Bot Fixes

### 4.1 Wrap create_transaction in try/except

**File**: `apps/bot/handlers/transaction_flow.py`

In `handle_confirm_callback`, wrap the `create_transaction` call (around line 572) in a try/except block. On failure, show an error message to the user and return `CONFIRM` so they can retry:

```python
if data == "txn:confirm":
    user_client = txn['user_client']
    transaction_data = {
        "type": txn['type'],
        "account_id": txn['account_id'],
        "amount": txn['amount'],
        "description": txn['description'] or txn['type'].title(),
    }
    if txn.get('category_id'):
        transaction_data['category_id'] = txn['category_id']

    try:
        user_client.create_transaction(transaction_data)
    except Exception as e:
        logger.error(f"Failed to create transaction: {e}")
        await query.edit_message_text(
            "Failed to record transaction. Please try again.",
            reply_markup=_build_confirm_keyboard(),
        )
        return CONFIRM

    await query.edit_message_text(
        _format_success_message(txn),
        parse_mode='Markdown'
    )
    context.user_data.pop(TXN_KEY, None)
    return ConversationHandler.END
```

### 4.2 Fix Hardcoded USD in Balance Total

**File**: `apps/bot/handlers/balance.py`

Change line 59 from:
```python
message += f"*Total:* {format_currency(total, 'USD')}"
```

To use the user's default currency:
```python
currency = user_data.get('default_currency', 'MYR')
message += f"*Total:* {format_currency(total, currency)}"
```

Also add a comment noting the limitation:
```python
# Note: mixed-currency totals are summed naively without conversion
```

The `currency` variable should be extracted once near the top of the try block and used for both individual accounts (as fallback) and the total.

### 4.3 Add Debt Account Type to Formatting

**File**: `apps/bot/utils/formatting.py`

Add `"debt"` to the `type_map` in `format_account_type`:

```python
type_map = {
    "cash": "💵 Cash",
    "investment": "📈 Investment",
    "credit_card": "💳 Credit Card",
    "debt": "🏦 Debt",
}
```

### 4.4 Verification

Manual review — no automated Python tests in this project currently.

---

## Phase 5: Frontend Fixes

### 5.1 Fix TelegramLink TypeScript Interface

**File**: `apps/web/src/hooks/use-telegram.ts`

Change:
```typescript
interface TelegramLink {
  id: number;
  user_id: number;
  telegram_user_id: number;
  // ...
}
```

To:
```typescript
interface TelegramLink {
  id: string;
  user_id: string;
  telegram_user_id: number;
  // ...
}
```

`id` and `user_id` are UUIDs from the backend (strings). `telegram_user_id` stays as `number` — Telegram user IDs are integers and current values fit within JavaScript's safe integer range.

### 5.2 Fix Dark Mode Colors

**File**: `apps/web/src/components/settings/telegram-settings.tsx`

Line 200 — change:
```tsx
<Alert className="border-green-200 bg-green-50">
```
To:
```tsx
<Alert className="border-green-200 bg-green-50 dark:border-green-800 dark:bg-green-950">
```

Line 201 — change:
```tsx
<AlertDescription className="text-green-900">
```
To:
```tsx
<AlertDescription className="text-green-900 dark:text-green-100">
```

### 5.3 Verification

Run `pnpm build` from `apps/web/` to verify TypeScript compilation. Visually verify dark mode in browser.

---

## Phase 6: Final Verification

### 6.1 Go Backend

Run `./scripts/check-go.sh apps/api`. All 5 steps must pass (build, vet, lint, test, test -race).

### 6.2 Frontend

Run `pnpm build` from `apps/web/`. Must compile without errors.

### 6.3 Verification Checklist

- [ ] Migration 000018 creates UNIQUE index on `user_id`
- [ ] Migration 000018 creates partial UNIQUE index on `telegram_user_id WHERE != 0`
- [ ] `GetUserWithAuthToken` returns `*TelegramUserAuth` (not `map[string]interface{}`)
- [ ] `TelegramLink` is in `testutil.allModels`
- [ ] `CreateTestTelegramLink` fixture exists
- [ ] `telegram_service_test.go` exists with tests for all service methods
- [ ] `telegram_handler_test.go` exists with tests for all endpoints
- [ ] `TestInternalAuthMiddleware` exists in `pipeline_auth_test.go`
- [ ] Bot `create_transaction` is wrapped in try/except
- [ ] Bot `/balance` total uses `user_data.get('default_currency', 'MYR')` not `'USD'`
- [ ] `format_account_type` includes `"debt"` key
- [ ] TypeScript `TelegramLink.id` is `string`, `TelegramLink.user_id` is `string`
- [ ] Telegram settings alert has `dark:` variants for green colors
- [ ] All Go tests pass (including new ones)
- [ ] Frontend builds without TypeScript errors

---

## Files Changed

### New Files

```
apps/api/internal/services/telegram_service_test.go
apps/api/internal/handlers/telegram_handler_test.go
```

### Modified Files — API

```
apps/api/
├── migrations/
│   └── 000018_create_telegram_links.up.sql     # UNIQUE indexes
├── internal/
│   ├── services/
│   │   ├── interfaces.go                       # TelegramUserAuth struct, interface signature
│   │   └── telegram_service.go                 # Return *TelegramUserAuth
│   ├── handlers/
│   │   └── telegram_handler.go                 # Type change (compatible)
│   ├── middleware/
│   │   └── pipeline_auth_test.go               # Add TestInternalAuthMiddleware
│   └── testutil/
│       ├── database.go                         # Add TelegramLink to allModels
│       └── fixtures.go                         # Add CreateTestTelegramLink
```

### Modified Files — Bot

```
apps/bot/
├── handlers/
│   ├── transaction_flow.py                     # try/except on create_transaction
│   └── balance.py                              # Fix hardcoded USD
└── utils/
    └── formatting.py                           # Add debt account type
```

### Modified Files — Frontend

```
apps/web/src/
├── hooks/
│   └── use-telegram.ts                         # Fix id/user_id types to string
└── components/
    └── settings/
        └── telegram-settings.tsx               # Add dark: color variants
```

---

## Implementation Order

```
Phase 1: Fix Migration Indexes
  1.1  Update migration 000018 — UNIQUE indexes
  1.2  Verify build

Phase 2: Typed Return Struct
  2.1  Define TelegramUserAuth struct in interfaces.go
  2.2  Update TelegramServicer interface signature
  2.3  Update service implementation
  2.4  Verify handler compatibility
  2.5  Verify build

Phase 3: Add Test Coverage
  3.1  Add TelegramLink to testutil allModels
  3.2  Add CreateTestTelegramLink fixture
  3.3  Write telegram_service_test.go
  3.4  Write telegram_handler_test.go
  3.5  Add TestInternalAuthMiddleware to pipeline_auth_test.go
  3.6  Run ./scripts/check-go.sh apps/api

Phase 4: Bot Fixes
  4.1  Wrap create_transaction in try/except
  4.2  Fix hardcoded USD in balance total
  4.3  Add debt account type to formatting
  4.4  Manual verification

Phase 5: Frontend Fixes
  5.1  Fix TelegramLink TypeScript types
  5.2  Fix dark mode colors
  5.3  Verify pnpm build

Phase 6: Final Verification
  6.1  ./scripts/check-go.sh apps/api
  6.2  pnpm build from apps/web/
  6.3  Verification checklist
```

## Verification

**Go backend** — after each code change:
```bash
cd apps/api && go build ./...
```
After completing Phase 3:
```bash
./scripts/check-go.sh apps/api
```

**Frontend** — after completing Phase 5:
```bash
cd apps/web && pnpm build
```
