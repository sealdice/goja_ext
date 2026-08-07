(function () {
  var TransformStream = globalThis.__goja_ext_streams_transform_stream;
  var encodeUTF8 = globalThis.__goja_ext_streams_encode_utf8;
  var decodeUTF8 = globalThis.__goja_ext_streams_decode_utf8;

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
      if (label === undefined) {
        label = "utf-8";
      }
      label = String(label).toLowerCase();
      if (label !== "utf-8" && label !== "utf8") {
        throw new RangeError("The encoding label is not supported");
      }
      if (options === undefined || options === null) {
        options = {};
      }

      var fatal = Boolean(options.fatal);
      var ignoreBOM = Boolean(options.ignoreBOM);
      var pending = new Uint8Array(0);
      var firstChunk = true;
      var transform = new TransformStream({
        transform: function (chunk, controller) {
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
          firstChunk = Boolean(decoded.bomPending);
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
    TextEncoderStream: TextEncoderStream,
    TextDecoderStream: TextDecoderStream,
  };
})()
