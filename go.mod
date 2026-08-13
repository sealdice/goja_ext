module github.com/sealdice/goja_ext

go 1.25

require (
	github.com/dop251/base64dec v0.0.0-20231022112746-c6c9f9a96217
	github.com/dop251/goja v0.0.0-20260806115107-493f22071ef6
	github.com/dop251/goja_nodejs v0.0.0-20211022123610-8dd9abb0616d
	github.com/go-resty/resty/v2 v2.16.5
	github.com/gorilla/websocket v1.5.3
	github.com/ncruces/go-sqlite3 v0.32.0
	github.com/spf13/afero v1.9.5
	go.uber.org/goleak v1.3.0
	golang.org/x/crypto v0.48.0
	golang.org/x/net v0.49.0
	golang.org/x/text v0.34.0
)

require (
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/tetratelabs/wazero v1.11.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

replace github.com/dop251/goja_nodejs => .
