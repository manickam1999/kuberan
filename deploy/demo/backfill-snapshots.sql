-- Sample net-worth history for the demo user, so the dashboard chart is not
-- empty. In production the oracle records one snapshot a day; the demo has no
-- oracle, so reset.sh asks the api for today's snapshot and this file derives
-- the previous 90 days from it with a gentle sample curve (cash drifts, the
-- investments wobble more). It is demo data, not a calculation of anything.
--
-- Usage: psql -d <db> -v email=<demo login> -f backfill-snapshots.sql
WITH latest AS (
    SELECT s.*
    FROM portfolio_snapshots s
    JOIN users u ON u.id = s.user_id
    WHERE u.email = :'email'
    ORDER BY s.recorded_at DESC
    LIMIT 1
), curve AS (
    SELECT l.user_id, l.recorded_at, l.currency, l.fx_missing, n,
           l.cash_balance, l.investment_value, l.debt_balance, l.fixed_deposit_balance,
           1 - 0.0010 * n + 0.004 * sin(n / 5.0) AS fc,
           1 - 0.0016 * n + 0.030 * sin(n / 3.0) AS fi
    FROM latest l, generate_series(1, 90) AS n
)
INSERT INTO portfolio_snapshots
    (user_id, recorded_at, total_net_worth, cash_balance, investment_value,
     debt_balance, fixed_deposit_balance, currency, fx_missing)
SELECT user_id,
       recorded_at - make_interval(days => n),
       round(cash_balance * fc + investment_value * fi + fixed_deposit_balance - debt_balance * fc)::bigint,
       round(cash_balance * fc)::bigint,
       round(investment_value * fi)::bigint,
       round(debt_balance * fc)::bigint,
       fixed_deposit_balance,
       currency,
       fx_missing
FROM curve
ON CONFLICT (user_id, recorded_at) DO NOTHING;
