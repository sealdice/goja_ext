# require 模块

Node.js 风格的模块系统（goja_nodejs require），支持 CommonJS 加载与原生（Go）模块注册。

## 能力

- `require.Registry`：可跨 runtime 复用的模块注册表与编译缓存。
- `registry.Enable(rt)` 安装全局 `require()`。
- `Registry.RegisterNativeModule(name, loader)`：注册 Go 原生模块（按 registry 隔离）。
- `require.RegisterCoreModule(name, loader)`：注册全局 core 模块（自动提供 `node:` 前缀别名）。
- `require.RegisterNativeModule(name, loader)`：注册全局原生模块。
- 支持 `./`、`../`、绝对路径、`node_modules` 查找、`package.json` `main`、`.js`/`.json`。
- `WithLoader` / `WithPathResolver` / `WithGlobalFolders` 配置。

## Go API

```go
registry := new(require.Registry)
req := registry.Enable(runtime)

// 原生模块
registry.RegisterNativeModule("mymod", func(rt *goja.Runtime, module *goja.Object) {
    _ = module.Get("exports").(*goja.Object).Set("hello", func(goja.FunctionCall) goja.Value {
        return rt.ToValue("world")
    })
})
```

## 相关

- `eventloop` 内部使用 `require.Registry` 装配 `require`。
- 各模块包通过 `init()` 调用 `require.RegisterCoreModule` 注册。
