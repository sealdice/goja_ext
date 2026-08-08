/*
 * Adapted from bare-events (https://github.com/holepunchto/bare-events)
 * Copyright Holepunch. Apache-2.0.
 *
 * Adaptations:
 *   - inlined lib/errors.js EventEmitterError
 *   - listeners()/rawListeners() return plain function arrays (Node semantics)
 *   - 'error' with no listeners throws synchronously inside emit() (Node semantics)
 *   - added getEventListeners / setMaxListeners / addAbortListener / emit / SymbolRejection
 */

module.exports = exports = class EventEmitterError extends Error {
  constructor(msg, code, fn = EventEmitterError, opts) {
    super(`${code}: ${msg}`, opts)
    this.code = code
    this.name = 'EventEmitterError'

    if (Error.captureStackTrace) {
      Error.captureStackTrace(this, fn)
    }
  }

  static OPERATION_ABORTED(cause, msg = 'Operation aborted') {
    const err = new EventEmitterError(msg, 'ABORT_ERR', EventEmitterError.OPERATION_ABORTED, {
      cause
    })
    err.name = 'AbortError'
    return err
  }

  static UNHANDLED_ERROR(cause, msg = 'Unhandled error') {
    return new EventEmitterError(msg, 'UNHANDLED_ERROR', EventEmitterError.UNHANDLED_ERROR, {
      cause
    })
  }
}

const errors = module.exports

class EventListener {
  constructor() {
    this.list = []
    this.count = 0
  }

  append(ctx, name, fn, once) {
    this.count++
    ctx.emit('newListener', name, fn) // Emit BEFORE adding
    this.list.push([once ? onceWrapper(fn) : fn, once])
  }

  prepend(ctx, name, fn, once) {
    this.count++
    ctx.emit('newListener', name, fn) // Emit BEFORE adding
    this.list.unshift([once ? onceWrapper(fn) : fn, once])
  }

  remove(ctx, name, fn) {
    for (let i = 0, n = this.list.length; i < n; i++) {
      const l = this.list[i]

      if (l[0] === fn || l[0].listener === fn) {
        this.list.splice(i, 1)

        if (this.count === 1) delete ctx._events[name]

        ctx.emit('removeListener', name, l[0].listener || l[0]) // Emit AFTER removing

        this.count--
        return
      }
    }
  }

  removeAll(ctx, name) {
    const list = [...this.list]
    this.list = []

    if (this.count === list.length) delete ctx._events[name]

    for (let i = list.length - 1; i >= 0; i--) {
      ctx.emit('removeListener', name, list[i][0].listener || list[i][0]) // Emit AFTER removing
    }

    this.count -= list.length
  }

  emit(ctx, name, ...args) {
    const list = [...this.list]

    for (let i = 0, n = list.length; i < n; i++) {
      const l = list[i]

      if (l[1] === true) this.remove(ctx, name, l[0])

      Reflect.apply(l[0], ctx, args)
    }

    return list.length > 0
  }
}

function onceWrapper(fn) {
  function wrapped(...args) {
    return Reflect.apply(fn, this, args)
  }
  wrapped.listener = fn
  return wrapped
}

function appendListener(ctx, name, fn, once) {
  if (ctx._events === undefined) ctx._events = Object.create(null)
  const e = ctx._events[name] || (ctx._events[name] = new EventListener())
  e.append(ctx, name, fn, once)
  return ctx
}

function prependListener(ctx, name, fn, once) {
  if (ctx._events === undefined) ctx._events = Object.create(null)
  const e = ctx._events[name] || (ctx._events[name] = new EventListener())
  e.prepend(ctx, name, fn, once)
  return ctx
}

function removeListener(ctx, name, fn) {
  if (ctx._events === undefined) return ctx
  const e = ctx._events[name]
  if (e !== undefined) e.remove(ctx, name, fn)
  return ctx
}

function EventEmitter() {
  this._events = Object.create(null)
}
module.exports = exports = EventEmitter

EventEmitter.prototype.addListener = function (name, fn) {
  return appendListener(this, name, fn, false)
}

EventEmitter.prototype.addOnceListener = function (name, fn) {
  return appendListener(this, name, fn, true)
}

EventEmitter.prototype.prependListener = function (name, fn) {
  return prependListener(this, name, fn, false)
}

EventEmitter.prototype.prependOnceListener = function (name, fn) {
  return prependListener(this, name, fn, true)
}

EventEmitter.prototype.removeListener = function (name, fn) {
  return removeListener(this, name, fn)
}

EventEmitter.prototype.on = function (name, fn) {
  return appendListener(this, name, fn, false)
}

EventEmitter.prototype.once = function (name, fn) {
  return appendListener(this, name, fn, true)
}

EventEmitter.prototype.off = function (name, fn) {
  return removeListener(this, name, fn)
}

EventEmitter.prototype.emit = function (name, ...args) {
  if (name === 'error' && (this._events === undefined || this._events.error === undefined)) {
    const err = args.length > 0 ? args[0] : undefined
    throw err === undefined ? new Error('Unhandled error.') : err
  }

  if (this._events === undefined) return false
  const e = this._events[name]
  return e === undefined ? false : e.emit(this, name, ...args)
}

EventEmitter.prototype.listeners = function (name) {
  if (this._events === undefined) return []
  const e = this._events[name]
  return e === undefined ? [] : e.list.map((l) => l[0].listener || l[0])
}

EventEmitter.prototype.rawListeners = function (name) {
  if (this._events === undefined) return []
  const e = this._events[name]
  return e === undefined ? [] : e.list.map((l) => l[0])
}

EventEmitter.prototype.eventNames = function () {
  if (this._events === undefined) return []
  return Reflect.ownKeys(this._events)
}

EventEmitter.prototype.listenerCount = function (name) {
  if (this._events === undefined) return 0
  const e = this._events[name]
  return e === undefined ? 0 : e.list.length
}

EventEmitter.prototype.getMaxListeners = function () {
  return this._maxListeners === undefined ? EventEmitter.defaultMaxListeners : this._maxListeners
}

EventEmitter.prototype.setMaxListeners = function (n) {
	if (typeof n !== 'number' || n < 0 || Number.isNaN(n)) {
		throw new RangeError('max listeners must be a non-negative number')
	}
	this._maxListeners = n
	return this
}

EventEmitter.prototype.removeAllListeners = function (name) {
  if (arguments.length === 0) {
    if (this._events === undefined) return this
    for (const key of Reflect.ownKeys(this._events)) {
      if (key === 'removeListener') continue
      this.removeAllListeners(key)
    }
    this.removeAllListeners('removeListener')
  } else {
    if (this._events === undefined) return this
    const e = this._events[name]
    if (e !== undefined) e.removeAll(this, name)
  }
  return this
}

exports.EventEmitter = exports

exports.errors = errors

exports.defaultMaxListeners = 10

exports.on = function on(emitter, name, opts = {}) {
  const { signal } = opts

  if (signal && signal.aborted) {
    throw exports.errors.OPERATION_ABORTED(signal.reason)
  }

  let error = null
  let done = false

  const events = []
  const promises = []

  if (name !== 'error') emitter.on('error', onerror)

  if (signal) signal.addEventListener('abort', onabort)

  emitter.on(name, onevent)

  return {
    next() {
      if (events.length) {
        return Promise.resolve({ value: events.shift(), done: false })
      }

      if (error) {
        const err = error

        error = null

        return Promise.reject(err)
      }

      if (done) return onclose()

      return new Promise((resolve, reject) => promises.push({ resolve, reject }))
    },

    return() {
      return onclose()
    },

    throw(err) {
      return onerror(err)
    },

    [Symbol.asyncIterator]() {
      return this
    }
  }

  function onevent(...args) {
    if (promises.length) {
      promises.shift().resolve({ value: args, done: false })
    } else {
      events.push(args)
    }
  }

  function onerror(err) {
    emitter.off(name, onevent).off('error', onerror)

    if (promises.length) {
      promises.shift().reject(err)
    } else {
      error = err
    }

    return Promise.resolve({ done: true })
  }

  function onabort() {
    signal.removeEventListener('abort', onabort)

    onerror(exports.errors.OPERATION_ABORTED(signal.reason))
  }

  function onclose() {
    emitter.off(name, onevent)

    if (name !== 'error') emitter.off('error', onerror)

    if (signal) signal.removeEventListener('abort', onabort)

    done = true

    if (promises.length) promises.shift().resolve({ done: true })

    return Promise.resolve({ done: true })
  }
}

exports.once = function once(emitter, name, opts = {}) {
  const { signal } = opts

  if (signal && signal.aborted) {
    return Promise.reject(exports.errors.OPERATION_ABORTED(signal.reason))
  }

  return new Promise((resolve, reject) => {
    if (name !== 'error') emitter.on('error', onerror)

    if (signal) signal.addEventListener('abort', onabort)

    emitter.once(name, onevent)

    function onevent(...args) {
      if (name !== 'error') emitter.off('error', onerror)

      if (signal) signal.removeEventListener('abort', onabort)

      resolve(args)
    }

    function onerror(err) {
      emitter.off(name, onevent)

      if (name !== 'error') emitter.off('error', onerror)

      reject(err)
    }

    function onabort() {
      signal.removeEventListener('abort', onabort)

      onerror(exports.errors.OPERATION_ABORTED(signal.reason))
    }
  })
}

exports.forward = function forward(from, to, names, opts = {}) {
  if (typeof names === 'string') names = [names]

  const { emit = to.emit.bind(to) } = opts

  const listeners = names.map(
    (name) =>
      function onevent(...args) {
        emit(name, ...args)
      }
  )

  to.on('newListener', (name) => {
    const i = names.indexOf(name)

    if (i !== -1 && to.listenerCount(name) === 0) {
      from.on(name, listeners[i])
    }
  }).on('removeListener', (name) => {
    const i = names.indexOf(name)

    if (i !== -1 && to.listenerCount(name) === 0) {
      from.off(name, listeners[i])
    }
  })
}

exports.listenerCount = function listenerCount(emitter, name) {
  return emitter.listenerCount(name)
}

exports.getMaxListeners = function getMaxListeners(emitter) {
  if (typeof emitter.getMaxListeners === 'function') {
    return emitter.getMaxListeners()
  }

  return exports.defaultMaxListeners
}

exports.setMaxListeners = function setMaxListeners(n, ...emitters) {
  if (emitters.length === 0) exports.defaultMaxListeners = n
  else {
    for (const emitter of emitters) {
      if (typeof emitter.setMaxListeners === 'function') {
        emitter.setMaxListeners(n)
      }
    }
  }
}

exports.getEventListeners = function getEventListeners(emitter, name) {
  if (emitter === null || emitter === undefined) return []
  if (typeof emitter.rawListeners === 'function') {
    return emitter.rawListeners(name)
  }
  return []
}

exports.addAbortListener = function addAbortListener(signal, listener) {
  if (signal === null || signal === undefined) {
    throw new TypeError('The "signal" argument must be an AbortSignal')
  }
  if (typeof listener !== 'function') {
    throw new TypeError('The "listener" argument must be a function')
  }
  const onAbort = () => {
    listener()
    signal.removeEventListener('abort', onAbort)
  }
  if (signal.aborted) {
    onAbort()
  } else {
    signal.addEventListener('abort', onAbort)
  }
  return {
    unsubscribe() {
      signal.removeEventListener('abort', onAbort)
    }
  }
}

exports.emit = function emit(emitter, ...args) {
  return emitter.emit(...args)
}

exports.SymbolRejection = Symbol.for('nodejs.rejection')
