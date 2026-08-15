package sqlite_test

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/fs"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/sqlite"
	"github.com/spf13/afero"
)

func TestNodeSQLiteModuleExecutesAgainstSharedFS(t *testing.T) {
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	backend := afero.NewMemMapFs()
	sqlite.RegisterWithOptions(registry, fs.WithFS(backend), fs.WithCwd("/workspace"))

	value, err := rt.RunString(`
		const { DatabaseSync } = require('node:sqlite')
		const db = new DatabaseSync('shared.db')
		db.exec('CREATE TABLE items (value TEXT)')
		db.prepare('INSERT INTO items VALUES (?)').run('ok')
		const row = db.prepare('SELECT value FROM items').get()
		if (row.value !== 'ok') throw new Error('unexpected row')
		db.close()
		row.value
	`)
	if err != nil {
		t.Fatalf("RunString() error = %v", err)
	}
	if got := value.String(); got != "ok" {
		t.Fatalf("result = %q, want %q", got, "ok")
	}
	if exists, err := afero.Exists(backend, "/workspace/shared.db"); err != nil || !exists {
		t.Fatalf("shared database exists = %v, err = %v", exists, err)
	}
}

func TestNodeSQLiteModuleCoreValuesAndParameters(t *testing.T) {
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	sqlite.RegisterWithOptions(registry)

	_, err := rt.RunString(`
		const { DatabaseSync } = require('sqlite')
		const db = new DatabaseSync(':memory:')
		db.exec('CREATE TABLE values_table (id INTEGER PRIMARY KEY, n INTEGER, b BLOB, label TEXT)')
		const insert = db.prepare('INSERT INTO values_table (n, b, label) VALUES (:n, ?, ?)')
		insert.run({ n: 9007199254740993n }, new Uint8Array([1, 2, 3]), 'ok')
		const statement = db.prepare('SELECT n, b, label FROM values_table WHERE n = :n')
		statement.setReadBigInts(true)
		const row = statement.get({ n: 9007199254740993n })
		if (row.n !== 9007199254740993n) throw new Error('bigint mismatch')
		if (!(row.b instanceof Uint8Array) || row.b.length !== 3 || row.b[1] !== 2) throw new Error('blob mismatch')
		if (row.label !== 'ok') throw new Error('text mismatch')
		const tags = db.createTagStore()
		const tagged = ['SELECT label FROM values_table WHERE id = ', '']
		if (tags.get(tagged, 1).label !== 'ok') throw new Error('tag mismatch')
		db.close()
	`)
	if err != nil {
		t.Fatalf("RunString() error = %v", err)
	}
}

func TestSQLiteReusesExistingFSCore(t *testing.T) {
	rt := goja.New()
	registry := new(require.Registry)
	backend := afero.NewMemMapFs()
	fs.RegisterWithOptions(registry, fs.WithFS(backend), fs.WithCwd("/workspace"))
	sqlite.RegisterWithRegistry(registry)
	registry.Enable(rt)

	_, err := rt.RunString(`
		const filesystem = require('fs')
		const { DatabaseSync } = require('node:sqlite')
		filesystem.writeTextFileSync('visible.txt', 'from fs')
		const db = new DatabaseSync('shared.db')
		db.exec('CREATE TABLE values_table (value TEXT)')
		db.prepare('INSERT INTO values_table VALUES (?)').run(filesystem.readTextFileSync('visible.txt'))
		if (db.prepare('SELECT value FROM values_table').get().value !== 'from fs') throw new Error('Core was not shared')
	`)
	if err != nil {
		t.Fatalf("RunString() error = %v", err)
	}
}

func TestNodeSQLiteModuleMetadataAndErrors(t *testing.T) {
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	sqlite.RegisterWithRegistry(registry)

	_, err := rt.RunString(`
		const { DatabaseSync } = require('node:sqlite')
		const codeOf = (fn) => { try { fn(); return '' } catch (err) { return err.code } }
		const deferred = new DatabaseSync(':memory:', { open: false })
		if (deferred.isOpen) throw new Error('deferred database is open')
		if (codeOf(() => deferred.exec('SELECT 1')) !== 'DATABASE_NOT_OPEN') throw new Error('closed exec code')
		deferred.open()
		if (codeOf(() => deferred.open()) !== 'DATABASE_ALREADY_OPEN') throw new Error('double open code')
		deferred.exec('CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)')
		const stmt = deferred.prepare('SELECT id AS alias, name, 1 + 1 AS expr FROM t WHERE id = :id')
		const columns = stmt.columns()
		if (columns[0].column !== 'id' || columns[0].name !== 'alias' || columns[0].database !== 'main' || columns[0].table !== 't' || columns[0].type !== 'INTEGER') throw new Error('column metadata')
		if (columns[2].column !== null || columns[2].database !== null || columns[2].table !== null || columns[2].type !== null) throw new Error('expression metadata')
		if (codeOf(() => stmt.get({ id: 1, extra: 2 })) !== 'INVALID_ARGUMENT') throw new Error('unknown named code')
		stmt.setAllowUnknownNamedParameters(true)
		if (stmt.get({ id: 1, extra: 2 }) !== undefined) throw new Error('empty query result')
		stmt.setAllowBareNamedParameters(false)
		if (stmt.get({ ':id': 1 }) !== undefined) throw new Error('sigiled named parameter')
		if (codeOf(() => stmt.get({ id: 1 })) !== 'INVALID_ARGUMENT') throw new Error('bare named code')
		if (codeOf(() => deferred.prepare('SELECT :id').get()) !== 'INVALID_ARGUMENT') throw new Error('missing named code')
		const extensionDB = new DatabaseSync(':memory:', { allowExtension: true })
		extensionDB.enableLoadExtension(false)
		extensionDB.enableLoadExtension(true)
		if (codeOf(() => extensionDB.loadExtension('/missing')) !== 'ERROR') throw new Error('extension code')
		extensionDB.close()
		deferred.close()
		if (codeOf(() => deferred.close()) !== 'DATABASE_NOT_OPEN') throw new Error('double close code')
	`)
	if err != nil {
		t.Fatalf("RunString() error = %v", err)
	}
}

func TestNodeSQLiteReadOnlyReopensAferoDatabase(t *testing.T) {
	backend := afero.NewMemMapFs()

	writer := goja.New()
	writerRegistry := new(require.Registry)
	writerRegistry.Enable(writer)
	sqlite.RegisterWithOptions(writerRegistry, fs.WithFS(backend), fs.WithCwd("/workspace"))
	if _, err := writer.RunString(`
		const { DatabaseSync } = require('sqlite')
		const db = new DatabaseSync('readonly.db')
		db.exec("CREATE TABLE t (value TEXT); INSERT INTO t VALUES ('persisted')")
		db.close()
	`); err != nil {
		t.Fatalf("writer RunString() error = %v", err)
	}

	reader := goja.New()
	readerRegistry := new(require.Registry)
	readerRegistry.Enable(reader)
	sqlite.RegisterWithOptions(readerRegistry, fs.WithFS(backend), fs.WithCwd("/workspace"))
	if _, err := reader.RunString(`
		const { DatabaseSync } = require('node:sqlite')
		const db = new DatabaseSync('readonly.db', { readOnly: true })
		if (db.prepare('SELECT value FROM t').get().value !== 'persisted') throw new Error('read-only read failed')
		try {
			db.prepare("INSERT INTO t VALUES ('rejected')").run()
			throw new Error('read-only write succeeded')
		} catch (err) {
			if (!err.code || (err.code !== 'READONLY' && err.code !== 'ERROR')) throw err
		}
		db.close()
	`); err != nil {
		t.Fatalf("reader RunString() error = %v", err)
	}
}

func TestNodeSQLiteUsesBasePathFs(t *testing.T) {
	source := afero.NewMemMapFs()
	if err := source.MkdirAll("/root/workspace", 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	backend := afero.NewBasePathFs(source, "/root")
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	sqlite.RegisterWithOptions(registry, fs.WithFS(backend), fs.WithCwd("/workspace"))

	if _, err := rt.RunString(`
		const { DatabaseSync } = require('sqlite')
		const db = new DatabaseSync('base.db')
		db.exec('CREATE TABLE t (value TEXT); INSERT INTO t VALUES (\'base\')')
		if (db.prepare('SELECT value FROM t').get().value !== 'base') throw new Error('BasePathFs read failed')
		db.close()
	`); err != nil {
		t.Fatalf("RunString() error = %v", err)
	}
	if exists, err := afero.Exists(source, "/root/workspace/base.db"); err != nil || !exists {
		t.Fatalf("BasePathFs file exists = %v, err = %v", exists, err)
	}
}

func TestNodeSQLiteIterationValuesAndWALBoundary(t *testing.T) {
	backend := afero.NewMemMapFs()
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	sqlite.RegisterWithOptions(registry, fs.WithFS(backend), fs.WithCwd("/workspace"))

	if _, err := rt.RunString(`
		const { DatabaseSync } = require('sqlite')
		const db = new DatabaseSync('wal.db')
		db.exec('CREATE TABLE t (id INTEGER PRIMARY KEY, group_id INTEGER); INSERT INTO t(group_id) VALUES (1), (2), (1)')
		const stmt = db.prepare('SELECT id FROM t WHERE group_id = :group ORDER BY id')
		const ids = []
		for (const row of stmt.iterate({ group: 1 })) ids.push(row.id)
		if (ids.length !== 2 || ids[0] !== 1 || ids[1] !== 3) throw new Error('iteration mismatch')
		const values = db.prepare('SELECT id FROM t ORDER BY id').values()
		if (values.length !== 3 || values[2][0] !== 3) throw new Error('values mismatch')
		const run = db.prepare('INSERT INTO t(group_id) VALUES (?)').run(2)
		if (run.changes !== 1 || run.lastInsertRowid !== 4) throw new Error('run result mismatch')
		db.close()
	`); err != nil {
		t.Fatalf("RunString() error = %v", err)
	}
	if err := afero.WriteFile(backend, "/workspace/wal.db-wal", []byte("wal marker"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := rt.RunString(`
		const DatabaseSyncAgain = require('sqlite').DatabaseSync
		try {
			new DatabaseSyncAgain('wal.db')
			throw new Error('WAL database unexpectedly opened')
		} catch (err) {
			if (err.code !== 'NOT_IMPLEMENTED') throw err
		}
	`); err != nil {
		t.Fatalf("WAL boundary RunString() error = %v", err)
	}
}
