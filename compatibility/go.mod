module example.com/goja-nodejs-compatibility

go 1.25

require github.com/dop251/goja_nodejs v0.0.0

require (
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/dop251/goja v0.0.0-20260806115107-493f22071ef6 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8 // indirect
	golang.org/x/text v0.34.0 // indirect
)

replace github.com/dop251/goja_nodejs => ..
