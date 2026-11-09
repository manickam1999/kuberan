// Package ibkr is a pure (no database, no I/O beyond the supplied reader)
// parser for Interactive Brokers Activity Statement CSV files.
//
// Parse reads one multi-section CSV into a Statement. BuildEvents validates a
// set of statements (same account and base currency, contiguous periods) and
// turns them into one ordered, normalised list of Events plus the per-statement
// Checkpoints (Open Positions summary and Ending Cash) that a caller can use to
// verify a replay of the events.
//
// Money is parsed exactly with math/big.Rat and rounded to cents once, half
// away from zero. Quantities are float64 rounded to 10 decimal places. Columns
// are always looked up by header name, never by position, and unsupported
// sections, asset categories and corporate actions fail loudly instead of being
// silently dropped.
//
// The package never logs and never echoes the account holder's name.
package ibkr
