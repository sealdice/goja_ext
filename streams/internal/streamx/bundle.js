var __defProp = Object.defineProperty;
var __defProps = Object.defineProperties;
var __getOwnPropDescs = Object.getOwnPropertyDescriptors;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __getOwnPropSymbols = Object.getOwnPropertySymbols;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __propIsEnum = Object.prototype.propertyIsEnumerable;
var __defNormalProp = (obj, key, value) => key in obj ? __defProp(obj, key, { enumerable: true, configurable: true, writable: true, value }) : obj[key] = value;
var __spreadValues = (a, b) => {
  for (var prop in b || (b = {}))
    if (__hasOwnProp.call(b, prop))
      __defNormalProp(a, prop, b[prop]);
  if (__getOwnPropSymbols)
    for (var prop of __getOwnPropSymbols(b)) {
      if (__propIsEnum.call(b, prop))
        __defNormalProp(a, prop, b[prop]);
    }
  return a;
};
var __spreadProps = (a, b) => __defProps(a, __getOwnPropDescs(b));
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

// node_modules/b4a/lib/ascii.js
var require_ascii = __commonJS({
  "node_modules/b4a/lib/ascii.js"(exports2, module2) {
    function byteLength(string) {
      return string.length;
    }
    function toString(buffer) {
      const len = buffer.byteLength;
      let result = "";
      for (let i = 0; i < len; i++) {
        result += String.fromCharCode(buffer[i] & 127);
      }
      return result;
    }
    function write2(buffer, string) {
      const len = buffer.byteLength;
      for (let i = 0; i < len; i++) {
        buffer[i] = string.charCodeAt(i);
      }
      return len;
    }
    module2.exports = {
      byteLength,
      toString,
      write: write2
    };
  }
});

// node_modules/b4a/lib/base64.js
var require_base64 = __commonJS({
  "node_modules/b4a/lib/base64.js"(exports2, module2) {
    var alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    var codes = new Uint8Array(256);
    for (let i = 0; i < alphabet.length; i++) {
      codes[alphabet.charCodeAt(i)] = i;
    }
    codes[
      /* - */
      45
    ] = 62;
    codes[
      /* _ */
      95
    ] = 63;
    function byteLength(string) {
      let len = string.length;
      if (string.charCodeAt(len - 1) === 61) len--;
      if (len > 1 && string.charCodeAt(len - 1) === 61) len--;
      return len * 3 >>> 2;
    }
    function toString(buffer) {
      const len = buffer.byteLength;
      let result = "";
      for (let i = 0; i < len; i += 3) {
        result += alphabet[buffer[i] >> 2] + alphabet[(buffer[i] & 3) << 4 | buffer[i + 1] >> 4] + alphabet[(buffer[i + 1] & 15) << 2 | buffer[i + 2] >> 6] + alphabet[buffer[i + 2] & 63];
      }
      if (len % 3 === 2) {
        result = result.substring(0, result.length - 1) + "=";
      } else if (len % 3 === 1) {
        result = result.substring(0, result.length - 2) + "==";
      }
      return result;
    }
    function write2(buffer, string) {
      const len = buffer.byteLength;
      for (let i = 0, j = 0; j < len; i += 4) {
        const a = codes[string.charCodeAt(i)];
        const b = codes[string.charCodeAt(i + 1)];
        const c = codes[string.charCodeAt(i + 2)];
        const d = codes[string.charCodeAt(i + 3)];
        buffer[j++] = a << 2 | b >> 4;
        buffer[j++] = (b & 15) << 4 | c >> 2;
        buffer[j++] = (c & 3) << 6 | d & 63;
      }
      return len;
    }
    module2.exports = {
      byteLength,
      toString,
      write: write2
    };
  }
});

// node_modules/b4a/lib/hex.js
var require_hex = __commonJS({
  "node_modules/b4a/lib/hex.js"(exports2, module2) {
    function byteLength(string) {
      return string.length >>> 1;
    }
    function toString(buffer) {
      const len = buffer.byteLength;
      buffer = new DataView(buffer.buffer, buffer.byteOffset, len);
      let result = "";
      let i = 0;
      for (let n = len - len % 4; i < n; i += 4) {
        result += buffer.getUint32(i).toString(16).padStart(8, "0");
      }
      for (; i < len; i++) {
        result += buffer.getUint8(i).toString(16).padStart(2, "0");
      }
      return result;
    }
    function write2(buffer, string) {
      const len = buffer.byteLength;
      for (let i = 0; i < len; i++) {
        const a = hexValue(string.charCodeAt(i * 2));
        const b = hexValue(string.charCodeAt(i * 2 + 1));
        if (a === void 0 || b === void 0) {
          return i;
        }
        buffer[i] = a << 4 | b;
      }
      return len;
    }
    module2.exports = {
      byteLength,
      toString,
      write: write2
    };
    function hexValue(char) {
      if (char >= 48 && char <= 57) return char - 48;
      if (char >= 65 && char <= 70) return char - 65 + 10;
      if (char >= 97 && char <= 102) return char - 97 + 10;
    }
  }
});

// node_modules/b4a/lib/latin1.js
var require_latin1 = __commonJS({
  "node_modules/b4a/lib/latin1.js"(exports2, module2) {
    function byteLength(string) {
      return string.length;
    }
    function toString(buffer) {
      const len = buffer.byteLength;
      let result = "";
      for (let i = 0; i < len; i++) {
        result += String.fromCharCode(buffer[i]);
      }
      return result;
    }
    function write2(buffer, string) {
      const len = buffer.byteLength;
      for (let i = 0; i < len; i++) {
        buffer[i] = string.charCodeAt(i);
      }
      return len;
    }
    module2.exports = {
      byteLength,
      toString,
      write: write2
    };
  }
});

// node_modules/b4a/lib/utf8.js
var require_utf8 = __commonJS({
  "node_modules/b4a/lib/utf8.js"(exports2, module2) {
    function byteLength(string) {
      let length = 0;
      for (let i = 0, n = string.length; i < n; i++) {
        const code = string.charCodeAt(i);
        if (code >= 55296 && code <= 56319 && i + 1 < n) {
          const code2 = string.charCodeAt(i + 1);
          if (code2 >= 56320 && code2 <= 57343) {
            length += 4;
            i++;
            continue;
          }
        }
        if (code <= 127) length += 1;
        else if (code <= 2047) length += 2;
        else length += 3;
      }
      return length;
    }
    var toString;
    if (typeof TextDecoder !== "undefined") {
      const decoder = new TextDecoder();
      toString = function toString2(buffer) {
        return decoder.decode(buffer);
      };
    } else {
      toString = function toString2(buffer) {
        const len = buffer.byteLength;
        let output = "";
        let i = 0;
        while (i < len) {
          let byte = buffer[i];
          if (byte <= 127) {
            output += String.fromCharCode(byte);
            i++;
            continue;
          }
          let bytesNeeded = 0;
          let codePoint = 0;
          if (byte <= 223) {
            bytesNeeded = 1;
            codePoint = byte & 31;
          } else if (byte <= 239) {
            bytesNeeded = 2;
            codePoint = byte & 15;
          } else if (byte <= 244) {
            bytesNeeded = 3;
            codePoint = byte & 7;
          }
          if (len - i - bytesNeeded > 0) {
            let k = 0;
            while (k < bytesNeeded) {
              byte = buffer[i + k + 1];
              codePoint = codePoint << 6 | byte & 63;
              k += 1;
            }
          } else {
            codePoint = 65533;
            bytesNeeded = len - i;
          }
          output += String.fromCodePoint(codePoint);
          i += bytesNeeded + 1;
        }
        return output;
      };
    }
    var write2;
    if (typeof TextEncoder !== "undefined") {
      const encoder = new TextEncoder();
      write2 = function write3(buffer, string) {
        return encoder.encodeInto(string, buffer).written;
      };
    } else {
      write2 = function write3(buffer, string) {
        const len = buffer.byteLength;
        let i = 0;
        let j = 0;
        while (i < string.length) {
          const code = string.codePointAt(i);
          if (code <= 127) {
            if (j + 1 > len) break;
            buffer[j++] = code;
            i++;
            continue;
          }
          let count = 0;
          let bits = 0;
          if (code <= 2047) {
            count = 6;
            bits = 192;
          } else if (code <= 65535) {
            count = 12;
            bits = 224;
          } else if (code <= 2097151) {
            count = 18;
            bits = 240;
          }
          if (j + count / 6 + 1 > len) break;
          buffer[j++] = bits | code >> count;
          count -= 6;
          while (count >= 0) {
            buffer[j++] = 128 | code >> count & 63;
            count -= 6;
          }
          i += code >= 65536 ? 2 : 1;
        }
        return j;
      };
    }
    module2.exports = {
      byteLength,
      toString,
      write: write2
    };
  }
});

// node_modules/b4a/lib/utf16le.js
var require_utf16le = __commonJS({
  "node_modules/b4a/lib/utf16le.js"(exports2, module2) {
    function byteLength(string) {
      return string.length * 2;
    }
    function toString(buffer) {
      const len = buffer.byteLength;
      let result = "";
      for (let i = 0; i < len - 1; i += 2) {
        result += String.fromCharCode(buffer[i] + buffer[i + 1] * 256);
      }
      return result;
    }
    function write2(buffer, string) {
      const len = buffer.byteLength;
      let units = len;
      for (let i = 0; i < string.length; ++i) {
        if ((units -= 2) < 0) break;
        const c = string.charCodeAt(i);
        const hi = c >> 8;
        const lo = c % 256;
        buffer[i * 2] = lo;
        buffer[i * 2 + 1] = hi;
      }
      return len;
    }
    module2.exports = {
      byteLength,
      toString,
      write: write2
    };
  }
});

// node_modules/b4a/browser.js
var require_browser = __commonJS({
  "node_modules/b4a/browser.js"(exports2, module2) {
    var ascii = require_ascii();
    var base64 = require_base64();
    var hex = require_hex();
    var latin1 = require_latin1();
    var utf8 = require_utf8();
    var utf16le = require_utf16le();
    var LE = new Uint8Array(Uint16Array.of(255).buffer)[0] === 255;
    function codecFor(encoding) {
      switch (encoding) {
        case "ascii":
          return ascii;
        case "base64":
          return base64;
        case "hex":
          return hex;
        case "binary":
        case "latin1":
          return latin1;
        case "utf8":
        case "utf-8":
        case void 0:
        case null:
          return utf8;
        case "ucs2":
        case "ucs-2":
        case "utf16le":
        case "utf-16le":
          return utf16le;
        default:
          throw new Error(`Unknown encoding '${encoding}'`);
      }
    }
    function isBuffer(value) {
      return value instanceof Uint8Array;
    }
    function isEncoding(encoding) {
      try {
        codecFor(encoding);
        return true;
      } catch (e) {
        return false;
      }
    }
    function alloc(size, fill2, encoding) {
      const buffer = new Uint8Array(size);
      if (fill2 !== void 0) {
        exports2.fill(buffer, fill2, 0, buffer.byteLength, encoding);
      }
      return buffer;
    }
    function allocUnsafe(size) {
      return new Uint8Array(size);
    }
    function allocUnsafeSlow(size) {
      return new Uint8Array(size);
    }
    function byteLength(string, encoding) {
      return codecFor(encoding).byteLength(string);
    }
    function compare(a, b) {
      if (a === b) return 0;
      const len = Math.min(a.byteLength, b.byteLength);
      a = new DataView(a.buffer, a.byteOffset, a.byteLength);
      b = new DataView(b.buffer, b.byteOffset, b.byteLength);
      let i = 0;
      for (let n = len - len % 4; i < n; i += 4) {
        const x = a.getUint32(i, LE);
        const y = b.getUint32(i, LE);
        if (x !== y) break;
      }
      for (; i < len; i++) {
        const x = a.getUint8(i);
        const y = b.getUint8(i);
        if (x < y) return -1;
        if (x > y) return 1;
      }
      return a.byteLength > b.byteLength ? 1 : a.byteLength < b.byteLength ? -1 : 0;
    }
    function concat(buffers, length) {
      if (length === void 0) {
        length = buffers.reduce((len, buffer) => len + buffer.byteLength, 0);
      }
      const result = new Uint8Array(length);
      let offset = 0;
      for (const buffer of buffers) {
        if (offset + buffer.byteLength > result.byteLength) {
          result.set(buffer.subarray(0, result.byteLength - offset), offset);
          return result;
        }
        result.set(buffer, offset);
        offset += buffer.byteLength;
      }
      return result;
    }
    function copy(source, target, targetStart = 0, sourceStart = 0, sourceEnd = source.byteLength) {
      if (targetStart < 0) targetStart = 0;
      if (targetStart >= target.byteLength) return 0;
      const targetLength = target.byteLength - targetStart;
      if (sourceStart < 0) sourceStart = 0;
      if (sourceStart >= source.byteLength) return 0;
      if (sourceEnd <= sourceStart) return 0;
      if (sourceEnd > source.byteLength) sourceEnd = source.byteLength;
      if (sourceEnd - sourceStart > targetLength) {
        sourceEnd = sourceStart + targetLength;
      }
      const sourceLength = sourceEnd - sourceStart;
      if (source === target) {
        target.copyWithin(targetStart, sourceStart, sourceEnd);
      } else {
        if (sourceStart !== 0 || sourceEnd !== source.byteLength) {
          source = source.subarray(sourceStart, sourceEnd);
        }
        target.set(source, targetStart);
      }
      return sourceLength;
    }
    function equals(a, b) {
      if (a === b) return true;
      if (a.byteLength !== b.byteLength) return false;
      return compare(a, b) === 0;
    }
    function fill(buffer, value, offset = 0, end = buffer.byteLength, encoding = "utf8") {
      if (typeof value === "string") {
        if (typeof offset === "string") {
          encoding = offset;
          offset = 0;
          end = buffer.byteLength;
        } else if (typeof end === "string") {
          encoding = end;
          end = buffer.byteLength;
        }
      } else if (typeof value === "number") {
        value = value & 255;
      } else if (typeof value === "boolean") {
        value = +value;
      }
      if (offset < 0) offset = 0;
      if (offset >= buffer.byteLength) return buffer;
      if (end <= offset) return buffer;
      if (end > buffer.byteLength) end = buffer.byteLength;
      if (typeof value === "number") return buffer.fill(value, offset, end);
      if (typeof value === "string") value = exports2.from(value, encoding);
      const len = value.byteLength;
      for (let i = 0, n = end - offset; i < n; ++i) {
        buffer[i + offset] = value[i % len];
      }
      return buffer;
    }
    function from(value, encodingOrOffset, length) {
      if (typeof value === "string") return fromString(value, encodingOrOffset);
      if (Array.isArray(value)) return fromArray(value);
      if (ArrayBuffer.isView(value)) return fromBuffer(value);
      return fromArrayBuffer(value, encodingOrOffset, length);
    }
    function fromString(string, encoding) {
      const codec = codecFor(encoding);
      const buffer = new Uint8Array(codec.byteLength(string));
      codec.write(buffer, string);
      return buffer;
    }
    function fromArray(array) {
      const buffer = new Uint8Array(array.length);
      buffer.set(array);
      return buffer;
    }
    function fromBuffer(buffer) {
      const copy2 = new Uint8Array(buffer.byteLength);
      copy2.set(buffer);
      return copy2;
    }
    function fromArrayBuffer(arrayBuffer, byteOffset, length) {
      return new Uint8Array(arrayBuffer, byteOffset, length);
    }
    function includes(buffer, value, byteOffset, encoding) {
      return indexOf(buffer, value, byteOffset, encoding) !== -1;
    }
    function indexOf(buffer, value, byteOffset, encoding) {
      return bidirectionalIndexOf(
        buffer,
        value,
        byteOffset,
        encoding,
        true
        /* first */
      );
    }
    function lastIndexOf(buffer, value, byteOffset, encoding) {
      return bidirectionalIndexOf(
        buffer,
        value,
        byteOffset,
        encoding,
        false
        /* last */
      );
    }
    function bidirectionalIndexOf(buffer, value, byteOffset, encoding, first) {
      if (buffer.byteLength === 0) return -1;
      if (typeof byteOffset === "string") {
        encoding = byteOffset;
        byteOffset = 0;
      } else if (byteOffset === void 0) {
        byteOffset = first ? 0 : buffer.length - 1;
      } else if (byteOffset < 0) {
        byteOffset += buffer.byteLength;
      }
      if (byteOffset >= buffer.byteLength) {
        if (first) return -1;
        else byteOffset = buffer.byteLength - 1;
      } else if (byteOffset < 0) {
        if (first) byteOffset = 0;
        else return -1;
      }
      if (typeof value === "string") {
        value = from(value, encoding);
      } else if (typeof value === "number") {
        value = value & 255;
        if (first) {
          return buffer.indexOf(value, byteOffset);
        } else {
          return buffer.lastIndexOf(value, byteOffset);
        }
      }
      if (value.byteLength === 0) return -1;
      if (first) {
        let foundIndex = -1;
        for (let i = byteOffset; i < buffer.byteLength; i++) {
          if (buffer[i] === value[foundIndex === -1 ? 0 : i - foundIndex]) {
            if (foundIndex === -1) foundIndex = i;
            if (i - foundIndex + 1 === value.byteLength) return foundIndex;
          } else {
            if (foundIndex !== -1) i -= i - foundIndex;
            foundIndex = -1;
          }
        }
      } else {
        if (byteOffset + value.byteLength > buffer.byteLength) {
          byteOffset = buffer.byteLength - value.byteLength;
        }
        for (let i = byteOffset; i >= 0; i--) {
          let found = true;
          for (let j = 0; j < value.byteLength; j++) {
            if (buffer[i + j] !== value[j]) {
              found = false;
              break;
            }
          }
          if (found) return i;
        }
      }
      return -1;
    }
    function swap(buffer, n, m) {
      const i = buffer[n];
      buffer[n] = buffer[m];
      buffer[m] = i;
    }
    function swap16(buffer) {
      const len = buffer.byteLength;
      if (len % 2 !== 0) {
        throw new RangeError("Buffer size must be a multiple of 16-bits");
      }
      for (let i = 0; i < len; i += 2) swap(buffer, i, i + 1);
      return buffer;
    }
    function swap32(buffer) {
      const len = buffer.byteLength;
      if (len % 4 !== 0) {
        throw new RangeError("Buffer size must be a multiple of 32-bits");
      }
      for (let i = 0; i < len; i += 4) {
        swap(buffer, i, i + 3);
        swap(buffer, i + 1, i + 2);
      }
      return buffer;
    }
    function swap64(buffer) {
      const len = buffer.byteLength;
      if (len % 8 !== 0) {
        throw new RangeError("Buffer size must be a multiple of 64-bits");
      }
      for (let i = 0; i < len; i += 8) {
        swap(buffer, i, i + 7);
        swap(buffer, i + 1, i + 6);
        swap(buffer, i + 2, i + 5);
        swap(buffer, i + 3, i + 4);
      }
      return buffer;
    }
    function toBuffer(buffer) {
      return buffer;
    }
    function toString(buffer, encoding = "utf8", start = 0, end = buffer.byteLength) {
      if (arguments.length === 1) return utf8.toString(buffer);
      if (arguments.length === 2) return codecFor(encoding).toString(buffer);
      if (start < 0) start = 0;
      if (start >= buffer.byteLength) return "";
      if (end <= start) return "";
      if (end > buffer.byteLength) end = buffer.byteLength;
      if (start !== 0 || end !== buffer.byteLength) {
        buffer = buffer.subarray(start, end);
      }
      return codecFor(encoding).toString(buffer);
    }
    function write2(buffer, string, offset = 0, length = buffer.byteLength, encoding) {
      if (arguments.length === 2) return utf8.write(buffer, string);
      if (typeof offset === "string") {
        encoding = offset;
        offset = 0;
        length = buffer.byteLength;
      } else if (typeof length === "string") {
        encoding = length;
        length = buffer.byteLength - offset;
      }
      length = Math.min(length, exports2.byteLength(string, encoding));
      let start = offset;
      if (start < 0) start = 0;
      if (start >= buffer.byteLength) return 0;
      let end = offset + length;
      if (end <= start) return 0;
      if (end > buffer.byteLength) end = buffer.byteLength;
      if (start !== 0 || end !== buffer.byteLength) {
        buffer = buffer.subarray(start, end);
      }
      return codecFor(encoding).write(buffer, string);
    }
    function readDoubleBE(buffer, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      return view.getFloat64(offset, false);
    }
    function readDoubleLE(buffer, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      return view.getFloat64(offset, true);
    }
    function readFloatBE(buffer, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      return view.getFloat32(offset, false);
    }
    function readFloatLE(buffer, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      return view.getFloat32(offset, true);
    }
    function readInt32BE(buffer, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      return view.getInt32(offset, false);
    }
    function readInt32LE(buffer, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      return view.getInt32(offset, true);
    }
    function readUInt32BE(buffer, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      return view.getUint32(offset, false);
    }
    function readUInt32LE(buffer, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      return view.getUint32(offset, true);
    }
    function writeDoubleBE(buffer, value, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      view.setFloat64(offset, value, false);
      return offset + 8;
    }
    function writeDoubleLE(buffer, value, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      view.setFloat64(offset, value, true);
      return offset + 8;
    }
    function writeFloatBE(buffer, value, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      view.setFloat32(offset, value, false);
      return offset + 4;
    }
    function writeFloatLE(buffer, value, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      view.setFloat32(offset, value, true);
      return offset + 4;
    }
    function writeInt32BE(buffer, value, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      view.setInt32(offset, value, false);
      return offset + 4;
    }
    function writeInt32LE(buffer, value, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      view.setInt32(offset, value, true);
      return offset + 4;
    }
    function writeUInt32BE(buffer, value, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      view.setUint32(offset, value, false);
      return offset + 4;
    }
    function writeUInt32LE(buffer, value, offset = 0) {
      const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
      view.setUint32(offset, value, true);
      return offset + 4;
    }
    module2.exports = exports2 = {
      isBuffer,
      isEncoding,
      alloc,
      allocUnsafe,
      allocUnsafeSlow,
      byteLength,
      compare,
      concat,
      copy,
      equals,
      fill,
      from,
      includes,
      indexOf,
      lastIndexOf,
      swap16,
      swap32,
      swap64,
      toBuffer,
      toString,
      write: write2,
      readDoubleBE,
      readDoubleLE,
      readFloatBE,
      readFloatLE,
      readInt32BE,
      readInt32LE,
      readUInt32BE,
      readUInt32LE,
      writeDoubleBE,
      writeDoubleLE,
      writeFloatBE,
      writeFloatLE,
      writeInt32BE,
      writeInt32LE,
      writeUInt32BE,
      writeUInt32LE
    };
  }
});

// node_modules/fast-fifo/fixed-size.js
var require_fixed_size = __commonJS({
  "node_modules/fast-fifo/fixed-size.js"(exports2, module2) {
    module2.exports = class FixedFIFO {
      constructor(hwm) {
        if (!(hwm > 0) || (hwm - 1 & hwm) !== 0) throw new Error("Max size for a FixedFIFO should be a power of two");
        this.buffer = new Array(hwm);
        this.mask = hwm - 1;
        this.top = 0;
        this.btm = 0;
        this.next = null;
      }
      clear() {
        this.top = this.btm = 0;
        this.next = null;
        this.buffer.fill(void 0);
      }
      push(data) {
        if (this.buffer[this.top] !== void 0) return false;
        this.buffer[this.top] = data;
        this.top = this.top + 1 & this.mask;
        return true;
      }
      shift() {
        const last = this.buffer[this.btm];
        if (last === void 0) return void 0;
        this.buffer[this.btm] = void 0;
        this.btm = this.btm + 1 & this.mask;
        return last;
      }
      peek() {
        return this.buffer[this.btm];
      }
      isEmpty() {
        return this.buffer[this.btm] === void 0;
      }
    };
  }
});

// node_modules/fast-fifo/index.js
var require_fast_fifo = __commonJS({
  "node_modules/fast-fifo/index.js"(exports2, module2) {
    var FixedFIFO = require_fixed_size();
    module2.exports = class FastFIFO {
      constructor(hwm) {
        this.hwm = hwm || 16;
        this.head = new FixedFIFO(this.hwm);
        this.tail = this.head;
        this.length = 0;
      }
      clear() {
        this.head = this.tail;
        this.head.clear();
        this.length = 0;
      }
      push(val) {
        this.length++;
        if (!this.head.push(val)) {
          const prev = this.head;
          this.head = prev.next = new FixedFIFO(2 * this.head.buffer.length);
          this.head.push(val);
        }
      }
      shift() {
        if (this.length !== 0) this.length--;
        const val = this.tail.shift();
        if (val === void 0 && this.tail.next) {
          const next = this.tail.next;
          this.tail.next = null;
          this.tail = next;
          return this.tail.shift();
        }
        return val;
      }
      peek() {
        const val = this.tail.peek();
        if (val === void 0 && this.tail.next) return this.tail.next.peek();
        return val;
      }
      isEmpty() {
        return this.length === 0;
      }
    };
  }
});

// node_modules/text-decoder/lib/pass-through-decoder.js
var require_pass_through_decoder = __commonJS({
  "node_modules/text-decoder/lib/pass-through-decoder.js"(exports2, module2) {
    var b4a2 = require_browser();
    module2.exports = class PassThroughDecoder {
      constructor(encoding) {
        this.encoding = encoding;
      }
      get remaining() {
        return 0;
      }
      decode(data) {
        return b4a2.toString(data, this.encoding);
      }
      flush() {
        return "";
      }
    };
  }
});

// node_modules/text-decoder/lib/utf8-decoder.js
var require_utf8_decoder = __commonJS({
  "node_modules/text-decoder/lib/utf8-decoder.js"(exports2, module2) {
    var b4a2 = require_browser();
    module2.exports = class UTF8Decoder {
      constructor() {
        this._reset();
      }
      get remaining() {
        return this.bytesSeen;
      }
      decode(data) {
        if (data.byteLength === 0) return "";
        if (this.bytesNeeded === 0 && trailingIncomplete(data, 0) === 0) {
          this.bytesSeen = trailingBytesSeen(data);
          return b4a2.toString(data, "utf8");
        }
        let result = "";
        let start = 0;
        if (this.bytesNeeded > 0) {
          while (start < data.byteLength) {
            const byte = data[start];
            if (byte < this.lowerBoundary || byte > this.upperBoundary) {
              result += "\uFFFD";
              this._reset();
              break;
            }
            this.lowerBoundary = 128;
            this.upperBoundary = 191;
            this.codePoint = this.codePoint << 6 | byte & 63;
            this.bytesSeen++;
            start++;
            if (this.bytesSeen === this.bytesNeeded) {
              result += String.fromCodePoint(this.codePoint);
              this._reset();
              break;
            }
          }
          if (this.bytesNeeded > 0) return result;
        }
        const trailing = trailingIncomplete(data, start);
        const end = data.byteLength - trailing;
        if (end > start) result += b4a2.toString(data, "utf8", start, end);
        for (let i = end; i < data.byteLength; i++) {
          const byte = data[i];
          if (this.bytesNeeded === 0) {
            if (byte <= 127) {
              this.bytesSeen = 0;
              result += String.fromCharCode(byte);
            } else if (byte >= 194 && byte <= 223) {
              this.bytesNeeded = 2;
              this.bytesSeen = 1;
              this.codePoint = byte & 31;
            } else if (byte >= 224 && byte <= 239) {
              if (byte === 224) this.lowerBoundary = 160;
              else if (byte === 237) this.upperBoundary = 159;
              this.bytesNeeded = 3;
              this.bytesSeen = 1;
              this.codePoint = byte & 15;
            } else if (byte >= 240 && byte <= 244) {
              if (byte === 240) this.lowerBoundary = 144;
              else if (byte === 244) this.upperBoundary = 143;
              this.bytesNeeded = 4;
              this.bytesSeen = 1;
              this.codePoint = byte & 7;
            } else {
              this.bytesSeen = 1;
              result += "\uFFFD";
            }
            continue;
          }
          if (byte < this.lowerBoundary || byte > this.upperBoundary) {
            result += "\uFFFD";
            i--;
            this._reset();
            continue;
          }
          this.lowerBoundary = 128;
          this.upperBoundary = 191;
          this.codePoint = this.codePoint << 6 | byte & 63;
          this.bytesSeen++;
          if (this.bytesSeen === this.bytesNeeded) {
            result += String.fromCodePoint(this.codePoint);
            this._reset();
          }
        }
        return result;
      }
      flush() {
        const result = this.bytesNeeded > 0 ? "\uFFFD" : "";
        this._reset();
        return result;
      }
      _reset() {
        this.codePoint = 0;
        this.bytesNeeded = 0;
        this.bytesSeen = 0;
        this.lowerBoundary = 128;
        this.upperBoundary = 191;
      }
    };
    function trailingIncomplete(data, start) {
      const len = data.byteLength;
      if (len <= start) return 0;
      const limit = Math.max(start, len - 4);
      let i = len - 1;
      while (i > limit && (data[i] & 192) === 128) i--;
      if (i < start) return 0;
      const byte = data[i];
      let needed;
      if (byte <= 127) return 0;
      if (byte >= 194 && byte <= 223) needed = 2;
      else if (byte >= 224 && byte <= 239) needed = 3;
      else if (byte >= 240 && byte <= 244) needed = 4;
      else return 0;
      const available = len - i;
      return available < needed ? available : 0;
    }
    function trailingBytesSeen(data) {
      const len = data.byteLength;
      if (len === 0) return 0;
      const last = data[len - 1];
      if (last <= 127) return 0;
      if ((last & 192) !== 128) return 1;
      const limit = Math.max(0, len - 4);
      let i = len - 2;
      while (i >= limit && (data[i] & 192) === 128) i--;
      if (i < 0) return 1;
      const first = data[i];
      let needed;
      if (first >= 194 && first <= 223) needed = 2;
      else if (first >= 224 && first <= 239) needed = 3;
      else if (first >= 240 && first <= 244) needed = 4;
      else return 1;
      if (len - i !== needed) return 1;
      if (needed >= 3) {
        const second = data[i + 1];
        if (first === 224 && second < 160) return 1;
        if (first === 237 && second > 159) return 1;
        if (first === 240 && second < 144) return 1;
        if (first === 244 && second > 143) return 1;
      }
      return 0;
    }
  }
});

// node_modules/text-decoder/index.js
var require_text_decoder = __commonJS({
  "node_modules/text-decoder/index.js"(exports2, module2) {
    var PassThroughDecoder = require_pass_through_decoder();
    var UTF8Decoder = require_utf8_decoder();
    module2.exports = class TextDecoder {
      constructor(encoding = "utf8") {
        this.encoding = normalizeEncoding(encoding);
        switch (this.encoding) {
          case "utf8":
            this.decoder = new UTF8Decoder();
            break;
          case "utf16le":
          case "base64":
            throw new Error("Unsupported encoding: " + this.encoding);
          default:
            this.decoder = new PassThroughDecoder(this.encoding);
        }
      }
      get remaining() {
        return this.decoder.remaining;
      }
      push(data) {
        if (typeof data === "string") return data;
        return this.decoder.decode(data);
      }
      // For Node.js compatibility
      write(data) {
        return this.push(data);
      }
      end(data) {
        let result = "";
        if (data) result = this.push(data);
        result += this.decoder.flush();
        return result;
      }
    };
    function normalizeEncoding(encoding) {
      encoding = encoding.toLowerCase();
      switch (encoding) {
        case "utf8":
        case "utf-8":
          return "utf8";
        case "ucs2":
        case "ucs-2":
        case "utf16le":
        case "utf-16le":
          return "utf16le";
        case "latin1":
        case "binary":
          return "latin1";
        case "base64":
        case "ascii":
        case "hex":
          return encoding;
        default:
          throw new Error("Unknown encoding: " + encoding);
      }
    }
  }
});

// node_modules/streamx/lib/errors.js
var require_errors = __commonJS({
  "node_modules/streamx/lib/errors.js"(exports2, module2) {
    module2.exports = class StreamError extends Error {
      constructor(msg, code, fn = StreamError) {
        super(msg);
        this.code = code;
        if (Error.captureStackTrace) {
          Error.captureStackTrace(this, fn);
        }
      }
      static isStreamDestroyed(err) {
        return err && err.code === "STREAM_DESTROYED";
      }
      static isPrematureClose(err) {
        return err && err.code === "PREMATURE_CLOSE";
      }
      static isAborted(err) {
        return err && err.code === "ABORTED";
      }
      static isBadArgument(err) {
        return err && err.code === "BAD_ARGUMENT";
      }
      get name() {
        return "StreamError";
      }
      static STREAM_DESTROYED() {
        return new StreamError("Stream was destroyed", "STREAM_DESTROYED", StreamError.STREAM_DESTROYED);
      }
      static PREMATURE_CLOSE(msg = "Premature close") {
        return new StreamError(msg, "PREMATURE_CLOSE", StreamError.PREMATURE_CLOSE);
      }
      static ABORTED() {
        return new StreamError("Stream aborted", "ABORTED", StreamError.ABORTED);
      }
      static BAD_ARGUMENT(msg = "Bad argument") {
        return new StreamError(msg, "BAD_ARGUMENT", StreamError.BAD_ARGUMENT);
      }
    };
  }
});

// node_modules/streamx/index.js
var require_streamx = __commonJS({
  "node_modules/streamx/index.js"(exports2, module2) {
    var { EventEmitter } = require("events");
    var FIFO = require_fast_fifo();
    var TextDecoder2 = require_text_decoder();
    var StreamError = require_errors();
    var qmt = typeof queueMicrotask === "undefined" ? (fn) => global.process.nextTick(fn) : queueMicrotask;
    var MAX = (1 << 29) - 1;
    var OPENING = 1;
    var PREDESTROYING = 2;
    var DESTROYING = 4;
    var DESTROYED = 8;
    var NOT_OPENING = MAX ^ OPENING;
    var NOT_PREDESTROYING = MAX ^ PREDESTROYING;
    var READ_ACTIVE = 1 << 4;
    var READ_UPDATING = 2 << 4;
    var READ_PRIMARY = 4 << 4;
    var READ_QUEUED = 8 << 4;
    var READ_RESUMED = 16 << 4;
    var READ_PIPE_DRAINED = 32 << 4;
    var READ_ENDING = 64 << 4;
    var READ_EMIT_DATA = 128 << 4;
    var READ_EMIT_READABLE = 256 << 4;
    var READ_EMITTED_READABLE = 512 << 4;
    var READ_DONE = 1024 << 4;
    var READ_NEXT_TICK = 2048 << 4;
    var READ_NEEDS_PUSH = 4096 << 4;
    var READ_READ_AHEAD = 8192 << 4;
    var READ_FLOWING = READ_RESUMED | READ_PIPE_DRAINED;
    var READ_ACTIVE_AND_NEEDS_PUSH = READ_ACTIVE | READ_NEEDS_PUSH;
    var READ_PRIMARY_AND_ACTIVE = READ_PRIMARY | READ_ACTIVE;
    var READ_EMIT_READABLE_AND_QUEUED = READ_EMIT_READABLE | READ_QUEUED;
    var READ_RESUMED_READ_AHEAD = READ_RESUMED | READ_READ_AHEAD;
    var READ_NOT_ACTIVE = MAX ^ READ_ACTIVE;
    var READ_NON_PRIMARY = MAX ^ READ_PRIMARY;
    var READ_NON_PRIMARY_AND_PUSHED = MAX ^ (READ_PRIMARY | READ_NEEDS_PUSH);
    var READ_PUSHED = MAX ^ READ_NEEDS_PUSH;
    var READ_PAUSED = MAX ^ READ_RESUMED;
    var READ_NOT_QUEUED = MAX ^ (READ_QUEUED | READ_EMITTED_READABLE);
    var READ_NOT_ENDING = MAX ^ READ_ENDING;
    var READ_PIPE_NOT_DRAINED = MAX ^ READ_FLOWING;
    var READ_NOT_NEXT_TICK = MAX ^ READ_NEXT_TICK;
    var READ_NOT_UPDATING = MAX ^ READ_UPDATING;
    var READ_NO_READ_AHEAD = MAX ^ READ_READ_AHEAD;
    var READ_PAUSED_NO_READ_AHEAD = MAX ^ READ_RESUMED_READ_AHEAD;
    var WRITE_ACTIVE = 1 << 18;
    var WRITE_UPDATING = 2 << 18;
    var WRITE_PRIMARY = 4 << 18;
    var WRITE_QUEUED = 8 << 18;
    var WRITE_UNDRAINED = 16 << 18;
    var WRITE_DONE = 32 << 18;
    var WRITE_EMIT_DRAIN = 64 << 18;
    var WRITE_NEXT_TICK = 128 << 18;
    var WRITE_WRITING = 256 << 18;
    var WRITE_FINISHING = 512 << 18;
    var WRITE_CORKED = 1024 << 18;
    var WRITE_NOT_ACTIVE = MAX ^ (WRITE_ACTIVE | WRITE_WRITING);
    var WRITE_NON_PRIMARY = MAX ^ WRITE_PRIMARY;
    var WRITE_NOT_FINISHING = MAX ^ (WRITE_ACTIVE | WRITE_FINISHING);
    var WRITE_DRAINED = MAX ^ WRITE_UNDRAINED;
    var WRITE_NOT_QUEUED = MAX ^ WRITE_QUEUED;
    var WRITE_NOT_NEXT_TICK = MAX ^ WRITE_NEXT_TICK;
    var WRITE_NOT_UPDATING = MAX ^ WRITE_UPDATING;
    var WRITE_NOT_CORKED = MAX ^ WRITE_CORKED;
    var ACTIVE = READ_ACTIVE | WRITE_ACTIVE;
    var NOT_ACTIVE = MAX ^ ACTIVE;
    var DONE = READ_DONE | WRITE_DONE;
    var DESTROY_STATUS = DESTROYING | DESTROYED | PREDESTROYING;
    var OPEN_STATUS = DESTROY_STATUS | OPENING;
    var AUTO_DESTROY = DESTROY_STATUS | DONE;
    var NON_PRIMARY = WRITE_NON_PRIMARY & READ_NON_PRIMARY;
    var ACTIVE_OR_TICKING = WRITE_NEXT_TICK | READ_NEXT_TICK;
    var TICKING = ACTIVE_OR_TICKING & NOT_ACTIVE;
    var IS_OPENING = OPEN_STATUS | TICKING;
    var READ_PRIMARY_STATUS = OPEN_STATUS | READ_ENDING | READ_DONE;
    var READ_STATUS = OPEN_STATUS | READ_DONE | READ_QUEUED;
    var READ_ENDING_STATUS = OPEN_STATUS | READ_ENDING | READ_QUEUED;
    var READ_READABLE_STATUS = OPEN_STATUS | READ_EMIT_READABLE | READ_QUEUED | READ_EMITTED_READABLE;
    var SHOULD_NOT_READ = OPEN_STATUS | READ_ACTIVE | READ_ENDING | READ_DONE | READ_NEEDS_PUSH | READ_READ_AHEAD;
    var READ_BACKPRESSURE_STATUS = DESTROY_STATUS | READ_ENDING | READ_DONE;
    var READ_UPDATE_SYNC_STATUS = READ_UPDATING | OPEN_STATUS | READ_NEXT_TICK | READ_PRIMARY;
    var READ_NEXT_TICK_OR_OPENING = READ_NEXT_TICK | OPENING;
    var WRITE_PRIMARY_STATUS = OPEN_STATUS | WRITE_FINISHING | WRITE_DONE;
    var WRITE_QUEUED_AND_UNDRAINED = WRITE_QUEUED | WRITE_UNDRAINED;
    var WRITE_QUEUED_AND_ACTIVE = WRITE_QUEUED | WRITE_ACTIVE;
    var WRITE_DRAIN_STATUS = WRITE_QUEUED | WRITE_UNDRAINED | OPEN_STATUS | WRITE_ACTIVE;
    var WRITE_STATUS = OPEN_STATUS | WRITE_ACTIVE | WRITE_QUEUED | WRITE_CORKED;
    var WRITE_PRIMARY_AND_ACTIVE = WRITE_PRIMARY | WRITE_ACTIVE;
    var WRITE_ACTIVE_AND_WRITING = WRITE_ACTIVE | WRITE_WRITING;
    var WRITE_FINISHING_STATUS = OPEN_STATUS | WRITE_FINISHING | WRITE_QUEUED_AND_ACTIVE | WRITE_DONE;
    var WRITE_BACKPRESSURE_STATUS = WRITE_UNDRAINED | DESTROY_STATUS | WRITE_FINISHING | WRITE_DONE;
    var WRITE_UPDATE_SYNC_STATUS = WRITE_UPDATING | OPEN_STATUS | WRITE_NEXT_TICK | WRITE_PRIMARY;
    var WRITE_DROP_DATA = WRITE_FINISHING | WRITE_DONE | DESTROY_STATUS;
    var asyncIterator = Symbol.asyncIterator || Symbol("asyncIterator");
    var WritableState = class {
      constructor(stream2, { highWaterMark = 16384, map = null, mapWritable, byteLength, byteLengthWritable: byteLengthWritable2 } = {}) {
        this.stream = stream2;
        this.queue = new FIFO();
        this.highWaterMark = highWaterMark;
        this.buffered = 0;
        this.error = null;
        this.pipeline = null;
        this.drains = null;
        this.byteLength = byteLengthWritable2 || byteLength || defaultByteLength;
        this.map = mapWritable || map;
        this.afterWrite = afterWrite.bind(this);
        this.afterUpdateNextTick = updateWriteNT.bind(this);
      }
      get ending() {
        return (this.stream._duplexState & WRITE_FINISHING) !== 0;
      }
      get ended() {
        return (this.stream._duplexState & WRITE_DONE) !== 0;
      }
      push(data) {
        if ((this.stream._duplexState & WRITE_DROP_DATA) !== 0) return false;
        if (this.map !== null) data = this.map(data);
        this.buffered += this.byteLength(data);
        this.queue.push(data);
        if (this.buffered < this.highWaterMark) {
          this.stream._duplexState |= WRITE_QUEUED;
          return true;
        }
        this.stream._duplexState |= WRITE_QUEUED_AND_UNDRAINED;
        return false;
      }
      shift() {
        const data = this.queue.shift();
        this.buffered -= this.byteLength(data);
        if (this.buffered === 0) this.stream._duplexState &= WRITE_NOT_QUEUED;
        return data;
      }
      end(data) {
        if (typeof data === "function") {
          this.stream.once("finish", data);
        } else if (data !== void 0 && data !== null) {
          this.push(data);
        }
        this.stream._duplexState = (this.stream._duplexState | WRITE_FINISHING) & WRITE_NON_PRIMARY;
      }
      autoBatch(data, cb) {
        const buffer = [];
        const stream2 = this.stream;
        buffer.push(data);
        while ((stream2._duplexState & WRITE_STATUS) === WRITE_QUEUED_AND_ACTIVE) {
          buffer.push(stream2._writableState.shift());
        }
        if ((stream2._duplexState & OPEN_STATUS) !== 0) return cb(null);
        stream2._writev(buffer, cb);
      }
      update() {
        const stream2 = this.stream;
        stream2._duplexState |= WRITE_UPDATING;
        do {
          while ((stream2._duplexState & WRITE_STATUS) === WRITE_QUEUED) {
            const data = this.shift();
            stream2._duplexState |= WRITE_ACTIVE_AND_WRITING;
            stream2._write(data, this.afterWrite);
          }
          if ((stream2._duplexState & WRITE_PRIMARY_AND_ACTIVE) === 0) this.updateNonPrimary();
        } while (this.continueUpdate() === true);
        stream2._duplexState &= WRITE_NOT_UPDATING;
      }
      updateNonPrimary() {
        const stream2 = this.stream;
        if ((stream2._duplexState & WRITE_FINISHING_STATUS) === WRITE_FINISHING) {
          stream2._duplexState = stream2._duplexState | WRITE_ACTIVE;
          stream2._final(afterFinal.bind(this));
          return;
        }
        if ((stream2._duplexState & DESTROY_STATUS) === DESTROYING) {
          if ((stream2._duplexState & ACTIVE_OR_TICKING) === 0) {
            stream2._duplexState |= ACTIVE;
            stream2._destroy(afterDestroy.bind(this));
          }
          return;
        }
        if ((stream2._duplexState & IS_OPENING) === OPENING) {
          stream2._duplexState = (stream2._duplexState | ACTIVE) & NOT_OPENING;
          stream2._open(afterOpen.bind(this));
        }
      }
      continueUpdate() {
        if ((this.stream._duplexState & WRITE_NEXT_TICK) === 0) return false;
        this.stream._duplexState &= WRITE_NOT_NEXT_TICK;
        return true;
      }
      updateCallback() {
        if ((this.stream._duplexState & WRITE_UPDATE_SYNC_STATUS) === WRITE_PRIMARY) {
          this.update();
        } else {
          this.updateNextTick();
        }
      }
      updateNextTick() {
        if ((this.stream._duplexState & WRITE_NEXT_TICK) !== 0) return;
        this.stream._duplexState |= WRITE_NEXT_TICK;
        if ((this.stream._duplexState & WRITE_UPDATING) === 0) qmt(this.afterUpdateNextTick);
      }
    };
    var ReadableState = class {
      constructor(stream2, { highWaterMark = 16384, map = null, mapReadable, byteLength, byteLengthReadable } = {}) {
        this.stream = stream2;
        this.queue = new FIFO();
        this.highWaterMark = highWaterMark === 0 ? 1 : highWaterMark;
        this.buffered = 0;
        this.readAhead = highWaterMark > 0;
        this.error = null;
        this.pipeline = null;
        this.byteLength = byteLengthReadable || byteLength || defaultByteLength;
        this.map = mapReadable || map;
        this.pipeTo = null;
        this.afterRead = afterRead.bind(this);
        this.afterUpdateNextTick = updateReadNT.bind(this);
      }
      get ending() {
        return (this.stream._duplexState & READ_ENDING) !== 0;
      }
      get ended() {
        return (this.stream._duplexState & READ_DONE) !== 0;
      }
      pipe(pipeTo, cb) {
        if (this.pipeTo !== null) throw StreamError.BAD_ARGUMENT("Can only pipe to one destination");
        if (typeof cb !== "function") cb = null;
        this.stream._duplexState |= READ_PIPE_DRAINED;
        this.pipeTo = pipeTo;
        this.pipeline = new Pipeline(this.stream, pipeTo, cb);
        if (cb) this.stream.on("error", noop2);
        if (isStreamx(pipeTo)) {
          pipeTo._writableState.pipeline = this.pipeline;
          if (cb) pipeTo.on("error", noop2);
          pipeTo.on("finish", this.pipeline.finished.bind(this.pipeline));
        } else {
          const onerror = this.pipeline.done.bind(this.pipeline, pipeTo);
          const onclose = this.pipeline.done.bind(this.pipeline, pipeTo, null);
          pipeTo.on("error", onerror);
          pipeTo.on("close", onclose);
          pipeTo.on("finish", this.pipeline.finished.bind(this.pipeline));
        }
        pipeTo.on("drain", afterDrain.bind(this));
        this.stream.emit("piping", pipeTo);
        pipeTo.emit("pipe", this.stream);
      }
      push(data) {
        const stream2 = this.stream;
        if (data === null) {
          this.highWaterMark = 0;
          stream2._duplexState = (stream2._duplexState | READ_ENDING) & READ_NON_PRIMARY_AND_PUSHED;
          return false;
        }
        if (this.map !== null) {
          data = this.map(data);
          if (data === null) {
            stream2._duplexState &= READ_PUSHED;
            return this.buffered < this.highWaterMark;
          }
        }
        this.buffered += this.byteLength(data);
        this.queue.push(data);
        stream2._duplexState = (stream2._duplexState | READ_QUEUED) & READ_PUSHED;
        return this.buffered < this.highWaterMark;
      }
      shift() {
        const data = this.queue.shift();
        this.buffered -= this.byteLength(data);
        if (this.buffered === 0) {
          this.stream._duplexState &= READ_NOT_QUEUED;
        }
        return data;
      }
      unshift(data) {
        const pending = [this.map !== null ? this.map(data) : data];
        while (this.buffered > 0) pending.push(this.shift());
        for (let i = 0; i < pending.length - 1; i++) {
          const data2 = pending[i];
          this.buffered += this.byteLength(data2);
          this.queue.push(data2);
        }
        this.push(pending[pending.length - 1]);
      }
      read() {
        const stream2 = this.stream;
        if ((stream2._duplexState & READ_STATUS) === READ_QUEUED) {
          const data = this.shift();
          if (this.pipeTo !== null && this.pipeTo.write(data) === false) {
            stream2._duplexState &= READ_PIPE_NOT_DRAINED;
          }
          if ((stream2._duplexState & READ_EMIT_DATA) !== 0) {
            stream2.emit("data", data);
          }
          return data;
        }
        if (this.readAhead === false) {
          stream2._duplexState |= READ_READ_AHEAD;
          this.updateNextTick();
        }
        return null;
      }
      drain() {
        const stream2 = this.stream;
        while ((stream2._duplexState & READ_STATUS) === READ_QUEUED && (stream2._duplexState & READ_FLOWING) !== 0) {
          const data = this.shift();
          if (this.pipeTo !== null && this.pipeTo.write(data) === false) {
            stream2._duplexState &= READ_PIPE_NOT_DRAINED;
          }
          if ((stream2._duplexState & READ_EMIT_DATA) !== 0) {
            stream2.emit("data", data);
          }
        }
      }
      update() {
        const stream2 = this.stream;
        stream2._duplexState |= READ_UPDATING;
        do {
          this.drain();
          while (this.buffered < this.highWaterMark && (stream2._duplexState & SHOULD_NOT_READ) === READ_READ_AHEAD) {
            stream2._duplexState |= READ_ACTIVE_AND_NEEDS_PUSH;
            stream2._read(this.afterRead);
            this.drain();
          }
          if ((stream2._duplexState & READ_READABLE_STATUS) === READ_EMIT_READABLE_AND_QUEUED) {
            stream2._duplexState |= READ_EMITTED_READABLE;
            stream2.emit("readable");
          }
          if ((stream2._duplexState & READ_PRIMARY_AND_ACTIVE) === 0) {
            this.updateNonPrimary();
          }
        } while (this.continueUpdate() === true);
        stream2._duplexState &= READ_NOT_UPDATING;
      }
      updateNonPrimary() {
        const stream2 = this.stream;
        if ((stream2._duplexState & READ_ENDING_STATUS) === READ_ENDING) {
          stream2._duplexState = (stream2._duplexState | READ_DONE) & READ_NOT_ENDING;
          stream2.emit("end");
          if ((stream2._duplexState & AUTO_DESTROY) === DONE) {
            stream2._duplexState |= DESTROYING;
          }
          if (this.pipeTo !== null) {
            this.pipeTo.end();
          }
        }
        if ((stream2._duplexState & DESTROY_STATUS) === DESTROYING) {
          if ((stream2._duplexState & ACTIVE_OR_TICKING) === 0) {
            stream2._duplexState |= ACTIVE;
            stream2._destroy(afterDestroy.bind(this));
          }
          return;
        }
        if ((stream2._duplexState & IS_OPENING) === OPENING) {
          stream2._duplexState = (stream2._duplexState | ACTIVE) & NOT_OPENING;
          stream2._open(afterOpen.bind(this));
        }
      }
      continueUpdate() {
        if ((this.stream._duplexState & READ_NEXT_TICK) === 0) return false;
        this.stream._duplexState &= READ_NOT_NEXT_TICK;
        return true;
      }
      updateCallback() {
        if ((this.stream._duplexState & READ_UPDATE_SYNC_STATUS) === READ_PRIMARY) {
          this.update();
        } else {
          this.updateNextTick();
        }
      }
      updateNextTickIfOpen() {
        if ((this.stream._duplexState & READ_NEXT_TICK_OR_OPENING) !== 0) return;
        this.stream._duplexState |= READ_NEXT_TICK;
        if ((this.stream._duplexState & READ_UPDATING) === 0) qmt(this.afterUpdateNextTick);
      }
      updateNextTick() {
        if ((this.stream._duplexState & READ_NEXT_TICK) !== 0) return;
        this.stream._duplexState |= READ_NEXT_TICK;
        if ((this.stream._duplexState & READ_UPDATING) === 0) qmt(this.afterUpdateNextTick);
      }
    };
    var TransformState = class {
      constructor(stream2) {
        this.data = null;
        this.afterTransform = afterTransform.bind(stream2);
        this.afterFinal = null;
      }
    };
    var Pipeline = class {
      constructor(src, dst, cb) {
        this.from = src;
        this.to = dst;
        this.afterPipe = cb;
        this.error = null;
        this.pipeToFinished = false;
      }
      finished() {
        this.pipeToFinished = true;
      }
      done(stream2, err) {
        if (err) this.error = err;
        if (stream2 === this.to) {
          this.to = null;
          if (this.from !== null) {
            if ((this.from._duplexState & READ_DONE) === 0 || !this.pipeToFinished) {
              this.from.destroy(this.error || StreamError.PREMATURE_CLOSE("Writable stream closed"));
            }
            return;
          }
        }
        if (stream2 === this.from) {
          this.from = null;
          if (this.to !== null) {
            if ((stream2._duplexState & READ_DONE) === 0) {
              this.to.destroy(this.error || StreamError.PREMATURE_CLOSE("Readable stream closed"));
            }
            return;
          }
        }
        if (this.afterPipe !== null) this.afterPipe(this.error);
        this.to = this.from = this.afterPipe = null;
      }
    };
    function afterDrain() {
      this.stream._duplexState |= READ_PIPE_DRAINED;
      this.updateCallback();
    }
    function afterFinal(err) {
      const stream2 = this.stream;
      if (err) stream2.destroy(err);
      if ((stream2._duplexState & DESTROY_STATUS) === 0) {
        stream2._duplexState |= WRITE_DONE;
        stream2.emit("finish");
      }
      if ((stream2._duplexState & AUTO_DESTROY) === DONE) {
        stream2._duplexState |= DESTROYING;
      }
      stream2._duplexState &= WRITE_NOT_FINISHING;
      if ((stream2._duplexState & WRITE_UPDATING) === 0) {
        this.update();
      } else {
        this.updateNextTick();
      }
    }
    function afterDestroy(err) {
      const stream2 = this.stream;
      if (!err && !StreamError.isStreamDestroyed(this.error)) err = this.error;
      if (err) stream2.emit("error", err);
      stream2._duplexState |= DESTROYED;
      stream2.emit("close");
      const rs = stream2._readableState;
      const ws = stream2._writableState;
      if (rs !== null && rs.pipeline !== null) {
        rs.pipeline.done(stream2, err);
      }
      if (ws !== null) {
        while (ws.drains !== null && ws.drains.length > 0) {
          ws.drains.shift().resolve(false);
        }
        if (ws.pipeline !== null) {
          ws.pipeline.done(stream2, err);
        }
      }
    }
    function afterWrite(err) {
      const stream2 = this.stream;
      if (err) stream2.destroy(err);
      stream2._duplexState &= WRITE_NOT_ACTIVE;
      if (this.drains !== null) tickDrains(this.drains);
      if ((stream2._duplexState & WRITE_DRAIN_STATUS) === WRITE_UNDRAINED) {
        stream2._duplexState &= WRITE_DRAINED;
        if ((stream2._duplexState & WRITE_EMIT_DRAIN) === WRITE_EMIT_DRAIN) {
          stream2.emit("drain");
        }
      }
      this.updateCallback();
    }
    function afterRead(err) {
      if (err) this.stream.destroy(err);
      this.stream._duplexState &= READ_NOT_ACTIVE;
      if (this.readAhead === false && (this.stream._duplexState & READ_RESUMED) === 0) {
        this.stream._duplexState &= READ_NO_READ_AHEAD;
      }
      this.updateCallback();
    }
    function updateReadNT() {
      if ((this.stream._duplexState & READ_UPDATING) === 0) {
        this.stream._duplexState &= READ_NOT_NEXT_TICK;
        this.update();
      }
    }
    function updateWriteNT() {
      if ((this.stream._duplexState & WRITE_UPDATING) === 0) {
        this.stream._duplexState &= WRITE_NOT_NEXT_TICK;
        this.update();
      }
    }
    function tickDrains(drains) {
      for (let i = 0; i < drains.length; i++) {
        if (--drains[i].writes === 0) {
          drains.shift().resolve(true);
          i--;
        }
      }
    }
    function afterOpen(err) {
      const stream2 = this.stream;
      if (err) stream2.destroy(err);
      if ((stream2._duplexState & DESTROYING) === 0) {
        if ((stream2._duplexState & READ_PRIMARY_STATUS) === 0) {
          stream2._duplexState |= READ_PRIMARY;
        }
        if ((stream2._duplexState & WRITE_PRIMARY_STATUS) === 0) {
          stream2._duplexState |= WRITE_PRIMARY;
        }
        stream2.emit("open");
      }
      stream2._duplexState &= NOT_ACTIVE;
      if (stream2._writableState !== null) {
        stream2._writableState.updateCallback();
      }
      if (stream2._readableState !== null) {
        stream2._readableState.updateCallback();
      }
    }
    function afterTransform(err, data) {
      if (data !== void 0 && data !== null) this.push(data);
      this._writableState.afterWrite(err);
    }
    function newListener(name) {
      if (this._readableState !== null) {
        if (name === "data") {
          this._duplexState |= READ_EMIT_DATA | READ_RESUMED_READ_AHEAD;
          this._readableState.updateNextTick();
        }
        if (name === "readable") {
          this._duplexState |= READ_EMIT_READABLE;
          this._readableState.updateNextTick();
        }
      }
      if (this._writableState !== null) {
        if (name === "drain") {
          this._duplexState |= WRITE_EMIT_DRAIN;
          this._writableState.updateNextTick();
        }
      }
    }
    var Stream = class extends EventEmitter {
      constructor(opts) {
        super();
        this._duplexState = 0;
        this._readableState = null;
        this._writableState = null;
        if (opts) {
          if (opts.open) this._open = opts.open;
          if (opts.destroy) this._destroy = opts.destroy;
          if (opts.predestroy) this._predestroy = opts.predestroy;
          if (opts.signal) opts.signal.addEventListener("abort", abort.bind(this));
        }
        this.on("newListener", newListener);
      }
      _open(cb) {
        cb(null);
      }
      _destroy(cb) {
        cb(null);
      }
      _predestroy() {
      }
      get readable() {
        return this._readableState !== null ? true : void 0;
      }
      get writable() {
        return this._writableState !== null ? true : void 0;
      }
      get destroyed() {
        return (this._duplexState & DESTROYED) !== 0;
      }
      get destroying() {
        return (this._duplexState & DESTROY_STATUS) !== 0;
      }
      destroy(err) {
        if ((this._duplexState & DESTROY_STATUS) === 0) {
          if (!err) err = StreamError.STREAM_DESTROYED();
          this._duplexState = (this._duplexState | DESTROYING) & NON_PRIMARY;
          if (this._readableState !== null) {
            this._readableState.highWaterMark = 0;
            this._readableState.error = err;
          }
          if (this._writableState !== null) {
            this._writableState.highWaterMark = 0;
            this._writableState.error = err;
          }
          this._duplexState |= PREDESTROYING;
          this._predestroy();
          this._duplexState &= NOT_PREDESTROYING;
          if (this._readableState !== null) {
            this._readableState.updateNextTick();
          }
          if (this._writableState !== null) {
            this._writableState.updateNextTick();
          }
        }
      }
    };
    var Readable2 = class _Readable extends Stream {
      constructor(opts) {
        super(opts);
        this._duplexState |= OPENING | WRITE_DONE | READ_READ_AHEAD;
        this._readableState = new ReadableState(this, opts);
        if (opts) {
          if (this._readableState.readAhead === false) this._duplexState &= READ_NO_READ_AHEAD;
          if (opts.read) this._read = opts.read;
          if (opts.eagerOpen) this._readableState.updateNextTick();
          if (opts.encoding) this.setEncoding(opts.encoding);
        }
      }
      static deferred(fn, opts) {
        const out = new PassThrough2(opts);
        fn().then((src) => {
          if (src === null) return out.end();
          if (out.destroying) return;
          pipeline(src, out, noop2);
        }).catch((err) => out.destroy(err));
        return out;
      }
      setEncoding(encoding) {
        const dec = new TextDecoder2(encoding);
        const map = this._readableState.map || echo;
        this._readableState.map = mapOrSkip;
        return this;
        function mapOrSkip(data) {
          const next = dec.push(data);
          return next === "" && (data.byteLength !== 0 || dec.remaining > 0) ? null : map(next);
        }
      }
      _read(cb) {
        cb(null);
      }
      pipe(dest, cb) {
        this._readableState.updateNextTick();
        this._readableState.pipe(dest, cb);
        return dest;
      }
      read() {
        this._readableState.updateNextTick();
        return this._readableState.read();
      }
      push(data) {
        this._readableState.updateNextTickIfOpen();
        return this._readableState.push(data);
      }
      unshift(data) {
        this._readableState.updateNextTickIfOpen();
        return this._readableState.unshift(data);
      }
      resume() {
        this._duplexState |= READ_RESUMED_READ_AHEAD;
        this._readableState.updateNextTick();
        return this;
      }
      pause() {
        this._duplexState &= this._readableState.readAhead === false ? READ_PAUSED_NO_READ_AHEAD : READ_PAUSED;
        return this;
      }
      static _fromAsyncIterator(ite, opts) {
        let destroy2;
        const rs = new _Readable(__spreadProps(__spreadValues({}, opts), {
          read(cb) {
            ite.next().then(push).then(cb.bind(null, null)).catch(cb);
          },
          predestroy() {
            destroy2 = ite.return();
          },
          destroy(cb) {
            if (!destroy2) return cb(null);
            destroy2.then(cb.bind(null, null)).catch(cb);
          }
        }));
        return rs;
        function push(data) {
          if (data.done) rs.push(null);
          else rs.push(data.value);
        }
      }
      static from(data, opts) {
        if (isReadStreamx(data)) return data;
        if (data[asyncIterator]) return this._fromAsyncIterator(data[asyncIterator](), opts);
        if (!Array.isArray(data)) data = data === void 0 ? [] : [data];
        let i = 0;
        return new _Readable(__spreadProps(__spreadValues({}, opts), {
          read(cb) {
            this.push(i === data.length ? null : data[i++]);
            cb(null);
          }
        }));
      }
      static isBackpressured(rs) {
        return (rs._duplexState & READ_BACKPRESSURE_STATUS) !== 0 || rs._readableState.buffered >= rs._readableState.highWaterMark;
      }
      static isPaused(rs) {
        return (rs._duplexState & READ_RESUMED) === 0;
      }
      [asyncIterator]() {
        const stream2 = this;
        let error = null;
        let promiseResolve = null;
        let promiseReject = null;
        this.on("error", (err) => {
          error = err;
        });
        this.on("readable", onreadable);
        this.on("close", onclose);
        return {
          [asyncIterator]() {
            return this;
          },
          next() {
            return new Promise(function(resolve, reject) {
              promiseResolve = resolve;
              promiseReject = reject;
              const data = stream2.read();
              if (data !== null) ondata(data);
              else if ((stream2._duplexState & DESTROYED) !== 0) ondata(null);
            });
          },
          return() {
            return destroy2(null);
          },
          throw(err) {
            return destroy2(err);
          }
        };
        function onreadable() {
          if (promiseResolve !== null) ondata(stream2.read());
        }
        function onclose() {
          if (promiseResolve !== null) ondata(null);
        }
        function ondata(data) {
          if (promiseReject === null) return;
          if (error) {
            promiseReject(error);
          } else if (data === null && (stream2._duplexState & READ_DONE) === 0) {
            promiseReject(StreamError.STREAM_DESTROYED());
          } else {
            promiseResolve({ value: data, done: data === null });
          }
          promiseReject = promiseResolve = null;
        }
        function destroy2(err) {
          stream2.destroy(err);
          return new Promise((resolve, reject) => {
            if (stream2._duplexState & DESTROYED) return resolve({ value: void 0, done: true });
            stream2.once("close", function() {
              if (err) reject(err);
              else resolve({ value: void 0, done: true });
            });
          });
        }
      }
    };
    var Writable2 = class extends Stream {
      constructor(opts) {
        super(opts);
        this._duplexState |= OPENING | READ_DONE;
        this._writableState = new WritableState(this, opts);
        if (opts) {
          if (opts.writev) this._writev = opts.writev;
          if (opts.write) this._write = opts.write;
          if (opts.final) this._final = opts.final;
          if (opts.eagerOpen) this._writableState.updateNextTick();
        }
      }
      cork() {
        this._duplexState |= WRITE_CORKED;
      }
      uncork() {
        this._duplexState &= WRITE_NOT_CORKED;
        this._writableState.updateNextTick();
      }
      _writev(batch, cb) {
        cb(null);
      }
      _write(data, cb) {
        this._writableState.autoBatch(data, cb);
      }
      _final(cb) {
        cb(null);
      }
      static isBackpressured(ws) {
        return (ws._duplexState & WRITE_BACKPRESSURE_STATUS) !== 0;
      }
      static drained(ws) {
        if (ws.destroyed) return Promise.resolve(false);
        const state = ws._writableState;
        const pending = isWritev(ws) ? Math.min(1, state.queue.length) : state.queue.length;
        const writes = pending + (ws._duplexState & WRITE_WRITING ? 1 : 0);
        if (writes === 0) return Promise.resolve(true);
        if (state.drains === null) state.drains = [];
        return new Promise((resolve) => {
          state.drains.push({ writes, resolve });
        });
      }
      write(data) {
        this._writableState.updateNextTick();
        return this._writableState.push(data);
      }
      end(data) {
        this._writableState.updateNextTick();
        this._writableState.end(data);
        return this;
      }
    };
    var Duplex2 = class extends Readable2 {
      // and Writable
      constructor(opts) {
        super(opts);
        this._duplexState = OPENING | this._duplexState & READ_READ_AHEAD;
        this._writableState = new WritableState(this, opts);
        if (opts) {
          if (opts.writev) this._writev = opts.writev;
          if (opts.write) this._write = opts.write;
          if (opts.final) this._final = opts.final;
        }
      }
      cork() {
        this._duplexState |= WRITE_CORKED;
      }
      uncork() {
        this._duplexState &= WRITE_NOT_CORKED;
        this._writableState.updateNextTick();
      }
      _writev(batch, cb) {
        cb(null);
      }
      _write(data, cb) {
        this._writableState.autoBatch(data, cb);
      }
      _final(cb) {
        cb(null);
      }
      write(data) {
        this._writableState.updateNextTick();
        return this._writableState.push(data);
      }
      end(data) {
        this._writableState.updateNextTick();
        this._writableState.end(data);
        return this;
      }
    };
    var Transform2 = class extends Duplex2 {
      constructor(opts) {
        super(opts);
        this._transformState = new TransformState(this);
        if (opts) {
          if (opts.transform) this._transform = opts.transform;
          if (opts.flush) this._flush = opts.flush;
        }
      }
      _write(data, cb) {
        if (this._readableState.buffered >= this._readableState.highWaterMark) {
          this._transformState.data = data;
        } else {
          this._transform(data, this._transformState.afterTransform);
        }
      }
      _read(cb) {
        if (this._transformState.data !== null) {
          const data = this._transformState.data;
          this._transformState.data = null;
          cb(null);
          this._transform(data, this._transformState.afterTransform);
        } else {
          cb(null);
        }
      }
      destroy(err) {
        super.destroy(err);
        if (this._transformState.data !== null) {
          this._transformState.data = null;
          this._transformState.afterTransform();
        }
      }
      _transform(data, cb) {
        cb(null, data);
      }
      _flush(cb) {
        cb(null);
      }
      _final(cb) {
        this._transformState.afterFinal = cb;
        this._flush(transformAfterFlush.bind(this));
      }
    };
    var PassThrough2 = class extends Transform2 {
    };
    function transformAfterFlush(err, data) {
      const cb = this._transformState.afterFinal;
      if (err) return cb(err);
      if (data !== null && data !== void 0) this.push(data);
      this.push(null);
      cb(null);
    }
    function pipelinePromise(...streams) {
      return new Promise((resolve, reject) => {
        return pipeline(...streams, (err) => {
          if (err) return reject(err);
          resolve();
        });
      });
    }
    function pipeline(stream2, ...streams) {
      const all = Array.isArray(stream2) ? [...stream2, ...streams] : [stream2, ...streams];
      const done = all.length && typeof all[all.length - 1] === "function" ? all.pop() : null;
      if (all.length < 2) throw StreamError.BAD_ARGUMENT("Pipeline requires at least 2 streams");
      let src = all[0];
      let dest = null;
      let error = null;
      for (let i = 1; i < all.length; i++) {
        dest = all[i];
        if (isStreamx(src)) {
          src.pipe(dest, onerror);
        } else {
          errorHandle(src, true, i > 1, onerror);
          src.pipe(dest);
        }
        src = dest;
      }
      if (done) {
        let fin = false;
        const autoDestroy = isStreamx(dest) || !!(dest._writableState && dest._writableState.autoDestroy);
        dest.on("error", (err) => {
          if (error === null) error = err;
        });
        dest.on("finish", () => {
          fin = true;
          if (!autoDestroy) done(error);
        });
        if (autoDestroy) {
          dest.on("close", () => done(error || (fin ? null : StreamError.PREMATURE_CLOSE())));
        }
      }
      return dest;
      function errorHandle(s, rd, wr, onerror2) {
        s.on("error", onerror2);
        s.on("close", onclose);
        function onclose() {
          if (rd && s._readableState && !s._readableState.ended) {
            return onerror2(StreamError.PREMATURE_CLOSE());
          }
          if (wr && s._writableState && !s._writableState.ended) {
            return onerror2(StreamError.PREMATURE_CLOSE());
          }
        }
      }
      function onerror(err) {
        if (!err || error) return;
        error = err;
        for (const s of all) {
          s.destroy(err);
        }
      }
    }
    function echo(s) {
      return s;
    }
    function isStream(stream2) {
      return !!stream2._readableState || !!stream2._writableState;
    }
    function isStreamx(stream2) {
      return typeof stream2._duplexState === "number" && isStream(stream2);
    }
    function isEnding(stream2) {
      return !!stream2._readableState && stream2._readableState.ending;
    }
    function isEnded(stream2) {
      return !!stream2._readableState && stream2._readableState.ended;
    }
    function isFinishing(stream2) {
      return !!stream2._writableState && stream2._writableState.ending;
    }
    function isFinished(stream2) {
      return !!stream2._writableState && stream2._writableState.ended;
    }
    function getStreamError(stream2, opts = {}) {
      const err = stream2._readableState && stream2._readableState.error || stream2._writableState && stream2._writableState.error;
      return !opts.all && StreamError.isStreamDestroyed(err) ? null : err;
    }
    function isReadStreamx(stream2) {
      return isStreamx(stream2) && stream2.readable;
    }
    function isDisturbed(stream2) {
      return (stream2._duplexState & OPENING) !== OPENING || (stream2._duplexState & DESTROYING) === DESTROYING || (stream2._duplexState & ACTIVE_OR_TICKING) !== 0;
    }
    function isTypedArray(data) {
      return typeof data === "object" && data !== null && typeof data.byteLength === "number";
    }
    function defaultByteLength(data) {
      return isTypedArray(data) ? data.byteLength : 1024;
    }
    function noop2() {
    }
    function abort() {
      this.destroy(StreamError.ABORTED());
    }
    function isWritev(s) {
      return s._writev !== Writable2.prototype._writev && s._writev !== Duplex2.prototype._writev;
    }
    module2.exports = {
      pipeline,
      pipelinePromise,
      isStream,
      isStreamx,
      isEnding,
      isEnded,
      isFinishing,
      isFinished,
      isDisturbed,
      getStreamError,
      Stream,
      Writable: Writable2,
      Readable: Readable2,
      Duplex: Duplex2,
      Transform: Transform2,
      // Export PassThrough for compatibility with Node.js core's stream module
      PassThrough: PassThrough2
    };
  }
});

// streams/internal/streamx/bridge.js
var require_bridge = __commonJS({
  "streams/internal/streamx/bridge.js"(exports2, module2) {
    var stream2 = require_streamx();
    function canonical() {
      return globalThis.__goja_ext_streams_canonical;
    }
    function readableToWeb(readable, opts) {
      return canonical().ReadableStream.from(readable);
    }
    function writableToWeb(writable) {
      const streams = canonical();
      return new streams.WritableStream({
        write(chunk) {
          return new Promise(function(resolve, reject) {
            writable.write(chunk, function(err) {
              if (err) reject(err);
              else resolve();
            });
          });
        },
        close() {
          return new Promise(function(resolve, reject) {
            writable.end(function(err) {
              if (err) reject(err);
              else resolve();
            });
          });
        },
        abort(reason) {
          writable.destroy(reason);
          return Promise.resolve();
        }
      });
    }
    function readableFromWeb(webStream, opts) {
      const reader = webStream.getReader();
      const readable = new stream2.Readable({
        read(cb) {
          reader.read().then(function(r) {
            if (r.done) {
              readable.push(null);
              cb(null);
            } else {
              readable.push(r.value);
              cb(null);
            }
          }, function(err) {
            cb(err);
          });
        }
      });
      if (opts.encoding) readable.setEncoding(opts.encoding);
      if (opts.signal) addAbortSignal2(opts.signal, readable);
      return readable;
    }
    function writableFromWeb(webStream, opts) {
      const writer = webStream.getWriter();
      const writable = new stream2.Writable({
        write(data, cb) {
          writer.write(data).then(function() {
            cb(null);
          }, function(err) {
            cb(err);
          });
        },
        final(cb) {
          writer.close().then(function() {
            cb(null);
          }, function(err) {
            cb(err);
          });
        }
      });
      if (opts.signal) addAbortSignal2(opts.signal, writable);
      return writable;
    }
    function duplexFromWeb(readableStream, writableStream, opts) {
      const readable = readableFromWeb(readableStream, opts);
      const writable = writableFromWeb(writableStream, opts);
      const duplex = new stream2.Duplex({
        read(cb) {
          const data = readable.read();
          if (data === null) {
            readable.once("readable", function() {
              const next = readable.read();
              if (next === null && readable.destroyed) duplex.push(null);
              else if (next !== null) duplex.push(next);
              cb(null);
            });
            readable.once("end", function() {
              duplex.push(null);
            });
            cb(null);
          } else {
            duplex.push(data);
            cb(null);
          }
        },
        write(data, cb) {
          writable.write(data, cb);
        }
      });
      if (opts.signal) addAbortSignal2(opts.signal, duplex);
      return duplex;
    }
    function duplexToWeb(duplex) {
      return {
        readable: readableToWeb(duplex),
        writable: writableToWeb(duplex)
      };
    }
    function addAbortSignal2(signal, stream3) {
      function onAbort() {
        stream3.destroy(signal.reason);
      }
      if (signal.aborted) onAbort();
      else signal.addEventListener("abort", onAbort);
      return stream3;
    }
    module2.exports = {
      readableToWeb,
      writableToWeb,
      duplexToWeb,
      readableFromWeb,
      writableFromWeb,
      duplexFromWeb
    };
  }
});

// streams/internal/streamx/facade.js
var b4a = require_browser();
var stream = require_streamx();
var bridge = require_bridge();
var defaultEncoding = "utf8";
module.exports = exports = stream.Stream;
exports.pipeline = stream.pipeline;
exports.isStream = stream.isStream;
exports.isEnding = stream.isEnding;
exports.isEnded = stream.isEnded;
exports.isFinishing = stream.isFinishing;
exports.isFinished = stream.isFinished;
exports.isDisturbed = stream.isDisturbed;
exports.isErrored = function isErrored(stream2) {
  return exports.getStreamError(stream2) !== null;
};
exports.isReadable = function isReadable(stream2) {
  return stream2.readable && !stream2.destroying && !exports.isEnded(stream2);
};
exports.isWritable = function isWritable(stream2) {
  return stream2.writable && !stream2.destroying && !exports.isFinishing(stream2);
};
exports.getStreamError = stream.getStreamError;
exports.addAbortSignal = function addAbortSignal(signal, stream2) {
  function onAbort() {
    stream2.destroy(signal.reason);
  }
  if (signal.aborted) onAbort();
  else signal.addEventListener("abort", onAbort);
  return stream2;
};
exports.Stream = exports;
exports.Readable = class Readable extends stream.Readable {
  constructor(opts = {}) {
    super(__spreadProps(__spreadValues({}, opts), {
      byteLength: null,
      byteLengthReadable: null,
      map: null,
      mapReadable: null
    }));
    if (this._construct) this._open = this._construct;
    if (this._read !== stream.Readable.prototype._read) {
      this._read = read.bind(this, this._read);
    }
    if (this._destroy !== stream.Stream.prototype._destroy) {
      this._destroy = destroy.bind(this, this._destroy);
    }
  }
  get closed() {
    return !exports.isReadable(this);
  }
  get errored() {
    return stream.getStreamError(this);
  }
  push(chunk, encoding) {
    if (typeof chunk === "string") {
      chunk = b4a.from(chunk, encoding || defaultEncoding);
    }
    return super.push(chunk);
  }
  unshift(chunk, encoding) {
    if (typeof chunk === "string") {
      chunk = b4a.from(chunk, encoding || defaultEncoding);
    }
    super.unshift(chunk);
  }
  static fromWeb(readableStream, opts = {}) {
    return bridge.readableFromWeb(readableStream, opts);
  }
  static toWeb(readable, opts = {}) {
    return bridge.readableToWeb(readable, opts);
  }
  [Symbol.asyncDispose]() {
    return __async(this, null, function* () {
      if (!this.destroyed) this.destroy();
      yield new Promise((resolve) => exports.finished(this, resolve));
    });
  }
};
exports.Writable = class Writable extends stream.Writable {
  constructor(opts = {}) {
    super(__spreadProps(__spreadValues({}, opts), {
      byteLength: null,
      byteLengthWritable,
      map: null,
      mapWritable: null
    }));
    if (this._construct) this._open = this._construct;
    if (this._write !== stream.Writable.prototype._write) {
      this._write = write.bind(this, this._write);
    }
    if (this._destroy !== stream.Stream.prototype._destroy) {
      this._destroy = destroy.bind(this, this._destroy);
    }
  }
  get closed() {
    return !exports.isWritable(this);
  }
  get errored() {
    return stream.getStreamError(this);
  }
  write(chunk, encoding, cb) {
    if (typeof encoding === "function") {
      cb = encoding;
      encoding = null;
    }
    if (typeof chunk === "string") {
      encoding = encoding || defaultEncoding;
      chunk = b4a.from(chunk, encoding);
    } else {
      encoding = "buffer";
    }
    const result = super.write({ chunk, encoding });
    if (cb) stream.Writable.drained(this).then(() => cb(null), cb);
    return result;
  }
  end(chunk, encoding, cb) {
    if (typeof chunk === "function") {
      cb = chunk;
      chunk = null;
    } else if (typeof encoding === "function") {
      cb = encoding;
      encoding = null;
    }
    if (typeof chunk === "string") {
      encoding = encoding || defaultEncoding;
      chunk = b4a.from(chunk, encoding || defaultEncoding);
    } else {
      encoding = "buffer";
    }
    const result = chunk !== void 0 && chunk !== null ? super.end({ chunk, encoding }) : super.end();
    if (cb) this.once("finish", () => cb(null));
    return result;
  }
  static fromWeb(writableStream, opts = {}) {
    return bridge.writableFromWeb(writableStream, opts);
  }
  static toWeb(writable) {
    return bridge.writableToWeb(writable);
  }
  [Symbol.asyncDispose]() {
    return __async(this, null, function* () {
      if (!this.destroyed) this.destroy();
      yield new Promise((resolve) => exports.finished(this, resolve));
    });
  }
};
exports.Duplex = class Duplex extends stream.Duplex {
  constructor(opts = {}) {
    super(__spreadProps(__spreadValues({}, opts), {
      byteLength: null,
      byteLengthReadable: null,
      byteLengthWritable,
      map: null,
      mapReadable: null,
      mapWritable: null
    }));
    if (this._construct) this._open = this._construct;
    if (this._read !== stream.Readable.prototype._read) {
      this._read = read.bind(this, this._read);
    }
    if (this._write !== stream.Duplex.prototype._write) {
      this._write = write.bind(this, this._write);
    }
    if (this._destroy !== stream.Stream.prototype._destroy) {
      this._destroy = destroy.bind(this, this._destroy);
    }
  }
  push(chunk, encoding) {
    if (typeof chunk === "string") {
      chunk = b4a.from(chunk, encoding || defaultEncoding);
    }
    return super.push(chunk);
  }
  unshift(chunk, encoding) {
    if (typeof chunk === "string") {
      chunk = b4a.from(chunk, encoding || defaultEncoding);
    }
    super.unshift(chunk);
  }
  write(chunk, encoding, cb) {
    if (typeof encoding === "function") {
      cb = encoding;
      encoding = null;
    }
    if (typeof chunk === "string") {
      encoding = encoding || defaultEncoding;
      chunk = b4a.from(chunk, encoding);
    } else {
      encoding = "buffer";
    }
    const result = super.write({ chunk, encoding });
    if (cb) stream.Writable.drained(this).then(() => cb(null), cb);
    return result;
  }
  end(chunk, encoding, cb) {
    if (typeof chunk === "function") {
      cb = chunk;
      chunk = null;
    } else if (typeof encoding === "function") {
      cb = encoding;
      encoding = null;
    }
    if (typeof chunk === "string") {
      encoding = encoding || defaultEncoding;
      chunk = b4a.from(chunk, encoding);
    } else {
      encoding = "buffer";
    }
    const result = chunk !== void 0 && chunk !== null ? super.end({ chunk, encoding }) : super.end();
    if (cb) this.once("finish", () => cb(null));
    return result;
  }
  static fromWeb({ readable: readableStream, writable: writableStream }, opts) {
    return bridge.duplexFromWeb(readableStream, writableStream, opts);
  }
  static toWeb(duplex) {
    return bridge.duplexToWeb(duplex);
  }
};
var DuplexSide = class extends exports.Duplex {
  constructor(opts) {
    super(opts);
    this._otherSide = null;
    this._cb = null;
  }
  _read() {
    const cb = this._cb;
    if (!cb) return;
    this._cb = null;
    cb();
  }
  _write(chunk, encoding, cb) {
    this._otherSide.push(chunk, encoding);
    this._otherSide._cb = cb;
  }
  _final(cb) {
    this._otherSide.on("end", cb);
    this._otherSide.push(null);
  }
};
exports.duplexPair = function duplexPair(opts) {
  const sideA = new DuplexSide(opts);
  const sideB = new DuplexSide(opts);
  sideA._otherSide = sideB;
  sideB._otherSide = sideA;
  return [sideA, sideB];
};
exports.Transform = class Transform extends stream.Transform {
  constructor(opts = {}) {
    super(__spreadProps(__spreadValues({}, opts), {
      byteLength: null,
      byteLengthReadable: null,
      byteLengthWritable,
      map: null,
      mapReadable: null,
      mapWritable: null
    }));
    if (this._transform !== stream.Transform.prototype._transform) {
      this._transform = transform.bind(this, this._transform);
    } else {
      this._transform = passthrough;
    }
  }
  push(chunk, encoding) {
    if (typeof chunk === "string") {
      chunk = b4a.from(chunk, encoding || defaultEncoding);
    }
    return super.push(chunk);
  }
  unshift(chunk, encoding) {
    if (typeof chunk === "string") {
      chunk = b4a.from(chunk, encoding || defaultEncoding);
    }
    super.unshift(chunk);
  }
  write(chunk, encoding, cb) {
    if (typeof encoding === "function") {
      cb = encoding;
      encoding = null;
    }
    if (typeof chunk === "string") {
      encoding = encoding || defaultEncoding;
      chunk = b4a.from(chunk, encoding);
    } else {
      encoding = "buffer";
    }
    const result = super.write({ chunk, encoding });
    if (cb) stream.Writable.drained(this).then(() => cb(null), cb);
    return result;
  }
  end(chunk, encoding, cb) {
    if (typeof chunk === "function") {
      cb = chunk;
      chunk = null;
    } else if (typeof encoding === "function") {
      cb = encoding;
      encoding = null;
    }
    if (typeof chunk === "string") {
      encoding = encoding || defaultEncoding;
      chunk = b4a.from(chunk, encoding);
    } else {
      encoding = "buffer";
    }
    const result = chunk !== void 0 && chunk !== null ? super.end({ chunk, encoding }) : super.end();
    if (cb) this.once("finish", () => cb(null));
    return result;
  }
};
exports.PassThrough = class PassThrough extends exports.Transform {
};
exports.finished = function finished(stream2, opts, cb) {
  if (typeof opts === "function") {
    cb = opts;
    opts = {};
  }
  if (!opts) opts = {};
  const { cleanup = false } = opts;
  const done = () => {
    cb(exports.getStreamError(stream2, { all: true }));
    if (cleanup) detach();
  };
  const detach = () => {
    stream2.off("close", done);
    stream2.off("error", noop);
  };
  if (stream2.destroyed) {
    done();
  } else {
    stream2.on("close", done);
    stream2.on("error", noop);
  }
  return detach;
};
function read(read2, cb) {
  read2.call(this, 65536);
  cb(null);
}
function write(write2, data, cb) {
  write2.call(this, data.chunk, data.encoding, cb);
}
function transform(transform2, data, cb) {
  transform2.call(this, data.chunk, data.encoding, cb);
}
function destroy(destroy2, cb) {
  destroy2.call(this, exports.getStreamError(this), cb);
}
function passthrough(data, cb) {
  cb(null, data.chunk);
}
function byteLengthWritable(data) {
  return data.chunk.byteLength;
}
function noop() {
}
