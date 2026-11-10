package services

import (
	"sort"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

const (
	// ruleSuggestionWindow is how far back suggestions look.
	ruleSuggestionWindow = 90 * 24 * time.Hour
	// ruleSuggestionMinCount is the fewest identical rows that make a pattern.
	ruleSuggestionMinCount = 3
	// ruleSuggestionLimit caps the suggestions returned.
	ruleSuggestionLimit = 10
)

// RuleSuggestion proposes a rule: Count transactions in the last 90 days share
// the normalised description DescriptionPattern (lowercase, whitespace
// collapsed), the same CategoryID and the same Type, and no rule covers them.
type RuleSuggestion struct {
	DescriptionPattern string                 `json:"description_pattern"`
	CategoryID         string                 `json:"category_id"`
	Type               models.TransactionType `json:"type"`
	Count              int                    `json:"count"`
}

type suggestionKey struct {
	description string
	categoryID  string
	txType      models.TransactionType
}

type suggestionGroup struct {
	count   int
	covered bool
	// firstSeq is the index of the newest row (rows arrive newest first), used
	// to break ties between equally frequent patterns.
	firstSeq int
}

// SuggestRules finds repeated manual categorizations worth turning into a rule:
// at least three income/expense rows in the last 90 days sharing a normalised
// description and a live category of the matching type, where no active rule's
// conditions match any of them. Nothing records past recategorisations, so the
// suggestion is defined on the rows as they stand. Most frequent first.
func (s *ruleService) SuggestRules(userID string) ([]RuleSuggestion, error) {
	rules, err := s.ListRules(userID)
	if err != nil {
		return nil, err
	}
	active := make([]models.TransactionRule, 0, len(rules))
	for i := range rules {
		if rules[i].IsActive {
			active = append(active, rules[i])
		}
	}

	var categories []models.Category
	if err := s.db.Select("id", "type").Where("user_id = ?", userID).Find(&categories).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	categoryType := make(map[string]models.CategoryType, len(categories))
	for i := range categories {
		categoryType[categories[i].ID] = categories[i].Type
	}

	var rows []ruleCandidate
	if err := s.db.Model(&models.Transaction{}).
		Select("id", "account_id", "category_id", "type", "amount", "description").
		Where("user_id = ? AND type IN ? AND category_id IS NOT NULL AND date >= ?", userID,
			[]models.TransactionType{models.TransactionTypeIncome, models.TransactionTypeExpense},
			time.Now().Add(-ruleSuggestionWindow)).
		Order("date DESC, created_at DESC").
		Scan(&rows).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	groups := make(map[suggestionKey]*suggestionGroup)
	for i := range rows {
		r := &rows[i]
		desc := models.NormalizeCounterpartyKey(r.Description)
		if desc == "" || r.CategoryID == nil {
			continue
		}
		catType, ok := categoryType[*r.CategoryID]
		if !ok || !categoryTypeMatchesTxType(catType, r.Type) {
			continue
		}
		key := suggestionKey{description: desc, categoryID: *r.CategoryID, txType: r.Type}
		g := groups[key]
		if g == nil {
			g = &suggestionGroup{firstSeq: i}
			groups[key] = g
		}
		g.count++
		if !g.covered {
			in := txToRuleInput(r)
			for j := range active {
				if ConditionsMatch(active[j].Conditions, in) {
					g.covered = true
					break
				}
			}
		}
	}

	type ranked struct {
		RuleSuggestion
		seq int
	}
	found := make([]ranked, 0)
	for key, g := range groups {
		if g.count < ruleSuggestionMinCount || g.covered {
			continue
		}
		found = append(found, ranked{
			RuleSuggestion: RuleSuggestion{
				DescriptionPattern: key.description,
				CategoryID:         key.categoryID,
				Type:               key.txType,
				Count:              g.count,
			},
			seq: g.firstSeq,
		})
	}
	sort.Slice(found, func(a, b int) bool {
		if found[a].Count != found[b].Count {
			return found[a].Count > found[b].Count
		}
		return found[a].seq < found[b].seq
	})
	if len(found) > ruleSuggestionLimit {
		found = found[:ruleSuggestionLimit]
	}
	out := make([]RuleSuggestion, len(found))
	for i := range found {
		out[i] = found[i].RuleSuggestion
	}
	return out, nil
}
