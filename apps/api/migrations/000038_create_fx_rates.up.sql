-- Exchange-rate time series (plan 024). One row per observation of how many
-- units of `quote` one unit of `base` is worth; immutable, like security_prices.
CREATE TABLE IF NOT EXISTS fx_rates (
    id          UUID PRIMARY KEY,
    base        VARCHAR(3)  NOT NULL,
    quote       VARCHAR(3)  NOT NULL,
    rate        NUMERIC(20,10) NOT NULL CHECK (rate > 0),
    recorded_at TIMESTAMPTZ NOT NULL,
    source      VARCHAR(32) NOT NULL DEFAULT 'yahoo',
    CONSTRAINT uq_fx_rates_pair_recorded UNIQUE (base, quote, recorded_at),
    CONSTRAINT fx_rates_distinct_pair CHECK (base <> quote)
);

CREATE INDEX IF NOT EXISTS idx_fx_rates_pair_recorded_desc ON fx_rates (base, quote, recorded_at DESC);
