# CSV import

Import a bank or card statement exported as CSV into one account. It follows the
IBKR importer (`docs/ibkr-import.md`): one endpoint, a dry run unless you say
`apply`, the file held in memory only (never stored, so there is no upload id),
counts-only logs, one database transaction on apply.

In the web app: **Settings → Data → Import a CSV**.

## Endpoint

`POST /api/v1/imports/csv`, `multipart/form-data`, Bearer token.

| Field | Required | Meaning |
|---|---|---|
| `file` | yes | The CSV, UTF-8, a header row first, at most 5,000 data rows and `MAX_UPLOAD_BYTES` (10 MiB). |
| `account_id` | yes | The cash, credit-card or debt account the rows go into. Amounts are in that account's currency. |
| `date_column` | with a mapping | Header of the date column. |
| `description_column` | with a mapping | Header of the description column. |
| `amount_column` | one of | Header of a signed amount column. |
| `debit_column` + `credit_column` | one of | Headers of the money-out and money-in columns (both). |
| `category_column` | no | Header of a category column. |
| `notes_column` | no | Header of a notes column. |
| `date_format` | no | One of `YYYY-MM-DD`, `YYYY/MM/DD`, `YYYYMMDD`, `DD/MM/YYYY`, `MM/DD/YYYY`, `DD-MM-YYYY`, `DD.MM.YYYY`, `DD/MM/YY`, `MM/DD/YY`, `DD MMM YYYY`, `DD-MMM-YYYY`, `MMM DD, YYYY`. Defaults to the suggested format. |
| `decimal_separator` | no | `.` or `,`. Defaults to the suggested one. |
| `amount_sign` | no | `negative_is_expense` (default, bank statements) or `positive_is_expense` (card statements). Only for `amount_column`. |
| `apply` | no | `true` writes; anything else is a dry run. Needs a mapping. |

Columns are matched by header name, exactly first, then ignoring case. The route is
exempt from the 1 MiB JSON body limit (like `/imports/ibkr` and the attachment
upload) and enforces `MAX_UPLOAD_BYTES` plus a small allowance for the form fields.

## Two steps

1. **Inspect.** Send only `file` and `account_id`. The report carries the
   detected `delimiter` (comma, semicolon, tab or pipe), the `columns`, the first
   five rows as `sample`, and `suggested_mapping`: the columns guessed from the
   header names, the date format that parses every sampled date, a decimal comma
   when the amounts look like `12,50`, and, for a card or debt account, whether
   positive amounts are the spending.
2. **Dry run, then apply.** Send the mapping fields. The report adds `summary`
   and `rows` (up to 200, problems first). The summary counts what would be
   created (`would_create`), what repeats an earlier import (`skip_duplicate`),
   what matches an entry you typed by hand (`skip_manual`), what could not be read
   (`invalid`), how many get a category and from which source, and the income and
   expense totals of the rows to create. Repeat the call with `apply=true` to
   write them. The result has `applied: true` and `created`.

An apply is refused with `400 IMPORT_INVALID` (the report is in `error.details`)
only when no row at all can be read. Otherwise unreadable rows are skipped and
listed in `issues` (line, code, detail), so read the dry run first.

## What gets written

- One transaction per new row, `source = 'import'`, income or expense, dated at
  noon UTC of the statement date, in the account's currency, with the
  description (and notes, if mapped) cleaned of control characters.
- The account balance moves once by the sum of the new rows, using the account
  type's sign rules (a purchase on a credit card raises the debt).
- Unlike IBKR-imported rows, whose amount, type and account are locked, CSV rows
  are ordinary entries and can be edited or deleted. Editing a row's date or
  description changes what a re-import would hash, so delete rather than edit
  when you plan to re-import the same file.
- Not written: transfers (a statement cannot say which of your accounts the other
  leg is) and opening balances. There is no currency conversion: the amounts are
  taken as the account's currency, so import a USD statement into a USD account.

## Categories and rules

For each row to create:

1. A category cell that names one of your categories (case-insensitive, of the
   matching income or expense type) wins.
2. Otherwise your transaction rules decide. They are loaded once for the whole
   file and run through the same pure matcher (`Match`, `services/rule_matcher.go`)
   that the rest of the app uses, so the first active rule that matches and
   targets a valid category assigns it.
3. Otherwise the row stays uncategorised. A category cell that matches nothing is
   counted as `unknown_category` and the rule step still runs.

## Idempotence and duplicates

- Every row gets `import_ref = "csv:<account id>:<row hash>"`, the column added in
  migration `000044`. The hash is the SHA-256 of the date, the signed amount, the
  normalised description (lower case, letters and digits only) and how many
  identical rows came before it in the file, truncated to 32 hex characters.
  Importing the same file again creates nothing (`skip_duplicate`), and two
  identical purchases on one day stay two rows.
- Rows that match an entry you typed by hand are skipped too (`skip_manual`):
  same account, same type and amount, a date within one day, and the same
  normalised description or one containing the other (statements append
  references). Each hand-typed entry absorbs at most one statement row.
- Overlapping exports of different ranges can still double an identical purchase
  whose position in the file changed; the manual-entry match does not apply
  between two imports. Import non-overlapping ranges, or delete the extra rows.

## Parsing rules

- UTF-8 only (a leading BOM is removed). Blank lines are skipped. Quoted fields
  and a delimiter inside quotes work.
- Amounts accept currency symbols and codes, spaces and thousands separators,
  parentheses or a leading or trailing minus for negatives, and a trailing `CR`
  (money in) or `DR` (money out). More than two decimals is refused unless the
  extra digits are zeros. A zero amount is invalid.
- With debit and credit columns, a row with both filled is invalid.
- Dates may carry a time of day, which is ignored. Years outside 1970 to next
  year are refused.
- Limits: 5,000 data rows (`400 CSV_TOO_MANY_ROWS`), 100 columns, 500 characters
  per description, 2,000 per note.

## Errors

| Code | Status | When |
|---|---|---|
| `INVALID_INPUT` | 400 | Not multipart, no file, bad `account_id`, unknown column, bad format, no header or data rows, not UTF-8, apply without a mapping, target not a cash, card or debt account. |
| `CSV_TOO_MANY_ROWS` | 400 | More than 5,000 data rows. |
| `IMPORT_INVALID` | 400 | Apply with no readable row (report in `error.details`). |
| `ACCOUNT_NOT_FOUND` | 404 | The account is not yours or inactive. |
| `IMPORT_IN_PROGRESS` | 409 | Another import of yours is being applied. |
| `PAYLOAD_TOO_LARGE` | 413 | The file is over `MAX_UPLOAD_BYTES`. |

Applied imports are audited as `IMPORT_CSV` with counts only (no descriptions or
amounts).
