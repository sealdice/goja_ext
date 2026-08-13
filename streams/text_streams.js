(function (TransformStream, encodeUTF8, decodeUTF8) {

  function normalizeLabel(label) {
    if (label === undefined) label = "utf-8";
    label = String(label).trim().toLowerCase();
    if (label !== "utf-8" && label !== "utf8" && label !== "unicode-1-1-utf-8") {
      throw new RangeError("The encoding label is not supported");
    }
    return "utf-8";
  }

  class TextEncoder {
    constructor() {
      this.encoding = "utf-8";
    }

    encode(input) {
      return encodeUTF8(input === undefined ? "" : String(input));
    }

    encodeInto(input, destination) {
      if (!(destination instanceof Uint8Array)) {
        throw new TypeError("TextEncoder.encodeInto destination must be a Uint8Array");
      }
      var source = String(input);
      var read = 0;
      var written = 0;
      while (read < source.length) {
        var first = source.charCodeAt(read);
        var width = first >= 0xd800 && first <= 0xdbff &&
          read + 1 < source.length &&
          source.charCodeAt(read + 1) >= 0xdc00 &&
          source.charCodeAt(read + 1) <= 0xdfff ? 2 : 1;
        var bytes = encodeUTF8(source.slice(read, read + width));
        if (written + bytes.byteLength > destination.byteLength) break;
        destination.set(bytes, written);
        read += width;
        written += bytes.byteLength;
      }
      return { read: read, written: written };
    }
  }

  class TextDecoder {
    constructor(label, options) {
      this.encoding = normalizeLabel(label);
      if (options === undefined || options === null) options = {};
      this.fatal = Boolean(options.fatal);
      this.ignoreBOM = Boolean(options.ignoreBOM);
      this._pending = new Uint8Array(0);
      this._firstChunk = true;
    }

    decode(input, options) {
      if (options === undefined || options === null) options = {};
      var stream = Boolean(options.stream);
      var chunk = input === undefined ? new Uint8Array(0) : input;
      var hadBytes = this._pending.byteLength !== 0 || chunk.byteLength !== 0;
      var decoded = decodeUTF8(
        chunk,
        this._pending,
        !stream,
        this.fatal,
        this._firstChunk && !this.ignoreBOM,
      );
      this._pending = decoded.pending;
      this._firstChunk = Boolean(decoded.bomPending) || (this._firstChunk && !hadBytes);
      if (!stream) {
        this._pending = new Uint8Array(0);
        this._firstChunk = true;
      }
      return decoded.text;
    }
  }

  class TextEncoderStream {
    constructor() {
      var pendingHighSurrogate = "";
      var transform = new TransformStream({
        transform: function (chunk, controller) {
          var text = pendingHighSurrogate + String(chunk);
          pendingHighSurrogate = "";
          if (text.length > 0) {
            var last = text.charCodeAt(text.length - 1);
            if (last >= 0xd800 && last <= 0xdbff) {
              pendingHighSurrogate = text.charAt(text.length - 1);
              text = text.slice(0, -1);
            }
          }
          if (text !== "") {
            controller.enqueue(encodeUTF8(text));
          }
        },
        flush: function (controller) {
          if (pendingHighSurrogate !== "") {
            controller.enqueue(encodeUTF8(pendingHighSurrogate));
            pendingHighSurrogate = "";
          }
        },
      });
      this.encoding = "utf-8";
      this.readable = transform.readable;
      this.writable = transform.writable;
    }
  }

  class TextDecoderStream {
    constructor(label, options) {
      normalizeLabel(label);
      if (options === undefined || options === null) {
        options = {};
      }

      var fatal = Boolean(options.fatal);
      var ignoreBOM = Boolean(options.ignoreBOM);
      var pending = new Uint8Array(0);
      var firstChunk = true;
      var transform = new TransformStream({
        transform: function (chunk, controller) {
          var hadBytes = pending.byteLength !== 0 || chunk.byteLength !== 0;
          var decoded = decodeUTF8(
            chunk,
            pending,
            false,
            fatal,
            firstChunk && !ignoreBOM,
          );
          pending = decoded.pending;
          if (decoded.text !== "") {
            controller.enqueue(decoded.text);
          }
          firstChunk = Boolean(decoded.bomPending) || (firstChunk && !hadBytes);
        },
        flush: function (controller) {
          var decoded = decodeUTF8(
            new Uint8Array(0),
            pending,
            true,
            fatal,
            firstChunk && !ignoreBOM,
          );
          if (decoded.text !== "") {
            controller.enqueue(decoded.text);
          }
        },
      });

      this.encoding = "utf-8";
      this.fatal = fatal;
      this.ignoreBOM = ignoreBOM;
      this.readable = transform.readable;
      this.writable = transform.writable;
    }
  }

  return {
    TextEncoder: TextEncoder,
    TextDecoder: TextDecoder,
    TextEncoderStream: TextEncoderStream,
    TextDecoderStream: TextDecoderStream,
  };
})
