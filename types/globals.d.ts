export {};

declare global {
  interface RequestInit {
    duplex?: "half";
  }

  var process: typeof import("process");
  var fs: typeof import("fs");
}
