package sqlite

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/vfs"
	"github.com/sealdice/goja_ext/buffer"
	hostfs "github.com/sealdice/goja_ext/fs"
)

var bindingVFSID atomic.Uint64

type runtimeBinding struct {
	rt    *goja.Runtime
	core  *hostfs.Core
	vfs   *aferoVFS
	vfsID string

	nextID uint64
	dbs    map[uint64]*databaseState
	stmts  map[uint64]*statementState
}

type databaseState struct {
	conn           *sqlite3.Conn
	allowExtension bool
}

type statementState struct {
	db                          *databaseState
	stmt                        *sqlite3.Stmt
	readBigInts                 bool
	allowBareNamedParameters    bool
	allowUnknownNamedParameters bool
}

func newRuntimeBinding(rt *goja.Runtime, core *hostfs.Core) *runtimeBinding {
	id := bindingVFSID.Add(1)
	name := fmt.Sprintf("goja_ext_afero_%d", id)
	b := &runtimeBinding{
		rt:    rt,
		core:  core,
		vfs:   newAferoVFS(core),
		vfsID: name,
		dbs:   make(map[uint64]*databaseState),
		stmts: make(map[uint64]*statementState),
	}
	// The VFS registry is process-global, while the VFS itself owns only the
	// runtime's Afero Core and process-local lock state.
	vfs.Register(name, b.vfs)
	return b
}

func (b *runtimeBinding) exports() *goja.Object {
	exports := b.rt.NewObject()
	_ = exports.Set("open", b.open)
	_ = exports.Set("close", b.close)
	_ = exports.Set("exec", b.exec)
	_ = exports.Set("prepare", b.prepare)
	_ = exports.Set("expandedSQL", b.expandedSQL)
	_ = exports.Set("all", b.all)
	_ = exports.Set("values", b.values)
	_ = exports.Set("get", b.get)
	_ = exports.Set("run", b.run)
	_ = exports.Set("bind", b.bind)
	_ = exports.Set("step", b.step)
	_ = exports.Set("reset", b.reset)
	_ = exports.Set("columns", b.columns)
	_ = exports.Set("finalize", b.finalize)
	_ = exports.Set("readBigInts", b.readBigInts)
	_ = exports.Set("allowBareNamedParameters", b.allowBareNamedParameters)
	_ = exports.Set("allowUnknownNamedParameters", b.allowUnknownNamedParameters)
	_ = exports.Set("enableLoadExtension", b.enableLoadExtension)
	_ = exports.Set("loadExtension", b.loadExtension)
	return exports
}

func (b *runtimeBinding) open(call goja.FunctionCall) goja.Value {
	location := call.Argument(0).String()
	readOnly := call.Argument(1).ToBoolean()
	foreignKeys := call.Argument(2).ToBoolean()
	dqs := call.Argument(3).ToBoolean()
	allowExtension := call.Argument(4).ToBoolean()
	timeout := call.Argument(5).ToInteger()

	uri, resolved := b.databaseURI(location)
	if resolved != "" {
		if _, err := b.core.Backend().Stat(resolved + "-wal"); err == nil {
			b.raiseCode("NOT_IMPLEMENTED", "WAL databases are not supported by the Afero VFS")
		}
	}

	flags := sqlite3.OPEN_URI
	if readOnly {
		flags |= sqlite3.OPEN_READONLY
	} else {
		flags |= sqlite3.OPEN_READWRITE | sqlite3.OPEN_CREATE
	}
	conn, err := sqlite3.OpenFlags(uri, flags)
	if err != nil {
		b.raise(err)
	}
	if timeout > 0 {
		if err := conn.BusyTimeout(time.Duration(timeout) * time.Millisecond); err != nil {
			_ = conn.Close()
			b.raise(err)
		}
	}
	if _, err := conn.Config(sqlite3.DBCONFIG_ENABLE_FKEY, foreignKeys); err != nil {
		_ = conn.Close()
		b.raise(err)
	}
	if _, err := conn.Config(sqlite3.DBCONFIG_DQS_DML, dqs); err != nil {
		_ = conn.Close()
		b.raise(err)
	}
	if _, err := conn.Config(sqlite3.DBCONFIG_DQS_DDL, dqs); err != nil {
		_ = conn.Close()
		b.raise(err)
	}

	state := &databaseState{conn: conn, allowExtension: allowExtension}
	handle := b.newHandle()
	b.dbs[handleID(handle)] = state
	return handle
}

func (b *runtimeBinding) databaseURI(location string) (string, string) {
	if location == ":memory:" {
		return "file::memory:?vfs=" + url.QueryEscape(b.vfsID), ""
	}
	if strings.HasPrefix(location, "file:") {
		u, err := url.Parse(location)
		if err != nil {
			b.raise(err)
		}
		query := u.Query()
		query.Set("vfs", b.vfsID)
		u.RawQuery = query.Encode()
		resolved := ""
		if u.Path != "" {
			resolved = b.core.ResolvePath(u.Path)
		}
		return u.String(), resolved
	}
	resolved := b.core.ResolvePath(location)
	query := url.Values{"vfs": {b.vfsID}}
	return (&url.URL{Scheme: "file", Path: resolved, RawQuery: query.Encode()}).String(), resolved
}

func (b *runtimeBinding) close(call goja.FunctionCall) goja.Value {
	handle := b.handle(call.Argument(0))
	db, ok := b.dbs[handleID(handle)]
	if !ok {
		b.raiseCode("DATABASE_NOT_OPEN", "Database is not open")
	}
	for id, state := range b.stmts {
		if state.db == db {
			_ = state.stmt.Close()
			delete(b.stmts, id)
		}
	}
	if err := db.conn.Close(); err != nil {
		b.raise(err)
	}
	delete(b.dbs, handleID(handle))
	return goja.Undefined()
}

func (b *runtimeBinding) exec(call goja.FunctionCall) goja.Value {
	db := b.database(call.Argument(0))
	if err := db.conn.Exec(call.Argument(1).String()); err != nil {
		b.raise(err)
	}
	return goja.Undefined()
}

func (b *runtimeBinding) prepare(call goja.FunctionCall) goja.Value {
	db := b.database(call.Argument(0))
	stmt, _, err := db.conn.Prepare(call.Argument(1).String())
	if err != nil {
		b.raise(err)
	}
	state := &statementState{db: db, stmt: stmt, allowBareNamedParameters: true}
	handle := b.newHandle()
	b.stmts[handleID(handle)] = state
	return handle
}

func (b *runtimeBinding) expandedSQL(call goja.FunctionCall) goja.Value {
	return b.rt.ToValue(b.statement(call.Argument(0)).stmt.ExpandedSQL())
}

func (b *runtimeBinding) all(call goja.FunctionCall) goja.Value {
	state := b.statement(call.Argument(0))
	b.bindParameters(state, call.Argument(1), call.Argument(2))
	rows := b.rt.NewArray()
	index := uint32(0)
	for state.stmt.Step() {
		_ = rows.Set(strconv.FormatUint(uint64(index), 10), b.row(state))
		index++
	}
	if err := state.stmt.Err(); err != nil {
		_ = state.stmt.Reset()
		b.raise(err)
	}
	b.resetStatement(state)
	return rows
}

func (b *runtimeBinding) values(call goja.FunctionCall) goja.Value {
	state := b.statement(call.Argument(0))
	b.bindParameters(state, call.Argument(1), call.Argument(2))
	rows := b.rt.NewArray()
	index := uint32(0)
	for state.stmt.Step() {
		_ = rows.Set(strconv.FormatUint(uint64(index), 10), b.valuesRow(state))
		index++
	}
	if err := state.stmt.Err(); err != nil {
		_ = state.stmt.Reset()
		b.raise(err)
	}
	b.resetStatement(state)
	return rows
}

func (b *runtimeBinding) get(call goja.FunctionCall) goja.Value {
	state := b.statement(call.Argument(0))
	b.bindParameters(state, call.Argument(1), call.Argument(2))
	if !state.stmt.Step() {
		if err := state.stmt.Err(); err != nil {
			_ = state.stmt.Reset()
			b.raise(err)
		}
		b.resetStatement(state)
		return goja.Undefined()
	}
	row := b.row(state)
	b.resetStatement(state)
	return row
}

func (b *runtimeBinding) run(call goja.FunctionCall) goja.Value {
	state := b.statement(call.Argument(0))
	b.bindParameters(state, call.Argument(1), call.Argument(2))
	if err := state.stmt.Exec(); err != nil {
		_ = state.stmt.Reset()
		b.raise(err)
	}
	result := b.runResult(state)
	b.resetStatement(state)
	return result
}

func (b *runtimeBinding) bind(call goja.FunctionCall) goja.Value {
	state := b.statement(call.Argument(0))
	b.bindParameters(state, call.Argument(1), call.Argument(2))
	return goja.Undefined()
}

func (b *runtimeBinding) step(call goja.FunctionCall) goja.Value {
	state := b.statement(call.Argument(0))
	if state.stmt.Step() {
		return b.row(state)
	}
	if err := state.stmt.Err(); err != nil {
		b.raise(err)
	}
	return goja.Undefined()
}

func (b *runtimeBinding) reset(call goja.FunctionCall) goja.Value {
	b.resetStatement(b.statement(call.Argument(0)))
	return goja.Undefined()
}

func (b *runtimeBinding) columns(call goja.FunctionCall) goja.Value {
	state := b.statement(call.Argument(0))
	columns := b.rt.NewArray()
	for i := range state.stmt.ColumnCount() {
		column := b.rt.NewObject()
		setNullableString(column, "column", state.stmt.ColumnOriginName(i))
		_ = column.Set("name", state.stmt.ColumnName(i))
		setNullableString(column, "database", state.stmt.ColumnDatabaseName(i))
		setNullableString(column, "table", state.stmt.ColumnTableName(i))
		setNullableString(column, "type", state.stmt.ColumnDeclType(i))
		_ = columns.Set(strconv.Itoa(i), column)
	}
	return columns
}

func (b *runtimeBinding) finalize(call goja.FunctionCall) goja.Value {
	handle := b.handle(call.Argument(0))
	state, ok := b.stmts[handleID(handle)]
	if !ok {
		return goja.Undefined()
	}
	if err := state.stmt.Close(); err != nil {
		b.raise(err)
	}
	delete(b.stmts, handleID(handle))
	return goja.Undefined()
}

func (b *runtimeBinding) readBigInts(call goja.FunctionCall) goja.Value {
	b.statement(call.Argument(0)).readBigInts = call.Argument(1).ToBoolean()
	return goja.Undefined()
}

func (b *runtimeBinding) allowBareNamedParameters(call goja.FunctionCall) goja.Value {
	b.statement(call.Argument(0)).allowBareNamedParameters = call.Argument(1).ToBoolean()
	return goja.Undefined()
}

func (b *runtimeBinding) allowUnknownNamedParameters(call goja.FunctionCall) goja.Value {
	b.statement(call.Argument(0)).allowUnknownNamedParameters = call.Argument(1).ToBoolean()
	return goja.Undefined()
}

func (b *runtimeBinding) enableLoadExtension(call goja.FunctionCall) goja.Value {
	db := b.database(call.Argument(0))
	if !db.allowExtension {
		b.raiseCode("LOAD_EXTENSION_DISABLED", "Extension loading is disabled")
	}
	return goja.Undefined()
}

func (b *runtimeBinding) loadExtension(call goja.FunctionCall) goja.Value {
	db := b.database(call.Argument(0))
	if !db.allowExtension {
		b.raiseCode("LOAD_EXTENSION_DISABLED", "Extension loading is disabled")
	}
	b.raiseCode("ERROR", "SQLite extensions are not supported by the CGO-free binding")
	return goja.Undefined()
}

func (b *runtimeBinding) bindParameters(state *statementState, named, positional goja.Value) {
	if err := state.stmt.ClearBindings(); err != nil {
		b.raise(err)
	}
	count := state.stmt.BindCount()
	bound := make(map[int]bool, count)
	if named != nil && !goja.IsNull(named) && !goja.IsUndefined(named) {
		object := named.ToObject(b.rt)
		for _, key := range object.Keys() {
			index := b.namedParameterIndex(state, key)
			if index == 0 {
				if state.allowUnknownNamedParameters {
					continue
				}
				b.raiseCode("INVALID_ARGUMENT", "Unknown named parameter: "+key)
			}
			if err := b.bindValue(state.stmt, index, object.Get(key)); err != nil {
				b.raise(err)
			}
			bound[index] = true
		}
	}

	positionalValues := arrayValues(b.rt, positional)
	position := 0
	for index := 1; index <= count && position < len(positionalValues); index++ {
		if bound[index] {
			continue
		}
		if err := b.bindValue(state.stmt, index, positionalValues[position]); err != nil {
			b.raise(err)
		}
		bound[index] = true
		position++
	}
	if position != len(positionalValues) {
		b.raiseCode("INVALID_ARGUMENT", "Too many positional parameters")
	}
	if len(bound) != count {
		b.raiseCode("INVALID_ARGUMENT", "Missing SQL parameters")
	}
}

func (b *runtimeBinding) namedParameterIndex(state *statementState, key string) int {
	if strings.HasPrefix(key, ":") || strings.HasPrefix(key, "@") || strings.HasPrefix(key, "$") {
		return state.stmt.BindIndex(key)
	}
	if !state.allowBareNamedParameters {
		return 0
	}
	for _, prefix := range []string{":", "@", "$"} {
		if index := state.stmt.BindIndex(prefix + key); index != 0 {
			return index
		}
	}
	return 0
}

func (b *runtimeBinding) bindValue(stmt *sqlite3.Stmt, index int, value goja.Value) error {
	if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
		return stmt.BindNull(index)
	}
	if goja.IsBigInt(value) {
		bigValue, ok := value.Export().(*big.Int)
		if !ok || !bigValue.IsInt64() {
			return errors.New("BigInt parameter is outside SQLite integer range")
		}
		return stmt.BindInt64(index, bigValue.Int64())
	}
	if value.ExportType() == reflect.TypeOf(true) {
		return stmt.BindBool(index, value.ToBoolean())
	}
	if goja.IsNumber(value) {
		number := value.ToFloat()
		if math.Trunc(number) == number && number >= math.MinInt64 && number <= math.MaxInt64 {
			return stmt.BindInt64(index, int64(number))
		}
		return stmt.BindFloat(index, number)
	}
	if goja.IsString(value) {
		return stmt.BindText(index, value.String())
	}
	if object, ok := value.(*goja.Object); ok {
		return stmt.BindBlob(index, buffer.DecodeBytes(b.rt, object, goja.Undefined()))
	}
	return errors.New("unsupported SQL parameter type")
}

func (b *runtimeBinding) row(state *statementState) *goja.Object {
	row := b.rt.NewObject()
	for i := range state.stmt.ColumnCount() {
		_ = row.Set(state.stmt.ColumnName(i), b.columnValue(state, i))
	}
	return row
}

func (b *runtimeBinding) valuesRow(state *statementState) *goja.Object {
	row := b.rt.NewArray()
	for i := range state.stmt.ColumnCount() {
		_ = row.Set(strconv.Itoa(i), b.columnValue(state, i))
	}
	return row
}

func (b *runtimeBinding) columnValue(state *statementState, index int) goja.Value {
	switch state.stmt.ColumnType(index) {
	case sqlite3.INTEGER:
		value := state.stmt.ColumnInt64(index)
		if state.readBigInts {
			return b.rt.ToValue(big.NewInt(value))
		}
		return b.rt.ToValue(value)
	case sqlite3.FLOAT:
		return b.rt.ToValue(state.stmt.ColumnFloat(index))
	case sqlite3.TEXT:
		return b.rt.ToValue(state.stmt.ColumnText(index))
	case sqlite3.BLOB:
		return buffer.WrapBytes(b.rt, append([]byte(nil), state.stmt.ColumnRawBlob(index)...))
	default:
		return goja.Null()
	}
}

func (b *runtimeBinding) runResult(state *statementState) *goja.Object {
	result := b.rt.NewObject()
	changes := state.db.conn.Changes()
	lastInsertRowID := state.db.conn.LastInsertRowID()
	if state.readBigInts {
		_ = result.Set("changes", big.NewInt(changes))
		_ = result.Set("lastInsertRowid", big.NewInt(lastInsertRowID))
	} else {
		_ = result.Set("changes", changes)
		_ = result.Set("lastInsertRowid", lastInsertRowID)
	}
	return result
}

func (b *runtimeBinding) resetStatement(state *statementState) {
	if err := state.stmt.Reset(); err != nil {
		b.raise(err)
	}
	if err := state.stmt.ClearBindings(); err != nil {
		b.raise(err)
	}
}

func (b *runtimeBinding) newHandle() *goja.Object {
	b.nextID++
	object := b.rt.NewObject()
	_ = object.Set("__goja_ext_sqlite_id", b.nextID)
	return object
}

func handleID(object *goja.Object) uint64 {
	return uint64(object.Get("__goja_ext_sqlite_id").ToInteger())
}

func (b *runtimeBinding) handle(value goja.Value) *goja.Object {
	object, ok := value.(*goja.Object)
	if !ok || object.Get("__goja_ext_sqlite_id") == nil {
		b.raiseCode("INVALID_ARGUMENT", "Invalid SQLite handle")
	}
	return object
}

func (b *runtimeBinding) database(value goja.Value) *databaseState {
	state, ok := b.dbs[handleID(b.handle(value))]
	if !ok {
		b.raiseCode("DATABASE_NOT_OPEN", "Database is not open")
	}
	return state
}

func (b *runtimeBinding) statement(value goja.Value) *statementState {
	state, ok := b.stmts[handleID(b.handle(value))]
	if !ok {
		b.raiseCode("INVALID_ARGUMENT", "Statement is finalized")
	}
	return state
}

func (b *runtimeBinding) raise(err error) {
	if err == nil {
		return
	}
	object := b.rt.NewGoError(err)
	_ = object.Set("code", sqliteErrorCode(err))
	panic(object)
}

func (b *runtimeBinding) raiseCode(code, message string) {
	object := b.rt.NewGoError(errors.New(message))
	_ = object.Set("code", code)
	panic(object)
}

func sqliteErrorCode(err error) string {
	var extended sqlite3.ExtendedErrorCode
	if errors.As(err, &extended) {
		return sqlitePrimaryCode(extended.Code())
	}
	var primary sqlite3.ErrorCode
	if errors.As(err, &primary) {
		return sqlitePrimaryCode(primary)
	}
	return "ERROR"
}

func sqlitePrimaryCode(code sqlite3.ErrorCode) string {
	names := map[sqlite3.ErrorCode]string{
		sqlite3.ERROR:      "ERROR",
		sqlite3.INTERNAL:   "INTERNAL",
		sqlite3.PERM:       "PERM",
		sqlite3.ABORT:      "ABORT",
		sqlite3.BUSY:       "BUSY",
		sqlite3.LOCKED:     "LOCKED",
		sqlite3.NOMEM:      "NOMEM",
		sqlite3.READONLY:   "READONLY",
		sqlite3.INTERRUPT:  "INTERRUPT",
		sqlite3.IOERR:      "IOERR",
		sqlite3.CORRUPT:    "CORRUPT",
		sqlite3.NOTFOUND:   "NOTFOUND",
		sqlite3.FULL:       "FULL",
		sqlite3.CANTOPEN:   "CANTOPEN",
		sqlite3.PROTOCOL:   "PROTOCOL",
		sqlite3.EMPTY:      "EMPTY",
		sqlite3.SCHEMA:     "SCHEMA",
		sqlite3.TOOBIG:     "TOOBIG",
		sqlite3.CONSTRAINT: "CONSTRAINT",
		sqlite3.MISMATCH:   "MISMATCH",
		sqlite3.MISUSE:     "MISUSE",
		sqlite3.NOLFS:      "NOLFS",
		sqlite3.AUTH:       "AUTH",
		sqlite3.FORMAT:     "FORMAT",
		sqlite3.RANGE:      "RANGE",
		sqlite3.NOTADB:     "NOTADB",
		sqlite3.NOTICE:     "NOTICE",
		sqlite3.WARNING:    "WARNING",
	}
	if name, ok := names[code]; ok {
		return name
	}
	return "ERROR"
}

func arrayValues(rt *goja.Runtime, value goja.Value) []goja.Value {
	if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
		return nil
	}
	object := value.ToObject(rt)
	length := object.Get("length").ToInteger()
	if length < 0 {
		return nil
	}
	values := make([]goja.Value, length)
	for i := range values {
		values[i] = object.Get(strconv.Itoa(i))
	}
	return values
}

func setNullableString(object *goja.Object, name, value string) {
	if value == "" {
		_ = object.Set(name, nil)
		return
	}
	_ = object.Set(name, value)
}
