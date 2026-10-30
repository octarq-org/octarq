package agentstore

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// ErrModelPriceMissing is returned when a turn names a model that has no
	// price in the workspace. The billing path refuses to invent a cost.
	ErrModelPriceMissing = errors.New("agentstore: no price configured for model")
	// ErrModelPriceNotFound is returned when deleting a price that does not exist.
	ErrModelPriceNotFound = errors.New("agentstore: model price not found")
	// ErrAgentPaused is returned when a turn targets a paused agent.
	ErrAgentPaused = errors.New("agentstore: agent is paused")
	// ErrBudgetExceeded is returned when auto-halt is on and the agent has
	// spent its monthly limit.
	ErrBudgetExceeded = errors.New("agentstore: agent budget is exhausted")
	// ErrInvalidSpend is returned for a spend event with a bad model label,
	// negative token counts, or an unknown source.
	ErrInvalidSpend = errors.New("agentstore: invalid spend event")
	// ErrInvalidPrice is returned for a price with an empty or over-long model
	// label, or a negative or non-finite amount.
	ErrInvalidPrice = errors.New("agentstore: invalid model price")
)

// priceTokenUnit is the token count a price row is quoted for: prices are per
// million tokens.
const priceTokenUnit = 1_000_000

// spendPrecision is the number of decimal places kept on an accrued cost. The
// ledger column stores six decimals; the agent counter keeps the same scale so
// the two never drift apart by rounding.
const spendPrecision = 1e6

// SpendInput is one reported agent turn. The caller reports usage; the store
// prices it, writes the ledger row, and accrues the agent budget.
type SpendInput struct {
	AgentID         string
	Model           string
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	Source          string
	SessionID       string
}

// monthStart returns the first instant of t's calendar month in UTC.
func monthStart(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func roundSpend(v float64) float64 {
	return math.Round(v*spendPrecision) / spendPrecision
}

// turnCost prices the token counts of one turn. Reasoning tokens are already
// part of OutputTokens on every supported provider, so they are not billed
// twice.
func turnCost(price models.WorkforceModelPrice, in, out int) float64 {
	cost := float64(in)*price.InputPerMTok/priceTokenUnit +
		float64(out)*price.OutputPerMTok/priceTokenUnit
	return roundSpend(cost)
}

// ListModelPrices returns every priced model of the workspace.
func (s *WorkforceStore) ListModelPrices(ctx context.Context, orgID uint) ([]models.WorkforceModelPrice, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	var prices []models.WorkforceModelPrice
	if err := s.db.WithContext(ctx).
		Where("owner_id = ?", orgID).
		Order("model ASC").
		Find(&prices).Error; err != nil {
		return nil, err
	}
	return prices, nil
}

// SetModelPrice creates or replaces the workspace price of one model.
func (s *WorkforceStore) SetModelPrice(ctx context.Context, orgID uint, model string, inputPerMTok, outputPerMTok float64) (*models.WorkforceModelPrice, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	model = strings.TrimSpace(model)
	if model == "" || len(model) > 64 {
		return nil, ErrInvalidPrice
	}
	if !validPrice(inputPerMTok) || !validPrice(outputPerMTok) {
		return nil, ErrInvalidPrice
	}

	now := time.Now().UTC()
	row := models.WorkforceModelPrice{
		OrgID:         orgID,
		Model:         model,
		InputPerMTok:  roundPrice(inputPerMTok),
		OutputPerMTok: roundPrice(outputPerMTok),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "owner_id"}, {Name: "model"}},
			DoUpdates: clause.AssignmentColumns([]string{"input_per_mtok", "output_per_mtok", "updated_at"})}).
		Create(&row).Error; err != nil {
		return nil, err
	}

	stored, err := s.priceFor(ctx, orgID, model)
	if err != nil {
		return nil, err
	}
	return stored, nil
}

// DeleteModelPrice removes the workspace price of one model.
func (s *WorkforceStore) DeleteModelPrice(ctx context.Context, orgID uint, model string) error {
	if s == nil || s.db == nil {
		return errors.New("agentstore: workforce store is not configured")
	}

	res := s.db.WithContext(ctx).
		Where("owner_id = ? AND model = ?", orgID, strings.TrimSpace(model)).
		Delete(&models.WorkforceModelPrice{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrModelPriceNotFound
	}
	return nil
}

// maxPricePerMTok is the largest price the column holds: decimal(12,4) keeps
// four decimals and eight integer digits.
const maxPricePerMTok = 99_999_999.9999

func validPrice(v float64) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > maxPricePerMTok {
		return false
	}
	// A non-zero price below the column scale would be stored as zero, making
	// the model free while the workspace believes it is priced. Refuse it.
	return v == 0 || roundPrice(v) > 0
}

func roundPrice(v float64) float64 {
	return math.Round(v*1e4) / 1e4
}

// priceFor reads the price row of a model, failing closed when the workspace
// never configured one.
func (s *WorkforceStore) priceFor(ctx context.Context, orgID uint, model string) (*models.WorkforceModelPrice, error) {
	return priceForDB(s.db.WithContext(ctx), orgID, model)
}

func priceForDB(db *gorm.DB, orgID uint, model string) (*models.WorkforceModelPrice, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, ErrModelPriceMissing
	}
	var row models.WorkforceModelPrice
	err := db.Where("owner_id = ? AND model = ?", orgID, model).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrModelPriceMissing
		}
		return nil, err
	}
	return &row, nil
}

// AuthorizeTurn decides whether an agent may run one turn on a model. The
// answer is fail-closed: an unknown agent, a paused agent, an unpriced model,
// or an exhausted budget with auto-halt on all refuse the turn.
//
// An exhausted budget whose period has rolled over is not exhausted: the
// counter covers the current calendar month only.
func (s *WorkforceStore) AuthorizeTurn(ctx context.Context, orgID uint, agentID, model string) (*models.WorkforceAgent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}

	agent, err := s.GetAgent(ctx, orgID, agentID)
	if err != nil {
		return nil, err
	}
	if agent.Status == "paused" {
		return nil, ErrAgentPaused
	}
	if _, err := s.priceFor(ctx, orgID, model); err != nil {
		return nil, err
	}

	autoHalt, err := s.GetAutoHalt(ctx, orgID)
	if err != nil {
		// An unreadable governance switch must not be read as "halt is off".
		return nil, err
	}
	if autoHalt && budgetExhausted(agent, time.Now().UTC()) {
		return nil, ErrBudgetExceeded
	}
	return agent, nil
}

// budgetExhausted reports whether the agent has spent its monthly limit.
func budgetExhausted(agent *models.WorkforceAgent, now time.Time) bool {
	used := agent.BudgetUsed
	if stalePeriod(agent.BudgetPeriodStart, now) {
		// The counter belongs to an earlier month; this month starts at zero.
		used = 0
	}
	return used >= agent.BudgetLimit
}

// stalePeriod reports whether a counter belongs to an earlier month. A zero
// period means the counter was never stamped, and its spend is charged to the
// current month — the fail-closed reading.
func stalePeriod(periodStart, now time.Time) bool {
	return !periodStart.IsZero() && periodStart.Before(monthStart(now))
}

// normalizeBudget projects a stored counter from an earlier month to zero.
// The database row is rewritten by the next spend event; until then, every
// reader sees this month's number.
func normalizeBudget(agent *models.WorkforceAgent, now time.Time) {
	if agent != nil && stalePeriod(agent.BudgetPeriodStart, now) {
		agent.BudgetUsed = 0
	}
}

// RecordSpend prices one reported turn, appends it to the ledger, and accrues
// the agent budget in one transaction. When auto-halt is on and the new total
// reaches the limit, the same transaction pauses the agent.
//
// A turn is recorded even when it failed halfway: the provider bills the
// tokens it generated, so the ledger must show them.
func (s *WorkforceStore) RecordSpend(ctx context.Context, orgID uint, in SpendInput) (*models.WorkforceSpendEvent, *models.WorkforceAgent, error) {
	if s == nil || s.db == nil {
		return nil, nil, errors.New("agentstore: workforce store is not configured")
	}
	// The model label is stored as the price row spells it: a surrounding
	// space would find the price but ledger a label that never matches it
	// again. An over-long label would fail the column bound on a server that
	// enforces it, so it is refused here as a client error instead.
	model := strings.TrimSpace(in.Model)
	if strings.TrimSpace(in.AgentID) == "" || model == "" || len(model) > 64 {
		return nil, nil, ErrInvalidSpend
	}
	if in.InputTokens < 0 || in.OutputTokens < 0 || in.ReasoningTokens < 0 {
		return nil, nil, ErrInvalidSpend
	}
	if !models.InStatusSet(in.Source, models.WorkforceSpendSources) {
		return nil, nil, ErrInvalidSpend
	}

	autoHalt, err := s.GetAutoHalt(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	period := monthStart(now)

	var event models.WorkforceSpendEvent
	var updated models.WorkforceAgent
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		agent := models.WorkforceAgent{}
		if err := tx.Where("id = ? AND owner_id = ?", in.AgentID, orgID).First(&agent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAgentNotFound
			}
			return err
		}

		price, err := priceForDB(tx, orgID, model)
		if err != nil {
			return err
		}

		cost := turnCost(*price, in.InputTokens, in.OutputTokens)

		event = models.WorkforceSpendEvent{
			OrgID:           orgID,
			AgentID:         in.AgentID,
			Model:           model,
			InputTokens:     in.InputTokens,
			OutputTokens:    in.OutputTokens,
			ReasoningTokens: in.ReasoningTokens,
			CostUSD:         cost,
			Source:          in.Source,
			SessionID:       in.SessionID,
			CreatedAt:       now,
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}

		// The accrual is one statement on the stored counter, never a
		// read-modify-write: two turns that finish together must both land.
		// A counter stamped with an earlier period restarts at this turn's
		// cost instead of adding to last month's total.
		if err := tx.Model(&models.WorkforceAgent{}).
			Where("id = ? AND owner_id = ?", in.AgentID, orgID).
			Updates(map[string]any{
				"budget_used": gorm.Expr(
					"CASE WHEN budget_period_start IS NULL OR budget_period_start < ? THEN ? ELSE budget_used + ? END",
					period, cost, cost),
				"budget_period_start": period,
				"updated_at":          now,
			}).Error; err != nil {
			return err
		}

		// Read the counter back after the write: the pause decision must see
		// this turn's cost, not the value read before it.
		if err := tx.Where("id = ? AND owner_id = ?", in.AgentID, orgID).First(&updated).Error; err != nil {
			return err
		}
		if autoHalt && updated.Status != "paused" && updated.BudgetUsed >= updated.BudgetLimit {
			if err := tx.Model(&models.WorkforceAgent{}).
				Where("id = ? AND owner_id = ?", in.AgentID, orgID).
				Update("status", "paused").Error; err != nil {
				return err
			}
			updated.Status = "paused"
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &event, &updated, nil
}

// ListSpendEvents returns the newest ledger rows of one agent, newest first.
// It refuses an unknown agent so a caller sees a 404 instead of an empty page
// that looks like "no spend".
func (s *WorkforceStore) ListSpendEvents(ctx context.Context, orgID uint, agentID string, limit int) ([]models.WorkforceSpendEvent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: workforce store is not configured")
	}
	if _, err := s.GetAgent(ctx, orgID, agentID); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	var events []models.WorkforceSpendEvent
	if err := s.db.WithContext(ctx).
		Where("owner_id = ? AND agent_id = ?", orgID, agentID).
		Order("id DESC").
		Limit(limit).
		Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}
