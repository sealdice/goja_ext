var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};
var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};
var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);
var __async = (__this, __arguments, generator) => {
  return new Promise((resolve, reject) => {
    var fulfilled = (value) => {
      try {
        step(generator.next(value));
      } catch (e) {
        reject(e);
      }
    };
    var rejected = (value) => {
      try {
        step(generator.throw(value));
      } catch (e) {
        reject(e);
      }
    };
    var step = (x) => x.done ? resolve(x.value) : Promise.resolve(x.value).then(fulfilled, rejected);
    step((generator = generator.apply(__this, __arguments)).next());
  });
};

// fetch/internal/fetch_event_source/entry.js
var entry_exports = {};
__export(entry_exports, {
  EventStreamContentType: () => EventStreamContentType,
  fetchEventSource: () => fetchEventSource2
});
module.exports = __toCommonJS(entry_exports);

// fetch/internal/fetch_event_source/headless-shim.js
var AbortController = __gojaAbortController;
var TextDecoder = __gojaTextDecoder;
var document = __gojaDocument;
var window = __gojaWindow;

// fetch/internal/fetch_event_source/parse.js
function getBytes(stream, onChunk) {
  return __async(this, null, function* () {
    const reader = stream.getReader();
    let result;
    while (!(result = yield reader.read()).done) onChunk(result.value);
  });
}
function getLines(onLine) {
  let buffer;
  let position;
  let fieldLength;
  let discardTrailingNewline = false;
  return function onChunk(arr) {
    if (buffer === void 0) {
      buffer = arr;
      position = 0;
      fieldLength = -1;
    } else {
      buffer = concat(buffer, arr);
    }
    const bufferLength = buffer.length;
    let lineStart = 0;
    while (position < bufferLength) {
      if (discardTrailingNewline) {
        if (buffer[position] === 10) lineStart = ++position;
        discardTrailingNewline = false;
      }
      let lineEnd = -1;
      for (; position < bufferLength && lineEnd === -1; ++position) {
        switch (buffer[position]) {
          case 58:
            if (fieldLength === -1) fieldLength = position - lineStart;
            break;
          case 13:
            discardTrailingNewline = true;
          case 10:
            lineEnd = position;
            break;
        }
      }
      if (lineEnd === -1) break;
      onLine(buffer.subarray(lineStart, lineEnd), fieldLength);
      lineStart = position;
      fieldLength = -1;
    }
    if (lineStart === bufferLength) {
      buffer = void 0;
    } else if (lineStart !== 0) {
      buffer = buffer.subarray(lineStart);
      position -= lineStart;
    }
  };
}
function getMessages(onId, onRetry, onMessage) {
  let message = newMessage();
  let hasData = false;
  const decoder = new TextDecoder();
  return function onLine(line, fieldLength) {
    if (line.length === 0) {
      if (hasData) onMessage == null ? void 0 : onMessage(message);
      message = newMessage();
      hasData = false;
      return;
    }
    if (fieldLength <= 0) return;
    const field = decoder.decode(line.subarray(0, fieldLength));
    const valueOffset = fieldLength + (line[fieldLength + 1] === 32 ? 2 : 1);
    const value = decoder.decode(line.subarray(valueOffset));
    switch (field) {
      case "data":
        message.data = hasData ? message.data + "\n" + value : value;
        hasData = true;
        break;
      case "event":
        message.event = value;
        break;
      case "id":
        onId(message.id = value);
        break;
      case "retry": {
        const retry = parseInt(value, 10);
        if (!Number.isNaN(retry)) onRetry(message.retry = retry);
        break;
      }
    }
  };
}
function concat(a, b) {
  const result = new Uint8Array(a.length + b.length);
  result.set(a);
  result.set(b, a.length);
  return result;
}
function newMessage() {
  return { data: "", event: "", id: "", retry: void 0 };
}

// node_modules/@microsoft/fetch-event-source/lib/esm/fetch.js
var __rest = function(s, e) {
  var t = {};
  for (var p in s) if (Object.prototype.hasOwnProperty.call(s, p) && e.indexOf(p) < 0)
    t[p] = s[p];
  if (s != null && typeof Object.getOwnPropertySymbols === "function")
    for (var i = 0, p = Object.getOwnPropertySymbols(s); i < p.length; i++) {
      if (e.indexOf(p[i]) < 0 && Object.prototype.propertyIsEnumerable.call(s, p[i]))
        t[p[i]] = s[p[i]];
    }
  return t;
};
var EventStreamContentType = "text/event-stream";
var DefaultRetryInterval = 1e3;
var LastEventId = "last-event-id";
function fetchEventSource(input, _a) {
  var { signal: inputSignal, headers: inputHeaders, onopen: inputOnOpen, onmessage, onclose, onerror, openWhenHidden, fetch: inputFetch } = _a, rest = __rest(_a, ["signal", "headers", "onopen", "onmessage", "onclose", "onerror", "openWhenHidden", "fetch"]);
  return new Promise((resolve, reject) => {
    const headers = Object.assign({}, inputHeaders);
    if (!headers.accept) {
      headers.accept = EventStreamContentType;
    }
    let curRequestController;
    function onVisibilityChange() {
      curRequestController.abort();
      if (!document.hidden) {
        create();
      }
    }
    if (!openWhenHidden) {
      document.addEventListener("visibilitychange", onVisibilityChange);
    }
    let retryInterval = DefaultRetryInterval;
    let retryTimer = 0;
    function dispose() {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.clearTimeout(retryTimer);
      curRequestController.abort();
    }
    inputSignal === null || inputSignal === void 0 ? void 0 : inputSignal.addEventListener("abort", () => {
      dispose();
      resolve();
    });
    const fetch = inputFetch !== null && inputFetch !== void 0 ? inputFetch : window.fetch;
    const onopen = inputOnOpen !== null && inputOnOpen !== void 0 ? inputOnOpen : defaultOnOpen;
    function create() {
      return __async(this, null, function* () {
        var _a2;
        curRequestController = new AbortController();
        try {
          const response = yield fetch(input, Object.assign(Object.assign({}, rest), { headers, signal: curRequestController.signal }));
          yield onopen(response);
          yield getBytes(response.body, getLines(getMessages((id) => {
            if (id) {
              headers[LastEventId] = id;
            } else {
              delete headers[LastEventId];
            }
          }, (retry) => {
            retryInterval = retry;
          }, onmessage)));
          onclose === null || onclose === void 0 ? void 0 : onclose();
          dispose();
          resolve();
        } catch (err) {
          if (!curRequestController.signal.aborted) {
            try {
              const interval = (_a2 = onerror === null || onerror === void 0 ? void 0 : onerror(err)) !== null && _a2 !== void 0 ? _a2 : retryInterval;
              window.clearTimeout(retryTimer);
              retryTimer = window.setTimeout(create, interval);
            } catch (innerErr) {
              dispose();
              reject(innerErr);
            }
          }
        }
      });
    }
    create();
  });
}
function defaultOnOpen(response) {
  const contentType = response.headers.get("content-type");
  if (!(contentType === null || contentType === void 0 ? void 0 : contentType.startsWith(EventStreamContentType))) {
    throw new Error(`Expected content-type to be ${EventStreamContentType}, Actual: ${contentType}`);
  }
}

// fetch/internal/fetch_event_source/entry.js
function fetchEventSource2(input, options) {
  var _a;
  if ((_a = options.signal) == null ? void 0 : _a.aborted) return Promise.resolve();
  return fetchEventSource(input, options);
}
