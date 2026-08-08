const Headers = require('bare-fetch/headers')
const Body = require('bare-fetch/body')
const Request = require('bare-fetch/request')
const Response = require('bare-fetch/response')
const FetchError = require('bare-fetch/errors')
const formData = require('bare-form-data')

const { FormData, Blob, File } = formData

const cloneResponse = Response.prototype.clone
Response.prototype.clone = function clone() {
  const cloned = cloneResponse.call(this)
  cloned._urls = [...this._urls]
  return cloned
}

function defineFormDataMethod(name, method) {
  if (typeof FormData.prototype[name] === 'function') return
  Object.defineProperty(FormData.prototype, name, {
    value: method,
    writable: true,
    configurable: true
  })
}

defineFormDataMethod('entries', function entries() {
  return this[Symbol.iterator]()
})

defineFormDataMethod('keys', function* keys() {
  for (const [name] of this) yield name
})

defineFormDataMethod('values', function* values() {
  for (const [, value] of this) yield value
})

defineFormDataMethod('forEach', function forEach(callback, thisArg) {
  for (const [name, value] of this) callback.call(thisArg, value, name, this)
})

function createFetch(host) {
  return function fetch(input, init = {}) {
    let request
    try {
      request = new Request(input, init)
    } catch (err) {
      return Promise.reject(err)
    }

    if (request._agent !== null) {
      return Promise.reject(new TypeError('The agent option is not supported'))
    }

    return send(request)

    async function send(request) {
      const signal = request.signal
      if (signal && signal.aborted) throw signal.reason

      const body = await readBody(request, signal)
      if (signal && signal.aborted) throw signal.reason

      if (!request.headers.has('user-agent')) {
        request.headers.set('user-agent', 'goja_nodejs-fetch/1.0')
      }
      if (!request.headers.has('accept')) request.headers.set('accept', '*/*')

      let raw
      try {
        raw = await host.dispatch({
          url: request.url,
          method: request.method,
          headers: Array.from(request.headers),
          body,
          signal
        })
      } catch (err) {
        if (signal && signal.aborted) throw signal.reason
        throw FetchError.NETWORK_ERROR('Network error', err)
      }

      const response = new Response(raw.body, {
        status: raw.status,
        statusText: raw.statusText,
        headers: raw.headers
      })
      response._type = 'basic'
      response._urls = raw.urls.map((url) => new URL(url))
      return response
    }
  }
}

async function readBody(request, signal) {
  if (request.body === null) return null
  if (Body.isUnusable(request)) {
    throw FetchError.BODY_UNUSABLE('Body has already been consumed')
  }

  const reader = request.body.getReader()
  const chunks = []
  let length = 0
  let rejectAbort = null
  let abortPromise = null
  let onAbort = null

  if (signal) {
    abortPromise = new Promise((resolve, reject) => {
      rejectAbort = reject
    })
    onAbort = () => {
      const reason = signal.reason
      Promise.resolve(reader.cancel(reason)).catch(() => {})
      rejectAbort(reason)
    }
    signal.addEventListener('abort', onAbort)
  }

  try {
    while (true) {
      const item = abortPromise
        ? await Promise.race([reader.read(), abortPromise])
        : await reader.read()
      if (item.done) break
      const chunk = item.value
      chunks.push(chunk)
      length += chunk.byteLength
    }
  } finally {
    if (signal && onAbort && typeof signal.removeEventListener === 'function') {
      signal.removeEventListener('abort', onAbort)
    }
    try {
      reader.releaseLock()
    } catch {}
  }

  const result = new Uint8Array(length)
  let offset = 0
  for (const chunk of chunks) {
    result.set(chunk, offset)
    offset += chunk.byteLength
  }
  return result
}

const api = {
  Headers,
  Request,
  Response,
  FormData,
  Blob,
  File
}

Object.defineProperties(api, {
  _createFetch: { value: createFetch },
  _Body: { value: Body },
  _FetchError: { value: FetchError },
  _formData: { value: formData }
})

module.exports = api
