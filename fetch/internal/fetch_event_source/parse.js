export async function getBytes(stream, onChunk) {
  const reader = stream.getReader()
  let result
  while (!(result = await reader.read()).done) onChunk(result.value)
}

export function getLines(onLine) {
  let buffer
  let position
  let fieldLength
  let discardTrailingNewline = false

  return function onChunk(arr) {
    if (buffer === undefined) {
      buffer = arr
      position = 0
      fieldLength = -1
    } else {
      buffer = concat(buffer, arr)
    }

    const bufferLength = buffer.length
    let lineStart = 0
    while (position < bufferLength) {
      if (discardTrailingNewline) {
        if (buffer[position] === 10) lineStart = ++position
        discardTrailingNewline = false
      }

      let lineEnd = -1
      for (; position < bufferLength && lineEnd === -1; ++position) {
        switch (buffer[position]) {
          case 58:
            if (fieldLength === -1) fieldLength = position - lineStart
            break
          case 13:
            discardTrailingNewline = true
          case 10:
            lineEnd = position
            break
        }
      }

      if (lineEnd === -1) break
      onLine(buffer.subarray(lineStart, lineEnd), fieldLength)
      lineStart = position
      fieldLength = -1
    }

    if (lineStart === bufferLength) {
      buffer = undefined
    } else if (lineStart !== 0) {
      buffer = buffer.subarray(lineStart)
      position -= lineStart
    }
  }
}

export function getMessages(onId, onRetry, onMessage) {
  let message = newMessage()
  let hasData = false
  const decoder = new TextDecoder()

  return function onLine(line, fieldLength) {
    if (line.length === 0) {
      if (hasData) onMessage?.(message)
      message = newMessage()
      hasData = false
      return
    }
    if (fieldLength <= 0) return

    const field = decoder.decode(line.subarray(0, fieldLength))
    const valueOffset = fieldLength + (line[fieldLength + 1] === 32 ? 2 : 1)
    const value = decoder.decode(line.subarray(valueOffset))
    switch (field) {
      case 'data':
        message.data = hasData ? message.data + '\n' + value : value
        hasData = true
        break
      case 'event':
        message.event = value
        break
      case 'id':
        onId(message.id = value)
        break
      case 'retry': {
        const retry = parseInt(value, 10)
        if (!Number.isNaN(retry)) onRetry(message.retry = retry)
        break
      }
    }
  }
}

function concat(a, b) {
  const result = new Uint8Array(a.length + b.length)
  result.set(a)
  result.set(b, a.length)
  return result
}

function newMessage() {
  return { data: '', event: '', id: '', retry: undefined }
}
