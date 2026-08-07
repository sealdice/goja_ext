const timers = require('timers')

function promisifyTimeout(timerFn) {
  return function (delay, value, options) {
    if (typeof delay !== 'number') delay = 1
    return new Promise(function (resolve) {
      timerFn(function () { resolve(value) }, delay)
    })
  }
}

function promisifyImmediate(immediateFn) {
  return function (value, options) {
    return new Promise(function (resolve) {
      immediateFn(function () { resolve(value) })
    })
  }
}

function makeSetInterval(intervalFn, clearFn) {
  return function (delay, value, options) {
    if (typeof delay !== 'number') delay = 1

    const queue = []
    let waiting = null
    let stopped = false
    let id = null

    id = intervalFn(function () {
      if (waiting !== null) {
        const w = waiting
        waiting = null
        w({ value, done: false })
      } else {
        queue.push({ value, done: false })
      }
    }, delay)

    const iterator = {}
    iterator.next = function () {
      if (queue.length > 0) return Promise.resolve(queue.shift())
      if (stopped) return Promise.resolve({ value: undefined, done: true })
      return new Promise(function (resolve) { waiting = resolve })
    }
    iterator.return = function () {
      stopped = true
      if (id !== null) {
        clearFn(id)
        id = null
      }
      if (waiting !== null) {
        const w = waiting
        waiting = null
        w({ value: undefined, done: true })
      }
      return Promise.resolve({ value: undefined, done: true })
    }
    iterator[Symbol.asyncIterator] = function () { return this }
    return iterator
  }
}

module.exports = {
  setTimeout: promisifyTimeout(timers.setTimeout),
  setImmediate: promisifyImmediate(timers.setImmediate),
  setInterval: makeSetInterval(timers.setInterval, timers.clearInterval),
  scheduler: {
    wait: promisifyTimeout(timers.setTimeout)
  }
}
