package models

import "time"

// AgentHarnessSpec is one version of a workspace's agent harness.
//
// A version is immutable once written: the spec document never changes, only
// its status does. A workspace has at most one **published** version — the
// one its assistant actually runs — and any number of drafts, archived
// versions and discarded drafts.
//
// The lifecycle is deliberately not "publish straight to live": a version is
// staged as a draft, a human reads the diff, and only then is it activated.
// That is the gate an offline lab proposal has to pass as well.
type AgentHarnessSpec struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// OrgID scopes the row to one workspace. The column is owner_id for
	// consistency with the rest of the tenant-scoped tables.
	OrgID uint   `gorm:"index:idx_agent_harness_org;uniqueIndex:idx_agent_harness_org_version;uniqueIndex:idx_agent_harness_live_slot;not null;column:owner_id" json:"orgId"`
	Name  string `gorm:"size:64;not null" json:"name"`
	// Version counts up within the workspace. The (workspace, version) pair is
	// unique: two admins publishing at the same moment must not produce two
	// versions with the same number.
	Version int `gorm:"uniqueIndex:idx_agent_harness_org_version;not null" json:"version"`
	// SpecJSON is the canonical spec document.
	SpecJSON string `gorm:"type:text;not null" json:"spec"`
	// Fingerprint is the SHA-256 of the canonical spec document.
	Fingerprint string `gorm:"size:64;index;not null" json:"fingerprint"`
	// Status is one of draft / published / archived / discarded.
	Status string `gorm:"size:16;not null;index" json:"status"`
	// LiveSlot marks the row that occupies the workspace's single live slot:
	// 1 for the live version, NULL for every other row.
	//
	// It exists so the database can refuse a second live version. A status
	// column cannot carry that rule — "published" is a value many rows may
	// hold — but a nullable column inside a unique index on
	// (owner_id, live_slot) can: NULLs are distinct on every supported
	// database, so any number of drafts may carry NULL while only one row may
	// carry 1.
	//
	// The column is written in the same statement as the status and is never
	// read to decide which version is live. The status is the truth; this is
	// the constraint.
	LiveSlot *int `gorm:"column:live_slot;uniqueIndex:idx_agent_harness_live_slot" json:"-"`
	// Evidence is the JSON document that justifies this version, when it came
	// from somewhere else — an offline lab proposal, for instance. Empty when
	// a human wrote the spec themselves.
	Evidence    string     `gorm:"type:text" json:"evidence"`
	CreatedBy   uint       `gorm:"not null" json:"createdBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	PublishedAt *time.Time `json:"publishedAt"`
}

func (AgentHarnessSpec) TableName() string {
	return "agent_harness_specs"
}

// Agent harness spec statuses.
const (
	// AgentHarnessStatusDraft is a staged version that is not live.
	AgentHarnessStatusDraft = "draft"
	// AgentHarnessStatusPublished is the live version of the workspace.
	AgentHarnessStatusPublished = "published"
	// AgentHarnessStatusArchived was live once and no longer is.
	AgentHarnessStatusArchived = "archived"
	// AgentHarnessStatusDiscarded is a draft an operator threw away.
	AgentHarnessStatusDiscarded = "discarded"
)

// AgentHarnessLesson is one thing the workspace learned about its agent.
//
// A lesson is a record with evidence, scoped to a workspace: what was
// observed, where, and what it came from. It never changes the harness on
// its own — an operator decides whether the lesson becomes a spec rule. The
// point is that the reasoning survives: the next proposal can cite the lesson
// it answers.
type AgentHarnessLesson struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// OrgID scopes the row to one workspace. The column is owner_id for
	// consistency with the rest of the tenant-scoped tables.
	OrgID uint `gorm:"index:idx_agent_harness_lesson_org;not null;column:owner_id" json:"orgId"`
	// Scope names the area the lesson applies to ("links", "billing", ...).
	Scope string `gorm:"size:64;not null" json:"scope"`
	// Note is the lesson itself, in one sentence.
	Note string `gorm:"type:text;not null" json:"note"`
	// Evidence is the JSON document that backs the lesson, if any.
	Evidence string `gorm:"type:text" json:"evidence"`
	// Status is suggested / adopted / dismissed.
	Status string `gorm:"size:16;not null;index" json:"status"`
	// SpecID is the harness version this lesson motivated, if any.
	SpecID    uint      `gorm:"index" json:"specId"`
	CreatedBy uint      `gorm:"not null" json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

func (AgentHarnessLesson) TableName() string {
	return "agent_harness_lessons"
}

// Agent harness lesson statuses.
const (
	// AgentHarnessLessonSuggested is a lesson nobody has acted on yet.
	AgentHarnessLessonSuggested = "suggested"
	// AgentHarnessLessonAdopted is a lesson that made it into a live version.
	AgentHarnessLessonAdopted = "adopted"
	// AgentHarnessLessonDismissed is a lesson an operator rejected.
	AgentHarnessLessonDismissed = "dismissed"
)
