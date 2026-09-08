package store

import "database/sql"

// DB exposes the underlying handle for Llama Hugs extension packages
// (internal/hugs). Read/write access is governed by those packages' own
// queries; this is a deliberate narrow bridge added by the fork.
func (s *Store) DB() *sql.DB {
	return s.db
}
