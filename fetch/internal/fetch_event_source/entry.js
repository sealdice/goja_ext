import {
  EventStreamContentType,
  fetchEventSource as upstreamFetchEventSource
} from '@microsoft/fetch-event-source'

function fetchEventSource(input, options) {
  if (options.signal?.aborted) return Promise.resolve()
  return upstreamFetchEventSource(input, options)
}

export { EventStreamContentType, fetchEventSource }
