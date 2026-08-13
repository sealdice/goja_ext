// Package sqlite provides a CGO-free SQLite module for Goja.
//
// The sqlite and node:sqlite module names expose the synchronous
// DatabaseSync, StatementSync, and TagStore APIs. Database files and rollback
// journals are opened through the runtime's shared fs.Core and its Afero
// backend. Connections are intended for one Goja runtime and one connection;
// the custom VFS deliberately does not provide SQLite shared-memory support,
// so WAL databases are rejected or remain on SQLite's rollback journal mode.
package sqlite
