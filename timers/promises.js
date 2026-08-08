const timers = require('timers')

function optionsObject(options) {
  return options !== null && typeof options === 'object' ? options : {}
}

function abortError(signal) {
  const error = new Error('The operation was aborted')
  error.name = 'AbortError'
  error.code = 'ABORT_ERR'
  error.cause = signal.reason
  return error
}

function applyRefOption(handle, options) {
  if (options.ref === false && handle && typeof handle.unref === 'function') {
    handle.unref()
  }
}

function setTimeoutPromise(delay, value, options) {
  if (typeof delay !== 'number') delay = 1
  options = optionsObject(options)
  const signal = options.signal

  return new Promise(function (resolve, reject) {
    if (signal && signal.aborted) {
      reject(abortError(signal))
      return
    }

    let handle = null
    let settled = false
    const cleanup = function () {
      if (signal) signal.removeEventListener('abort', onAbort)
    }
    const onAbort = function () {
      if (settled) return
      settled = true
      if (handle !== null) timers.clearTimeout(handle)
      cleanup()
      reject(abortError(signal))
    }

    if (signal) signal.addEventListener('abort', onAbort)
    handle = timers.setTimeout(function () {
      if (settled) return
      settled = true
      cleanup()
      resolve(value)
    }, delay)
    applyRefOption(handle, options)
  })
}

function setImmediatePromise(value, options) {
  options = optionsObject(options)
  const signal = options.signal

  return new Promise(function (resolve, reject) {
    if (signal && signal.aborted) {
      reject(abortError(signal))
      return
    }

    let handle = null
    let settled = false
    const cleanup = function () {
      if (signal) signal.removeEventListener('abort', onAbort)
    }
    const onAbort = function () {
      if (settled) return
      settled = true
      if (handle !== null) timers.clearImmediate(handle)
      cleanup()
      reject(abortError(signal))
    }

    if (signal) signal.addEventListener('abort', onAbort)
    handle = timers.setImmediate(function () {
      if (settled) return
      settled = true
      cleanup()
      resolve(value)
    })
    applyRefOption(handle, options)
  })
}

function setIntervalIterator(delay, value, options) {
  if (typeof delay !== 'number') delay = 1
  options = optionsObject(options)
  const signal = options.signal
  const queue = []
  const waiting = []
  let stopped = false
  let failure = signal && signal.aborted ? abortError(signal) : null
  let handle = null

  function cleanup() {
    if (handle !== null) {
      timers.clearInterval(handle)
      handle = null
    }
    if (signal) signal.removeEventListener('abort', onAbort)
  }

  function onAbort() {
    if (stopped) return
    stopped = true
    failure = abortError(signal)
    cleanup()
    while (waiting.length > 0) waiting.shift().reject(failure)
  }

  if (!failure) {
    if (signal) signal.addEventListener('abort', onAbort)
    handle = timers.setInterval(function () {
      if (waiting.length > 0) {
        waiting.shift().resolve({ value, done: false })
      } else {
        queue.push({ value, done: false })
      }
    }, delay)
    applyRefOption(handle, options)
  }

  return {
    next() {
      if (failure) return Promise.reject(failure)
      if (queue.length > 0) return Promise.resolve(queue.shift())
      if (stopped) return Promise.resolve({ value: undefined, done: true })
      return new Promise(function (resolve, reject) { waiting.push({ resolve, reject }) })
    },

    return() {
      if (!stopped) {
        stopped = true
        cleanup()
      }
      while (waiting.length > 0) waiting.shift().resolve({ value: undefined, done: true })
      return Promise.resolve({ value: undefined, done: true })
    },

    [Symbol.asyncIterator]() { return this }
  }
}

module.exports = {
  setTimeout: setTimeoutPromise,
  setImmediate: setImmediatePromise,
  setInterval: setIntervalIterator,
  scheduler: {
    wait(delay, options) {
      return setTimeoutPromise(delay, undefined, options)
    }
  }
}
