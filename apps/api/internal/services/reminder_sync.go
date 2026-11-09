package services

import (
	"errors"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// Reminder lead times (plan 023, 5.6): how long before the due date the
// reminder becomes sendable, and the hour (UTC) it is sent at (09:00 in
// Malaysia, the app's home market).
const (
	cardReminderLeadDays = 3
	loanReminderLeadDays = 3
	fdReminderLeadDays   = 7
	reminderSendHourUTC  = 1
)

// NextDueDate returns the first date on or after from (compared by UTC date)
// that falls on dueDay (1-31) of its month. Months with fewer days use their
// last day, so dueDay 31 gives Feb 28/29, Apr 30 and so on.
func NextDueDate(dueDay int, from time.Time) time.Time {
	from = utcDay(from)
	for i := 0; i < 2; i++ {
		first := time.Date(from.Year(), from.Month()+time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		day := dueDay
		if day > last {
			day = last
		}
		candidate := time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC)
		if !candidate.Before(from) {
			return candidate
		}
	}
	// Unreachable: the second month's candidate is always after from.
	return from
}

// reminderRemindAt is when the reminder for a due date becomes sendable.
func reminderRemindAt(dueOn time.Time, leadDays int) time.Time {
	d := utcDay(dueOn).AddDate(0, 0, -leadDays)
	return d.Add(reminderSendHourUTC * time.Hour)
}

// isRecurringReminder reports whether a kind repeats every month (the next
// occurrence is created once the current one has been sent).
func isRecurringReminder(k models.ReminderKind) bool {
	return k == models.ReminderKindCardDue || k == models.ReminderKindLoanDue
}

// reminderPlan is what an account wants: the kind and the first due date that
// still needs a reminder, or kind "" for none.
type reminderPlan struct {
	kind     models.ReminderKind
	dueOn    time.Time
	leadDays int
}

// planAccountReminder works out the reminder an account should have as of now.
// after, when non-zero, is the due date of the latest reminder already sent:
// the plan then starts the day after it, so a sent occurrence is not repeated.
func planAccountReminder(acct *models.Account, fd *models.FixedDepositDetails, now, after time.Time) reminderPlan {
	if !acct.IsActive {
		return reminderPlan{}
	}
	from := utcDay(now)
	if !after.IsZero() && !utcDay(after).Before(from) {
		from = utcDay(after).AddDate(0, 0, 1)
	}
	switch acct.Type {
	case models.AccountTypeCreditCard, models.AccountTypeDebt:
		if acct.DueDay == nil || *acct.DueDay < 1 || *acct.DueDay > 31 {
			return reminderPlan{}
		}
		kind, lead := models.ReminderKindCardDue, cardReminderLeadDays
		if acct.Type == models.AccountTypeDebt {
			kind, lead = models.ReminderKindLoanDue, loanReminderLeadDays
		}
		return reminderPlan{kind: kind, dueOn: NextDueDate(*acct.DueDay, from), leadDays: lead}
	case models.AccountTypeFixedDeposit:
		if fd == nil || fd.Status != models.FixedDepositStatusActive {
			return reminderPlan{}
		}
		due := utcDay(fd.MaturityDate)
		if due.Before(from) {
			return reminderPlan{}
		}
		return reminderPlan{kind: models.ReminderKindFDMaturity, dueOn: due, leadDays: fdReminderLeadDays}
	default:
		return reminderPlan{}
	}
}

// syncAccountReminders brings the reminders of one account in line with its
// due_day / maturity date, inside tx. It removes unsent reminders that no
// longer match (due day changed, account deactivated or closed), keeps the
// ones that do, and creates the missing one. A reminder switched off for the
// account stays off across occurrences. Idempotent.
func syncAccountReminders(tx *gorm.DB, accountID string, now time.Time) error {
	var acct models.Account
	err := tx.Preload("FixedDeposit").Where("id = ?", accountID).First(&acct).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return deleteUnsentReminders(tx, accountID, nil)
	}
	if err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// The latest occurrence (for the on/off switch, which carries over) and the
	// latest one already sent (the plan starts after it).
	enabled := true
	var latest []models.Reminder
	if err := tx.Where("ref_id = ? AND user_id = ?", accountID, acct.UserID).
		Order("due_on DESC").Limit(1).Find(&latest).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if len(latest) > 0 {
		enabled = latest[0].Enabled
	}
	var after time.Time
	var sent []models.Reminder
	if err := tx.Where("ref_id = ? AND user_id = ? AND sent_at IS NOT NULL", accountID, acct.UserID).
		Order("due_on DESC").Limit(1).Find(&sent).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if len(sent) > 0 {
		after = sent[0].DueOn
	}

	plan := planAccountReminder(&acct, acct.FixedDeposit, now, after)
	if plan.kind == "" {
		return deleteUnsentReminders(tx, accountID, nil)
	}

	// Drop unsent reminders for any other occurrence or kind.
	if err := deleteUnsentReminders(tx, accountID, &plan.dueOn); err != nil {
		return err
	}
	var existing int64
	if err := tx.Model(&models.Reminder{}).
		Where("kind = ? AND ref_id = ? AND due_on = ?", plan.kind, accountID, plan.dueOn).
		Count(&existing).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if existing > 0 {
		return nil
	}
	rem := &models.Reminder{
		UserID:   acct.UserID,
		Kind:     plan.kind,
		RefID:    accountID,
		DueOn:    plan.dueOn,
		RemindAt: reminderRemindAt(plan.dueOn, plan.leadDays),
		Enabled:  enabled,
	}
	if err := tx.Create(rem).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if !enabled {
		// GORM writes the column default (true) for a zero-value false on insert.
		if err := tx.Model(rem).Update("enabled", false).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}
	return nil
}

// deleteUnsentReminders hard-deletes an account's unsent reminders, except
// the one due on keepDue (nil keeps none). They are hard
// deleted so the unique (kind, ref_id, due_on) index never blocks recreating
// the same occurrence later.
func deleteUnsentReminders(tx *gorm.DB, accountID string, keepDue *time.Time) error {
	q := tx.Unscoped().Where("ref_id = ? AND sent_at IS NULL", accountID)
	if keepDue != nil {
		q = q.Where("due_on <> ?", *keepDue)
	}
	if err := q.Delete(&models.Reminder{}).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}
