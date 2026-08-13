(function () {
  return function (binding) {
    class SQLiteError extends Error {
      constructor(msg, fn = SQLiteError, code = fn.name) {
        super(`${code}: ${msg}`)
        this.code = code
        if (Error.captureStackTrace) Error.captureStackTrace(this, fn)
      }

      get name() {
        return 'SQLiteError'
      }

      static DATABASE_ALREADY_OPEN(msg) { return new SQLiteError(msg, SQLiteError.DATABASE_ALREADY_OPEN) }
      static DATABASE_NOT_OPEN(msg) { return new SQLiteError(msg, SQLiteError.DATABASE_NOT_OPEN) }
      static INVALID_ARGUMENT(msg) { return new SQLiteError(msg, SQLiteError.INVALID_ARGUMENT) }
      static NOT_IMPLEMENTED(msg) { return new SQLiteError(msg, SQLiteError.NOT_IMPLEMENTED) }
      static LOAD_EXTENSION_DISABLED(msg) { return new SQLiteError(msg, SQLiteError.LOAD_EXTENSION_DISABLED) }

      static from(err) {
        if (err instanceof SQLiteError) return err
        if (err instanceof TypeError) return SQLiteError.INVALID_ARGUMENT(err.message)
        return new SQLiteError(err && err.message, SQLiteError.from, (err && err.code) || 'ERROR')
      }
    }

    const errors = SQLiteError
    const dispose = typeof Symbol === 'function' && Symbol.dispose ? Symbol.dispose : Symbol('dispose')

    class StatementSync {
      constructor(db, sql) {
        this._db = db
        this._sourceSQL = sql
        try {
          this._handle = binding.prepare(db._handle, sql)
        } catch (err) {
          throw errors.from(err)
        }
      }

      get sourceSQL() { return this._sourceSQL }
      get expandedSQL() { return binding.expandedSQL(this._handle) }

      all(...params) {
        const [named, positional] = splitParameters(params)
        try { return binding.all(this._handle, named, positional) } catch (err) { throw errors.from(err) }
      }

      values(...params) {
        const [named, positional] = splitParameters(params)
        try { return binding.values(this._handle, named, positional) } catch (err) { throw errors.from(err) }
      }

      get(...params) {
        const [named, positional] = splitParameters(params)
        try {
          const row = binding.get(this._handle, named, positional)
          return row === undefined ? undefined : row
        } catch (err) { throw errors.from(err) }
      }

      run(...params) {
        const [named, positional] = splitParameters(params)
        try { return binding.run(this._handle, named, positional) } catch (err) { throw errors.from(err) }
      }

      *iterate(...params) {
        const [named, positional] = splitParameters(params)
        try {
          binding.bind(this._handle, named, positional)
          let row
          while ((row = binding.step(this._handle)) !== undefined) yield row
        } catch (err) {
          throw errors.from(err)
        } finally {
          binding.reset(this._handle)
        }
      }

      columns() { return binding.columns(this._handle) }
      setAllowBareNamedParameters(allow) { binding.allowBareNamedParameters(this._handle, !!allow) }
      setAllowUnknownNamedParameters(allow) { binding.allowUnknownNamedParameters(this._handle, !!allow) }
      setReadBigInts(enabled) { binding.readBigInts(this._handle, !!enabled) }

      [dispose]() {
        if (this._handle === null) return
        binding.finalize(this._handle)
        this._handle = null
      }
    }

    class TagStore {
      constructor(db, maxSize = 1000) {
        if (typeof maxSize !== 'number' || !Number.isInteger(maxSize) || maxSize <= 0) {
          throw errors.INVALID_ARGUMENT('maxSize must be a positive integer')
        }
        this._db = db
        this._maxSize = maxSize
        this._cache = new Map()
      }

      get db() { return this._db }
      get size() { return this._cache.size }
      get capacity() { return this._maxSize }
      clear() { this._cache.clear() }
      all(strings, ...params) { return this._lookup(strings).all({}, ...params) }
      values(strings, ...params) { return this._lookup(strings).values({}, ...params) }
      get(strings, ...params) { return this._lookup(strings).get({}, ...params) }
      iterate(strings, ...params) { return this._lookup(strings).iterate({}, ...params) }
      run(strings, ...params) { return this._lookup(strings).run({}, ...params) }

      _lookup(strings) {
        const sql = strings.join('?')
        let stmt = this._cache.get(sql)
        if (stmt !== undefined) {
          this._cache.delete(sql)
          this._cache.set(sql, stmt)
          return stmt
        }
        stmt = this._db.prepare(sql)
        this._cache.set(sql, stmt)
        if (this._cache.size > this._maxSize) {
          const oldest = this._cache.keys().next().value
          const old = this._cache.get(oldest)
          this._cache.delete(oldest)
          if (old && old[dispose]) old[dispose]()
        }
        return stmt
      }
    }

    class DatabaseSync {
      constructor(location, opts = {}) {
        const {
          open = true,
          readOnly = false,
          enableForeignKeyConstraints = true,
          enableDoubleQuotedStringLiterals = false,
          allowExtension = false,
          timeout = 0
        } = opts

        this._location = location
        this._readOnly = readOnly
        this._enableForeignKeyConstraints = enableForeignKeyConstraints
        this._enableDoubleQuotedStringLiterals = enableDoubleQuotedStringLiterals
        this._allowExtension = allowExtension
        this._timeout = timeout
        this._handle = null
        if (open) this.open()
      }

      get isOpen() { return this._handle !== null }
      get isTransaction() { throw errors.NOT_IMPLEMENTED('isTransaction is not implemented') }

      open() {
        if (this._handle !== null) throw errors.DATABASE_ALREADY_OPEN('Database is already open')
        try {
          this._handle = binding.open(
            this._location,
            this._readOnly,
            this._enableForeignKeyConstraints,
            this._enableDoubleQuotedStringLiterals,
            this._allowExtension,
            this._timeout
          )
        } catch (err) { throw errors.from(err) }
      }

      close() {
        if (this._handle === null) throw errors.DATABASE_NOT_OPEN('Database is not open')
        try { binding.close(this._handle) } catch (err) { throw errors.from(err) }
        this._handle = null
      }

      [dispose]() { if (this.isOpen) this.close() }

      exec(sql) {
        if (this._handle === null) throw errors.DATABASE_NOT_OPEN('Database is not open')
        try { binding.exec(this._handle, sql) } catch (err) { throw errors.from(err) }
      }

      prepare(sql) {
        if (this._handle === null) throw errors.DATABASE_NOT_OPEN('Database is not open')
        return new StatementSync(this, sql)
      }

      createTagStore(maxSize) {
        if (this._handle === null) throw errors.DATABASE_NOT_OPEN('Database is not open')
        return new TagStore(this, maxSize)
      }

      function() { throw errors.NOT_IMPLEMENTED('function is not implemented') }
      aggregate() { throw errors.NOT_IMPLEMENTED('aggregate is not implemented') }
      createSession() { throw errors.NOT_IMPLEMENTED('createSession is not implemented') }
      applyChangeset() { throw errors.NOT_IMPLEMENTED('applyChangeset is not implemented') }
      backup() { throw errors.NOT_IMPLEMENTED('backup is not implemented') }
      location() { throw errors.NOT_IMPLEMENTED('location is not implemented') }

      enableLoadExtension(allow) {
        if (this._handle === null) throw errors.DATABASE_NOT_OPEN('Database is not open')
        if (!this._allowExtension) throw errors.LOAD_EXTENSION_DISABLED('Extension loading is disabled')
        try { binding.enableLoadExtension(this._handle, !!allow) } catch (err) { throw errors.from(err) }
      }

      loadExtension(path, entryPoint = null) {
        if (this._handle === null) throw errors.DATABASE_NOT_OPEN('Database is not open')
        if (!this._allowExtension) throw errors.LOAD_EXTENSION_DISABLED('Extension loading is disabled')
        try { binding.loadExtension(this._handle, path, entryPoint) } catch (err) { throw errors.from(err) }
      }
    }

    function splitParameters(params) {
      if (params.length === 0) return [null, params]
      if (!isNamedParameters(params[0])) return [null, params]
      return [params[0], params.slice(1)]
    }

    function isNamedParameters(value) {
      if (value === null || typeof value !== 'object') return false
      if (Array.isArray(value)) return false
      if (ArrayBuffer.isView(value)) return false
      if (value instanceof ArrayBuffer) return false
      return true
    }

    return { DatabaseSync, StatementSync, TagStore, errors }
  }
})()
