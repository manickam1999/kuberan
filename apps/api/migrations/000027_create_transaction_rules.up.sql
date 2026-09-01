-- Transaction rules engine + auto-categorization (plan 018).
-- A rule = conditions (AND-ed) -> actions. Separate rules act as OR. On create,
-- the first active rule (by priority ASC, created_at ASC) whose conditions all
-- match and whose set_category target is valid assigns the transaction's category.
CREATE TABLE IF NOT EXISTS transaction_rules (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    user_id     UUID NOT NULL REFERENCES users(id),
    name        VARCHAR(120) NOT NULL,
    priority    INT NOT NULL DEFAULT 0,          -- lower = evaluated first
    is_active   BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE INDEX IF NOT EXISTS idx_transaction_rules_user_active_priority
    ON transaction_rules (user_id, is_active, priority);
CREATE INDEX IF NOT EXISTS idx_transaction_rules_deleted_at
    ON transaction_rules (deleted_at);

-- Conditions are wholly-owned children of a rule, replaced (hard delete + reinsert)
-- on rule update. ON DELETE CASCADE is the hard-delete safety net.
CREATE TABLE IF NOT EXISTS transaction_rule_conditions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    rule_id     UUID NOT NULL REFERENCES transaction_rules(id) ON DELETE CASCADE,
    field       VARCHAR(24) NOT NULL,            -- description | amount | account_id | type
    operator    VARCHAR(16) NOT NULL,            -- see plan 018 operator matrix
    value_text  VARCHAR(500),                    -- text / account uuid / type value
    amount_min  BIGINT,                          -- cents (gt, between)
    amount_max  BIGINT,                          -- cents (lt, between)
    CONSTRAINT trc_field_check CHECK (field IN ('description', 'amount', 'account_id', 'type')),
    CONSTRAINT trc_operator_check CHECK (operator IN
        ('contains', 'not_contains', 'equals', 'starts_with', 'ends_with', 'gt', 'lt', 'between'))
);

CREATE INDEX IF NOT EXISTS idx_transaction_rule_conditions_rule_id
    ON transaction_rule_conditions (rule_id);

-- Actions are wholly-owned children of a rule (symmetric with conditions), so future
-- action types (add_tag, rename, hide) are additive action_type values, not a schema
-- migration on populated data. v1 supports only set_category.
CREATE TABLE IF NOT EXISTS transaction_rule_actions (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ,
    rule_id      UUID NOT NULL REFERENCES transaction_rules(id) ON DELETE CASCADE,
    action_type  VARCHAR(24) NOT NULL,           -- v1: 'set_category'
    category_id  UUID REFERENCES categories(id), -- for set_category
    value_text   VARCHAR(500),                   -- reserved for future actions
    CONSTRAINT tra_action_check CHECK (action_type IN ('set_category'))
);

CREATE INDEX IF NOT EXISTS idx_transaction_rule_actions_rule_id
    ON transaction_rule_actions (rule_id);
