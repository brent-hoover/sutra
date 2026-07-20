package store

import "fmt"

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
