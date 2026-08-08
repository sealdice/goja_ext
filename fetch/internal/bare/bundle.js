var __getOwnPropNames = Object.getOwnPropertyNames;
var __knownSymbol = (name, symbol) => (symbol = Symbol[name]) ? symbol : Symbol.for("Symbol." + name);
var __commonJS = (cb, mod) => function __require() {
  return mod || (0, cb[__getOwnPropNames(cb)[0]])((mod = { exports: {} }).exports, mod), mod.exports;
};
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
var __forAwait = (obj, it, method) => (it = obj[__knownSymbol("asyncIterator")]) ? it.call(obj) : (obj = obj[__knownSymbol("iterator")](), it = {}, method = (key, fn) => (fn = obj[key]) && (it[key] = (arg) => new Promise((yes, no, done) => (arg = fn.call(obj, arg), done = arg.done, Promise.resolve(arg.value).then((value) => yes({ value, done }), no)))), method("next"), method("return"), it);

// node_modules/bare-mime/index.js
var require_bare_mime = __commonJS({
  "node_modules/bare-mime/index.js"(exports2, module2) {
    var MIME = class {
      constructor(type, subtype, parameters = /* @__PURE__ */ new Map()) {
        this._type = type;
        this._subtype = subtype;
        this._parameters = parameters;
      }
      // https://mimesniff.spec.whatwg.org/#type
      get type() {
        return this._type;
      }
      // https://mimesniff.spec.whatwg.org/#subtype
      get subtype() {
        return this._subtype;
      }
      // https://mimesniff.spec.whatwg.org/#parameters
      get parameters() {
        return this._parameters;
      }
    };
    module2.exports = exports2 = MIME;
    exports2.parse = function parse(input) {
      input = input.replace(httpWhitespaceLeadingAndTrailing, "");
      let position = 0;
      let type = "";
      while (position < input.length && input[position] !== "/") {
        type += input[position++];
      }
      if (type === "" || !isHTTPTokenCodePoints(type)) return null;
      if (position >= input.length) return null;
      position++;
      let subtype = "";
      while (position < input.length && input[position] !== ";") {
        subtype += input[position++];
      }
      subtype = subtype.replace(httpWhitespaceTrailing, "");
      if (subtype === "" || !isHTTPTokenCodePoints(subtype)) return null;
      const mimeType = new MIME(type.toLowerCase(), subtype.toLowerCase());
      while (position < input.length) {
        position++;
        while (position < input.length && httpWhitespace.test(input[position])) {
          position++;
        }
        let parameterName = "";
        while (position < input.length && input[position] !== ";" && input[position] !== "=") {
          parameterName += input[position++];
        }
        parameterName = parameterName.toLowerCase();
        if (position < input.length && input[position] === ";") continue;
        if (position >= input.length) break;
        position++;
        let parameterValue;
        if (position < input.length && input[position] === '"') {
          parameterValue = collectHTTPQuotedString(input, position);
          position = parameterValue.position;
          parameterValue = parameterValue.value;
          while (position < input.length && input[position] !== ";") {
            position++;
          }
        } else {
          parameterValue = "";
          while (position < input.length && input[position] !== ";") {
            parameterValue += input[position++];
          }
          parameterValue = parameterValue.replace(httpWhitespaceTrailing, "");
          if (parameterValue === "") continue;
        }
        if (parameterName !== "" && isHTTPTokenCodePoints(parameterName) && isHTTPQuotedStringTokenCodePoints(parameterValue) && !mimeType._parameters.has(parameterName)) {
          mimeType._parameters.set(parameterName, parameterValue);
        }
      }
      return mimeType;
    };
    var httpWhitespace = /[\t\n\r ]/;
    var httpWhitespaceLeadingAndTrailing = /^[\t\n\r ]+|[\t\n\r ]+$/g;
    var httpWhitespaceTrailing = /[\t\n\r ]+$/;
    var httpTokenCodePoints = /^[!#$%&'*+\-.^_`|~A-Za-z0-9]+$/;
    var httpQuotedStringTokenCodePoints = /^[\t\x20-\x7e\x80-\xff]*$/;
    function isHTTPTokenCodePoints(s) {
      return httpTokenCodePoints.test(s);
    }
    function isHTTPQuotedStringTokenCodePoints(s) {
      return httpQuotedStringTokenCodePoints.test(s);
    }
    function collectHTTPQuotedString(input, position) {
      let value = "";
      position++;
      while (true) {
        while (position < input.length && input[position] !== '"' && input[position] !== "\\") {
          value += input[position++];
        }
        if (position >= input.length) break;
        const quoteOrBackslash = input[position++];
        if (quoteOrBackslash === "\\") {
          if (position >= input.length) {
            value += "\\";
            break;
          }
          value += input[position++];
        } else {
          break;
        }
      }
      return { value, position };
    }
  }
});

// node_modules/bare-fetch/lib/errors.js
var require_errors = __commonJS({
  "node_modules/bare-fetch/lib/errors.js"(exports2, module2) {
    module2.exports = class FetchError2 extends Error {
      constructor(msg, fn = FetchError2, code = fn.name, opts = {}) {
        if (typeof code === "object" && code !== null) {
          opts = code;
          code = fn.name;
        }
        super(`${code}: ${msg}`, opts);
        this.code = code;
        if (Error.captureStackTrace) Error.captureStackTrace(this, fn);
      }
      get name() {
        return "FetchError";
      }
      static INVALID_URL(msg, cause) {
        return new FetchError2(msg, FetchError2.INVALID_URL, { cause });
      }
      static INVALID_REDIRECT_STATUS(msg) {
        return new FetchError2(msg, FetchError2.INVALID_REDIRECT_STATUS);
      }
      static INVALID_JSON(msg, cause) {
        return new FetchError2(msg, FetchError2.INVALID_JSON, { cause });
      }
      static NETWORK_ERROR(msg, cause) {
        return new FetchError2(msg, FetchError2.NETWORK_ERROR, { cause });
      }
      static TOO_MANY_REDIRECTS(msg) {
        return new FetchError2(msg, FetchError2.TOO_MANY_REDIRECTS);
      }
      static UNKNOWN_PROTOCOL(msg) {
        return new FetchError2(msg, FetchError2.UNKNOWN_PROTOCOL);
      }
      static BODY_UNUSABLE(msg) {
        return new FetchError2(msg, FetchError2.BODY_UNUSABLE);
      }
      static INVALID_FORM_DATA(msg) {
        return new FetchError2(msg, FetchError2.INVALID_FORM_DATA);
      }
      static INVALID_METHOD(msg) {
        return new FetchError2(msg, FetchError2.INVALID_METHOD);
      }
      static FORBIDDEN_METHOD(msg) {
        return new FetchError2(msg, FetchError2.FORBIDDEN_METHOD);
      }
      static INVALID_HEADER_NAME(msg) {
        return new FetchError2(msg, FetchError2.INVALID_HEADER_NAME);
      }
      static INVALID_HEADER_VALUE(msg) {
        return new FetchError2(msg, FetchError2.INVALID_HEADER_VALUE);
      }
    };
  }
});

// node_modules/bare-fetch/lib/headers.js
var require_headers = __commonJS({
  "node_modules/bare-fetch/lib/headers.js"(exports2, module2) {
    var MIME = require_bare_mime();
    var errors = require_errors();
    var NUL = 0;
    var LF = 10;
    var CR = 13;
    var TOKEN_BYTES = Buffer.from([
      // 0x00-0x1f (control characters) + 0x20 (space)
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      // 0x21-0x41: ! " # $ % & ' ( ) * + , - . / 0-9 : ; < = > ? @ A
      1,
      0,
      1,
      1,
      1,
      1,
      1,
      0,
      0,
      1,
      1,
      0,
      1,
      1,
      0,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      0,
      0,
      0,
      0,
      0,
      0,
      0,
      1,
      // 0x42-0x62: B-Z [ \ ] ^ _ ` a b
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      0,
      0,
      0,
      1,
      1,
      1,
      1,
      1,
      // 0x63-0x7f: c-z { | } ~ DEL
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      1,
      0,
      1,
      0,
      1,
      0
    ]);
    function isTokenByte(b) {
      return b < TOKEN_BYTES.length && TOKEN_BYTES[b] === 1;
    }
    function isFieldByte(b) {
      return b !== NUL && b !== LF && b !== CR;
    }
    function validateName(name) {
      if (name.length === 0) {
        throw errors.INVALID_HEADER_NAME(`Header name must not be empty`);
      }
      for (let i = 0, n = name.length; i < n; i++) {
        if (!isTokenByte(name.charCodeAt(i))) {
          throw errors.INVALID_HEADER_NAME(`Invalid header name '${name}'`);
        }
      }
    }
    function validateValue(name, value) {
      for (let i = 0, n = value.length; i < n; i++) {
        if (!isFieldByte(value.charCodeAt(i))) {
          throw errors.INVALID_HEADER_VALUE(`Invalid header value for '${name}'`);
        }
      }
    }
    module2.exports = exports2 = class Headers {
      // https://fetch.spec.whatwg.org/#dom-headers
      constructor(init) {
        this._headers = /* @__PURE__ */ new Map();
        if (init) {
          for (const [key, value] of typeof init[Symbol.iterator] === "function" ? init : Object.entries(init)) {
            this.append(key, value);
          }
        }
      }
      // https://fetch.spec.whatwg.org/#dom-headers-append
      append(name, value) {
        value = value.trim();
        validateName(name);
        validateValue(name, value);
        name = name.toLowerCase();
        let list = this._headers.get(name);
        if (list === void 0) {
          list = [];
          this._headers.set(name, list);
        }
        list.push(value);
      }
      // https://fetch.spec.whatwg.org/#dom-headers-delete
      delete(name) {
        name = name.toLowerCase();
        this._headers.delete(name);
      }
      // https://fetch.spec.whatwg.org/#dom-headers-get
      get(name) {
        name = name.toLowerCase();
        const list = this._headers.get(name);
        if (list === void 0) return null;
        return list.join(", ");
      }
      // https://fetch.spec.whatwg.org/#dom-headers-has
      has(name) {
        name = name.toLowerCase();
        return this._headers.has(name);
      }
      getSetCookie() {
        const list = this._headers.get("set-cookie");
        if (list === void 0) return [];
        return [...list];
      }
      // https://fetch.spec.whatwg.org/#dom-headers-set
      set(name, value) {
        value = value.trim();
        validateName(name);
        validateValue(name, value);
        name = name.toLowerCase();
        this._headers.set(name, [value]);
      }
      // https://webidl.spec.whatwg.org/#idl-iterable
      entries() {
        return this[Symbol.iterator]();
      }
      // https://webidl.spec.whatwg.org/#idl-iterable
      *keys() {
        for (const [name] of this) {
          yield name;
        }
      }
      // https://webidl.spec.whatwg.org/#idl-iterable
      *values() {
        for (const [, value] of this) {
          yield value;
        }
      }
      // https://webidl.spec.whatwg.org/#idl-iterable
      forEach(callback, thisArg) {
        for (const [name, value] of this) {
          callback.call(thisArg, value, name, this);
        }
      }
      *[Symbol.iterator]() {
        const names = [...this._headers.keys()].sort();
        for (const name of names) {
          yield [name, this.get(name)];
        }
      }
    };
    exports2.extractMIMEType = function extractMIMEType(headers) {
      const contentType = headers.get("content-type");
      if (contentType === null) return null;
      const values2 = getDecodeSplit(contentType);
      let mimeType = null;
      for (const value of values2) {
        const parsed = MIME.parse(value);
        if (parsed !== null) {
          mimeType = parsed;
        }
      }
      return mimeType;
    };
    function getDecodeSplit(value) {
      const values2 = [];
      let current = "";
      let position = 0;
      while (position < value.length) {
        if (value[position] === '"') {
          current += value[position++];
          while (position < value.length) {
            if (value[position] === "\\" && position + 1 < value.length) {
              current += value[position++];
              current += value[position++];
            } else if (value[position] === '"') {
              current += value[position++];
              break;
            } else {
              current += value[position++];
            }
          }
        } else if (value[position] === ",") {
          values2.push(current.trim());
          current = "";
          position++;
        } else {
          current += value[position++];
        }
      }
      values2.push(current.trim());
      return values2;
    }
  }
});

// node_modules/bare-form-data/lib/errors.js
var require_errors2 = __commonJS({
  "node_modules/bare-form-data/lib/errors.js"(exports2, module2) {
    module2.exports = class FormDataError extends Error {
      constructor(msg, code, fn = FormDataError) {
        super(`${code}: ${msg}`);
        this.code = code;
        if (Error.captureStackTrace) {
          Error.captureStackTrace(this, fn);
        }
      }
      get name() {
        return "FormDataError";
      }
      static INVALID_MIME_TYPE(msg) {
        return new FormDataError(msg, "INVALID_MIME_TYPE", FormDataError.INVALID_MIME_TYPE);
      }
    };
  }
});

// node_modules/bare-form-data/index.js
var require_bare_form_data = __commonJS({
  "node_modules/bare-form-data/index.js"(exports2, module2) {
    var { ReadableStream } = require("goja:stream/web");
    var { isBuffer } = require("goja:buffer");
    var errors = require_errors2();
    var FormData2 = class _FormData {
      constructor() {
        this._entries = [];
      }
      append(name, value, filename) {
        if (typeof value !== "string") {
          if (!isFile(value) || filename) {
            value = new File2([value], filename || "blob", { type: value.type });
          }
        }
        this._entries.push([name, value]);
      }
      delete(name) {
        this._entries = this._entries.filter((entry) => entry[0] !== name);
      }
      get(name) {
        const entry = this._entries.find((entry2) => entry2[0] === name);
        return entry ? entry[1] : null;
      }
      getAll(name) {
        const entries2 = [];
        for (const entry of this._entries) {
          if (entry[0] === name) entries2.push(entry[1]);
        }
        return entries2;
      }
      has(name) {
        return this._entries.findIndex((entry) => entry[0] === name) !== -1;
      }
      set(name, value, filename) {
        this.delete(name);
        this.append(name, value, filename);
      }
      [Symbol.iterator]() {
        return this._entries[Symbol.iterator]();
      }
      [Symbol.for("bare.inspect")]() {
        return {
          __proto__: { constructor: _FormData },
          entries: this._entries
        };
      }
    };
    module2.exports = exports2 = FormData2;
    exports2.FormData = FormData2;
    function isFormData(value) {
      return value instanceof FormData2;
    }
    exports2.isFormData = isFormData;
    var Blob2 = class _Blob {
      // https://w3c.github.io/FileAPI/#dom-blob-blob
      constructor(parts, options = {}) {
        const { type = "" } = options;
        this._bytes = processBlobParts(parts);
        this._type = type;
      }
      // https://w3c.github.io/FileAPI/#dfn-size
      get size() {
        return this._bytes.byteLength;
      }
      // https://w3c.github.io/FileAPI/#dfn-type
      get type() {
        return this._type;
      }
      // https://w3c.github.io/FileAPI/#stream-method-algo
      stream() {
        const bytes = this._bytes;
        return new ReadableStream({
          start(controller) {
            controller.enqueue(bytes);
            controller.close();
          }
        });
      }
      buffer() {
        return __async(this, null, function* () {
          return Buffer.from(this._bytes);
        });
      }
      // https://w3c.github.io/FileAPI/#bytes-method-algo
      bytes() {
        return __async(this, null, function* () {
          return this.buffer();
        });
      }
      // https://w3c.github.io/FileAPI/#arraybuffer-method-algo
      arrayBuffer() {
        return __async(this, null, function* () {
          const buffer = new ArrayBuffer(this._bytes.byteLength);
          new Uint8Array(buffer).set(this._bytes);
          return buffer;
        });
      }
      // https://w3c.github.io/FileAPI/#text-method-algo
      text() {
        return __async(this, null, function* () {
          return this._bytes.toString();
        });
      }
      [Symbol.for("bare.inspect")]() {
        return {
          __proto__: { constructor: _Blob },
          size: this.size,
          type: this.type
        };
      }
    };
    exports2.Blob = Blob2;
    function isBlob(value) {
      return value instanceof Blob2;
    }
    exports2.isBlob = Blob2.isBlob = isBlob;
    var File2 = class _File extends Blob2 {
      // https://w3c.github.io/FileAPI/#dom-file-file
      constructor(parts, name, options = {}) {
        const { lastModified = Date.now() } = options;
        super(parts, options);
        this._name = name;
        this._lastModified = lastModified;
      }
      // https://w3c.github.io/FileAPI/#dfn-name
      get name() {
        return this._name;
      }
      // https://w3c.github.io/FileAPI/#dfn-lastModified
      get lastModified() {
        return this._lastModified;
      }
      [Symbol.for("bare.inspect")]() {
        return {
          __proto__: { constructor: _File },
          size: this.size,
          type: this.type,
          name: this.name,
          lastModified: this.lastModified
        };
      }
    };
    exports2.File = File2;
    function isFile(value) {
      return value instanceof File2;
    }
    exports2.isFile = File2.isFile = isFile;
    function processBlobParts(parts) {
      const chunks = [];
      for (const part of parts) {
        if (typeof part === "string") {
          const buffer = Buffer.from(part);
          if (parts.length === 1) return buffer;
          chunks.push(buffer);
        } else if (isBlob(part)) {
          chunks.push(part._bytes);
        } else if (isBuffer(part)) {
          chunks.push(part);
        } else if (ArrayBuffer.isView(part)) {
          chunks.push(Buffer.from(part.buffer, part.byteOffset, part.byteLength));
        } else {
          chunks.push(Buffer.from(part));
        }
      }
      return Buffer.concat(chunks);
    }
    function toBlob(formData2, mimeType = "multipart/form-data") {
      switch (mimeType) {
        case "multipart/form-data":
          return toMultipartBlob(formData2);
        default:
          throw errors.INVALID_MIME_TYPE(`Invalid MIME type '${mimeType}'`);
      }
    }
    exports2.toBlob = toBlob;
    function escape(value) {
      return value.replace(/\n/g, "%0A").replace(/\r/g, "%0D").replace(/"/g, "%22");
    }
    function normalizeLinefeeds(value) {
      return value.replace(/\r?\n|\r/g, "\r\n");
    }
    var linefeed = Buffer.from("\r\n");
    function toMultipartBlob(formData2) {
      const boundary = Math.random().toString(16).slice(2, 18).padStart(16, "0");
      const prefix = `--${boundary}\r
Content-Disposition: form-data`;
      const parts = [];
      for (const [name, value] of formData2) {
        if (typeof value === "string") {
          const chunk = Buffer.from(
            prefix + `; name="${escape(normalizeLinefeeds(name))}"\r
\r
${normalizeLinefeeds(value)}\r
`
          );
          parts.push(chunk);
        } else {
          const chunk = Buffer.from(
            prefix + `; name="${escape(normalizeLinefeeds(name))}"; filename="${escape(value.name)}"\r
Content-Type: ${value.type || "application/octet-stream"}\r
\r
`
          );
          parts.push(chunk, value, linefeed);
        }
      }
      parts.push(Buffer.from(`--${boundary}--\r
`));
      return new Blob2(parts, {
        type: "multipart/form-data; boundary=" + boundary
      });
    }
  }
});

// node_modules/bare-fetch/lib/body.js
var require_body = __commonJS({
  "node_modules/bare-fetch/lib/body.js"(exports2, module2) {
    var { ReadableStream, isReadableStream, isReadableStreamDisturbed } = require("goja:stream/web");
    var { isURLSearchParams } = require("goja:url");
    var { FormData: FormData2, File: File2, isFormData, isBlob } = require_bare_form_data();
    var Headers2 = require_headers();
    var errors = require_errors();
    var empty = Buffer.from(new ArrayBuffer(0));
    module2.exports = exports2 = class Body2 {
      constructor(body = null, type = null) {
        if (isReadableStream(body)) {
          if (isReadableStreamDisturbed(body) || body.locked) {
            throw errors.BODY_UNUSABLE("Body has already been consumed");
          }
        } else {
          if (typeof body === "string") body = Buffer.from(body);
          else if (isFormData(body)) body = FormData2.toBlob(body);
          else if (isURLSearchParams(body)) {
            body = body.toString();
            type = "application/x-www-form-urlencoded;charset=UTF-8";
          }
          if (isBlob(body)) {
            type = body.type || null;
            body = body.stream();
          } else if (body !== null) {
            if (ArrayBuffer.isView(body)) {
              body = Buffer.from(body.buffer, body.byteOffset, body.byteLength);
            } else {
              body = Buffer.from(body);
            }
            body = new ReadableStream({
              start(controller) {
                controller.enqueue(body);
                controller.close();
              }
            });
          }
        }
        this._contentType = type;
        this._body = body;
      }
      // https://fetch.spec.whatwg.org/#dom-body-body
      get body() {
        return this._body;
      }
      // https://fetch.spec.whatwg.org/#dom-body-bodyused
      get bodyUsed() {
        return this._body !== null && isReadableStreamDisturbed(this._body);
      }
      buffer() {
        return __async(this, null, function* () {
          if (this._body === null) return empty;
          if (Body2.isUnusable(this)) {
            throw errors.BODY_UNUSABLE("Body has already been consumed");
          }
          const chunks = [];
          let length = 0;
          try {
            for (var iter = __forAwait(this._body), more, temp, error; more = !(temp = yield iter.next()).done; more = false) {
              const chunk = temp.value;
              chunks.push(chunk);
              length += chunk.byteLength;
            }
          } catch (temp) {
            error = [temp];
          } finally {
            try {
              more && (temp = iter.return) && (yield temp.call(iter));
            } finally {
              if (error)
                throw error[0];
            }
          }
          const result = Buffer.from(new ArrayBuffer(length));
          let offset = 0;
          for (const chunk of chunks) {
            result.set(chunk, offset);
            offset += chunk.byteLength;
          }
          return result;
        });
      }
      // https://fetch.spec.whatwg.org/#dom-body-bytes
      bytes() {
        return __async(this, null, function* () {
          return yield this.buffer();
        });
      }
      // https://fetch.spec.whatwg.org/#dom-body-arraybuffer
      arrayBuffer() {
        return __async(this, null, function* () {
          return (yield this.buffer()).buffer;
        });
      }
      // https://fetch.spec.whatwg.org/#dom-body-text
      text() {
        return __async(this, null, function* () {
          return (yield this.buffer()).toString();
        });
      }
      // https://fetch.spec.whatwg.org/#dom-body-json
      json() {
        return __async(this, null, function* () {
          return JSON.parse(yield this.text());
        });
      }
      // https://fetch.spec.whatwg.org/#dom-body-formdata
      formData() {
        return __async(this, null, function* () {
          const mimeType = getMIMEType(this);
          if (mimeType !== null && mimeType.type === "multipart" && mimeType.subtype === "form-data") {
            const body = yield this.text();
            const boundary = mimeType.parameters.get("boundary");
            if (boundary === void 0) {
              throw errors.INVALID_FORM_DATA("Missing boundary parameter");
            }
            return parseMultipart(body, boundary);
          }
          if (mimeType !== null && mimeType.type === "application" && mimeType.subtype === "x-www-form-urlencoded") {
            const body = yield this.text();
            const formData2 = new FormData2();
            for (const [name, value] of new URLSearchParams(body)) {
              formData2.append(name, value);
            }
            return formData2;
          }
          throw errors.INVALID_FORM_DATA("Could not parse content as form data");
        });
      }
    };
    exports2.isUnusable = function isUsable(body) {
      return body._body !== null && (isReadableStreamDisturbed(body._body) || body._body.locked);
    };
    exports2.clone = function clone2(body) {
      if (body._body === null) return null;
      const [out1, out2] = body._body.tee();
      body._body = out1;
      return out2;
    };
    function getMIMEType(body) {
      const headers = body._headers;
      if (headers === void 0) return null;
      return Headers2.extractMIMEType(headers);
    }
    function parseMultipart(input, boundary) {
      const formData2 = new FormData2();
      const delimiter = `\r
--${boundary}`;
      const close = `\r
--${boundary}--`;
      let start = input.indexOf(`--${boundary}`);
      if (start === -1) return formData2;
      start += `--${boundary}`.length;
      while (true) {
        if (input.startsWith("--", start)) break;
        if (input.startsWith("\r\n", start)) start += 2;
        let end = input.indexOf(delimiter, start);
        if (end === -1) end = input.indexOf(close, start);
        if (end === -1) break;
        const part = input.slice(start, end);
        const headerEnd = part.indexOf("\r\n\r\n");
        if (headerEnd === -1) {
          start = end + delimiter.length;
          continue;
        }
        const headerSection = part.slice(0, headerEnd);
        const body = part.slice(headerEnd + 4);
        const contentDisposition = { name: null, filename: null };
        let contentType = null;
        for (const line of headerSection.split("\r\n")) {
          const colon = line.indexOf(":");
          if (colon === -1) continue;
          const name = line.slice(0, colon).trim().toLowerCase();
          const value = line.slice(colon + 1).trim();
          if (name === "content-disposition") {
            contentDisposition.name = getParameter(value, "name");
            contentDisposition.filename = getParameter(value, "filename");
          } else if (name === "content-type") {
            contentType = value;
          }
        }
        if (contentDisposition.name === null) {
          start = end + delimiter.length;
          continue;
        }
        if (contentDisposition.filename !== null) {
          const file = new File2([body], contentDisposition.filename, {
            type: contentType || "application/octet-stream"
          });
          formData2.append(contentDisposition.name, file);
        } else {
          formData2.append(contentDisposition.name, body);
        }
        start = end + delimiter.length;
      }
      return formData2;
    }
    function getParameter(header, name) {
      const pattern = new RegExp(`${name}="([^"]*)"|${name}=([^;\\s]*)`, "i");
      const match = header.match(pattern);
      if (match === null) return null;
      return match[1] !== void 0 ? match[1] : match[2];
    }
  }
});

// node_modules/bare-fetch/lib/request.js
var require_request = __commonJS({
  "node_modules/bare-fetch/lib/request.js"(exports2, module2) {
    var { isURL } = require("goja:url");
    var Body2 = require_body();
    var Headers2 = require_headers();
    var errors = require_errors();
    module2.exports = class Request2 extends Body2 {
      // https://fetch.spec.whatwg.org/#dom-request
      constructor(input, init = {}) {
        let url;
        try {
          if (isURL(input)) {
            url = input;
            input = {};
          } else if (typeof input === "string") {
            url = new URL(input);
            input = {};
          } else {
            url = new URL(input.url);
          }
        } catch (err) {
          throw errors.INVALID_URL("Invalid URL", err);
        }
        const {
          body = input.body || null,
          method = input.method || "GET",
          headers = input.headers,
          signal = input.signal || null,
          agent = input.agent || null
        } = init;
        super(body);
        this._url = url;
        if (!/^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/.test(method)) {
          throw errors.INVALID_METHOD(`'${method}' is not a valid method`);
        }
        if (/^(connect|trace|track)$/i.test(method)) {
          throw errors.FORBIDDEN_METHOD(`'${method}' is a forbidden method`);
        }
        this._method = /^(delete|get|head|options|post|put)$/i.test(method) ? method.toUpperCase() : method;
        this._headers = new Headers2(headers);
        this._signal = signal;
        this._agent = agent;
        if (this._contentType !== null && !this._headers.has("content-type")) {
          this._headers.set("content-type", this._contentType);
        }
      }
      // https://fetch.spec.whatwg.org/#dom-request-url
      get url() {
        return this._url.href;
      }
      // https://fetch.spec.whatwg.org/#dom-request-method
      get method() {
        return this._method;
      }
      // https://fetch.spec.whatwg.org/#dom-request-headers
      get headers() {
        return this._headers;
      }
      // https://fetch.spec.whatwg.org/#dom-request-signal
      get signal() {
        return this._signal;
      }
      // https://fetch.spec.whatwg.org/#dom-request-clone
      clone() {
        if (Body2.isUnusable(this)) throw errors.BODY_UNUSABLE("Body has already been consumed");
        return new Request2(this, { body: Body2.clone(this) });
      }
    };
  }
});

// node_modules/bare-fetch/lib/response.js
var require_response = __commonJS({
  "node_modules/bare-fetch/lib/response.js"(exports2, module2) {
    var Body2 = require_body();
    var Headers2 = require_headers();
    var errors = require_errors();
    module2.exports = class Response2 extends Body2 {
      // https://fetch.spec.whatwg.org/#dom-response
      constructor(body = null, init = {}) {
        const { status = 200, statusText = "", headers } = init;
        super(body);
        this._urls = [];
        this._type = "default";
        this._status = status;
        this._statusText = statusText;
        this._headers = new Headers2(headers);
        if (this._contentType !== null && !this._headers.has("content-type")) {
          this._headers.set("content-type", this._contentType);
        }
      }
      // https://fetch.spec.whatwg.org/#dom-response-type
      get type() {
        return this._type;
      }
      // https://fetch.spec.whatwg.org/#dom-response-url
      get url() {
        return this._urls.length === 0 ? null : this._urls[this._urls.length - 1].href;
      }
      // https://fetch.spec.whatwg.org/#dom-response-redirected
      get redirected() {
        return this._urls.length > 1;
      }
      // https://fetch.spec.whatwg.org/#dom-response-status
      get status() {
        return this._status;
      }
      // https://fetch.spec.whatwg.org/#dom-response-ok
      get ok() {
        return this._status >= 200 && this._status <= 299;
      }
      // https://fetch.spec.whatwg.org/#dom-response-statustext
      get statusText() {
        return this._statusText;
      }
      // https://fetch.spec.whatwg.org/#dom-response-headers
      get headers() {
        return this._headers;
      }
      // https://fetch.spec.whatwg.org/#dom-response-clone
      clone() {
        if (Body2.isUnusable(this)) throw errors.BODY_UNUSABLE("Body has already been consumed");
        const cloned = new Response2(Body2.clone(this), this);
        cloned._type = this._type;
        return cloned;
      }
      // https://fetch.spec.whatwg.org/#dom-response-error
      static error() {
        const response = new Response2(null);
        response._type = "error";
        response._status = 0;
        response._statusText = "";
        return response;
      }
      // https://fetch.spec.whatwg.org/#dom-response-redirect
      static redirect(url, status = 302) {
        let parsed;
        try {
          parsed = new URL(url);
        } catch (err) {
          throw errors.INVALID_URL("Invalid URL", err);
        }
        if (!isRedirectStatus(status)) {
          throw errors.INVALID_REDIRECT_STATUS(`'${status}' is not a redirect status`);
        }
        const response = new Response2(null, { status });
        response._headers.set("location", parsed.href);
        return response;
      }
      // https://fetch.spec.whatwg.org/#dom-response-json
      static json(data, init = {}) {
        let body;
        try {
          body = JSON.stringify(data);
        } catch (err) {
          throw errors.INVALID_JSON("Data could not be serialized to JSON", err);
        }
        if (body === void 0) {
          throw errors.INVALID_JSON("Data could not be serialized to JSON");
        }
        const response = new Response2(body, init);
        if (!response._headers.has("content-type")) {
          response._headers.set("content-type", "application/json");
        }
        return response;
      }
    };
    function isRedirectStatus(status) {
      return status === 301 || status === 302 || status === 303 || status === 307 || status === 308;
    }
  }
});

// fetch/internal/bare/facade.js
var Headers = require_headers();
var Body = require_body();
var Request = require_request();
var Response = require_response();
var FetchError = require_errors();
var formData = require_bare_form_data();
var { FormData, Blob, File } = formData;
var cloneResponse = Response.prototype.clone;
Response.prototype.clone = function clone() {
  const cloned = cloneResponse.call(this);
  cloned._urls = [...this._urls];
  return cloned;
};
function defineFormDataMethod(name, method) {
  if (typeof FormData.prototype[name] === "function") return;
  Object.defineProperty(FormData.prototype, name, {
    value: method,
    writable: true,
    configurable: true
  });
}
defineFormDataMethod("entries", function entries() {
  return this[Symbol.iterator]();
});
defineFormDataMethod("keys", function* keys() {
  for (const [name] of this) yield name;
});
defineFormDataMethod("values", function* values() {
  for (const [, value] of this) yield value;
});
defineFormDataMethod("forEach", function forEach(callback, thisArg) {
  for (const [name, value] of this) callback.call(thisArg, value, name, this);
});
function createFetch(host) {
  return function fetch(input, init = {}) {
    let request;
    try {
      request = new Request(input, init);
    } catch (err) {
      return Promise.reject(err);
    }
    if (request._agent !== null) {
      return Promise.reject(new TypeError("The agent option is not supported"));
    }
    return send(request);
    function send(request2) {
      return __async(this, null, function* () {
        const signal = request2.signal;
        if (signal && signal.aborted) throw signal.reason;
        const body = yield readBody(request2, signal);
        if (signal && signal.aborted) throw signal.reason;
        if (!request2.headers.has("user-agent")) {
          request2.headers.set("user-agent", "goja_nodejs-fetch/1.0");
        }
        if (!request2.headers.has("accept")) request2.headers.set("accept", "*/*");
        let raw;
        try {
          raw = yield host.dispatch({
            url: request2.url,
            method: request2.method,
            headers: Array.from(request2.headers),
            body,
            signal
          });
        } catch (err) {
          if (signal && signal.aborted) throw signal.reason;
          throw FetchError.NETWORK_ERROR("Network error", err);
        }
        const response = new Response(raw.body, {
          status: raw.status,
          statusText: raw.statusText,
          headers: raw.headers
        });
        response._type = "basic";
        response._urls = raw.urls.map((url) => new URL(url));
        return response;
      });
    }
  };
}
function readBody(request, signal) {
  return __async(this, null, function* () {
    if (request.body === null) return null;
    if (Body.isUnusable(request)) {
      throw FetchError.BODY_UNUSABLE("Body has already been consumed");
    }
    const reader = request.body.getReader();
    const chunks = [];
    let length = 0;
    let rejectAbort = null;
    let abortPromise = null;
    let onAbort = null;
    if (signal) {
      abortPromise = new Promise((resolve, reject) => {
        rejectAbort = reject;
      });
      onAbort = () => {
        const reason = signal.reason;
        Promise.resolve(reader.cancel(reason)).catch(() => {
        });
        rejectAbort(reason);
      };
      signal.addEventListener("abort", onAbort);
    }
    try {
      while (true) {
        const item = abortPromise ? yield Promise.race([reader.read(), abortPromise]) : yield reader.read();
        if (item.done) break;
        const chunk = item.value;
        chunks.push(chunk);
        length += chunk.byteLength;
      }
    } finally {
      if (signal && onAbort && typeof signal.removeEventListener === "function") {
        signal.removeEventListener("abort", onAbort);
      }
      try {
        reader.releaseLock();
      } catch (e) {
      }
    }
    const result = new Uint8Array(length);
    let offset = 0;
    for (const chunk of chunks) {
      result.set(chunk, offset);
      offset += chunk.byteLength;
    }
    return result;
  });
}
var api = {
  Headers,
  Request,
  Response,
  FormData,
  Blob,
  File
};
Object.defineProperties(api, {
  _createFetch: { value: createFetch },
  _Body: { value: Body },
  _FetchError: { value: FetchError },
  _formData: { value: formData }
});
module.exports = api;
