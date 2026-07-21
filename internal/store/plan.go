package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/brent-hoover/sutra/internal/domain"
)

// migrateApproval adds the additive `approval` column to issues. It is empty for
// non-plan issues and holds pending|approved for plan issues. Registered from
// migrate(); safe to run repeatedly (addColumnIfMissing is a no-op once present),
// and it back-fills a freshly-created issues table that omits the column.
func (s *Store) migrateApproval() error {
	if err := s.addColumnIfMissing("issues", "approval", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate issues approval: %w", err)
	}
	return nil
}

// BuildPlan inserts the plan issue, its tracer children, and the sequential
// blocking chain (child i blocks child i+1) in one transaction, along with the
// supplied ledger entries. If plan.ParentID is set it must exist; the plan's
// project is inherited from that parent when plan.ProjectID is nil, and the
// resolved project is stamped on the plan and every child. Child order comes from
// the slice order: BuildPlan stamps sortable child timestamps before persisting
// so ListIssues returns tracers in run order. Any failure rolls the whole tree
// back, so a partial plan is never visible. The caller is responsible for setting
// each child's ParentID to the plan's id.
func (s *Store) BuildPlan(plan domain.Issue, children []domain.Issue, ledger []domain.LedgerEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Resolve the parent (if any) inside the tx so a concurrent change can't
	// dangle the link or race the inherited project.
	var parentProject sql.NullString
	if plan.ParentID != nil {
		err := tx.QueryRow(`SELECT project_id FROM issues WHERE id = ?`, *plan.ParentID).Scan(&parentProject)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lookup plan parent: %w", err)
		}
		if plan.ProjectID == nil && parentProject.Valid {
			pid := parentProject.String
			plan.ProjectID = &pid
		}
	}
	if plan.ProjectID != nil {
		if err := existsInTx(tx, "projects", *plan.ProjectID); err != nil {
			return err
		}
	}
	if parentProject.Valid && plan.ProjectID != nil && *plan.ProjectID != parentProject.String {
		return errors.Join(domain.ErrInvalidIssue, errors.New("plan project does not match parent project"))
	}

	if err := insertIssueTx(tx, plan); err != nil {
		return err
	}
	childBase := plan.CreatedAt.UTC().Truncate(time.Second)
	for i := range children {
		childAt := childBase.Add(time.Duration(i+1) * time.Second)
		children[i].CreatedAt = childAt
		children[i].UpdatedAt = childAt
		children[i].ProjectID = plan.ProjectID
		if err := insertIssueTx(tx, children[i]); err != nil {
			return err
		}
	}
	for i := 0; i+1 < len(children); i++ {
		if _, err := tx.Exec(
			`INSERT INTO issue_block (blocker_id, blocked_id) VALUES (?, ?)`,
			children[i].ID, children[i+1].ID,
		); err != nil {
			return fmt.Errorf("insert plan block edge: %w", err)
		}
	}

	if err := insertLedger(tx, ledger); err != nil {
		return err
	}
	return tx.Commit()
}

// ApprovePlan sets a plan issue's approval to approved and, in one transaction,
// advances updated_at and appends the ledger entry (stamped inside the tx).
// Returns ErrNotFound if the issue is missing and ErrInvalidIssue if it is not a
// plan issue. Approving an already-approved plan is an idempotent no-op: no
// updated_at bump and no ledger entry, matching the relation/block operations.
func (s *Store) ApprovePlan(id string, entry domain.LedgerEntry) (domain.Issue, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Issue{}, err
	}
	defer tx.Rollback()

	var typ, approval string
	err = tx.QueryRow(`SELECT type, approval FROM issues WHERE id = ?`, id).Scan(&typ, &approval)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Issue{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Issue{}, fmt.Errorf("lookup plan: %w", err)
	}
	if domain.IssueType(typ) != domain.TypePlan {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, errors.New("issue is not a plan"))
	}
	planApproval := domain.Approval(approval)
	if planApproval == domain.ApprovalApproved {
		_ = tx.Rollback() // release the connection before the read-back
		return s.GetIssue(id)
	}
	if planApproval != domain.ApprovalPending {
		return domain.Issue{}, errors.Join(domain.ErrInvalidIssue, fmt.Errorf("plan approval is %q", approval))
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(
		`UPDATE issues SET approval = ?, updated_at = ? WHERE id = ?`,
		string(domain.ApprovalApproved), now.Format(timeFmt), id,
	); err != nil {
		return domain.Issue{}, fmt.Errorf("approve plan: %w", err)
	}
	entry.At = now
	if err := insertLedger(tx, []domain.LedgerEntry{entry}); err != nil {
		return domain.Issue{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Issue{}, err
	}
	return s.GetIssue(id)
}
