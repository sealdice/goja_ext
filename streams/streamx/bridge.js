/*
 * Web <-> classic stream bridge, backed by the canonical Web Streams exports
 * supplied lazily by the Go host and the bundled streamx engine.
 */

const stream = require('streamx')

function canonical() {
  return require('goja:stream/web')
}

function readableToWeb(readable, opts) {
  return canonical().ReadableStream.from(readable)
}

function writableToWeb(writable) {
  const streams = canonical()
  return new streams.WritableStream({
    write(chunk) {
      return new Promise(function (resolve, reject) {
        writable.write(chunk, function (err) {
          if (err) reject(err)
          else resolve()
        })
      })
    },
    close() {
      return new Promise(function (resolve, reject) {
        writable.end(function (err) {
          if (err) reject(err)
          else resolve()
        })
      })
    },
    abort(reason) {
      writable.destroy(reason)
      return Promise.resolve()
    }
  })
}

function readableFromWeb(webStream, opts) {
  const reader = webStream.getReader()
  const readable = new stream.Readable({
    read(cb) {
      reader.read().then(function (r) {
        if (r.done) {
          readable.push(null)
          cb(null)
        } else {
          readable.push(r.value)
          cb(null)
        }
      }, function (err) {
        cb(err)
      })
    }
  })
  if (opts.encoding) readable.setEncoding(opts.encoding)
  if (opts.signal) addAbortSignal(opts.signal, readable)
  return readable
}

function writableFromWeb(webStream, opts) {
  const writer = webStream.getWriter()
  const writable = new stream.Writable({
    write(data, cb) {
      writer.write(data).then(function () { cb(null) }, function (err) { cb(err) })
    },
    final(cb) {
      writer.close().then(function () { cb(null) }, function (err) { cb(err) })
    }
  })
  if (opts.signal) addAbortSignal(opts.signal, writable)
  return writable
}

function duplexFromWeb(readableStream, writableStream, opts) {
  const readable = readableFromWeb(readableStream, opts)
  const writable = writableFromWeb(writableStream, opts)
  const duplex = new stream.Duplex({
    read(cb) {
      const data = readable.read()
      if (data === null) {
        readable.once('readable', function () {
          const next = readable.read()
          if (next === null && readable.destroyed) duplex.push(null)
          else if (next !== null) duplex.push(next)
          cb(null)
        })
        readable.once('end', function () { duplex.push(null) })
        cb(null)
      } else {
        duplex.push(data)
        cb(null)
      }
    },
    write(data, cb) {
      writable.write(data, cb)
    }
  })
  if (opts.signal) addAbortSignal(opts.signal, duplex)
  return duplex
}

function duplexToWeb(duplex) {
  return {
    readable: readableToWeb(duplex),
    writable: writableToWeb(duplex)
  }
}

function addAbortSignal(signal, stream) {
  function onAbort() {
    stream.destroy(signal.reason)
  }
  if (signal.aborted) onAbort()
  else signal.addEventListener('abort', onAbort)
  return stream
}

module.exports = {
  readableToWeb,
  writableToWeb,
  duplexToWeb,
  readableFromWeb,
  writableFromWeb,
  duplexFromWeb
}
