package fetch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/go-resty/resty/v2"
)

type fetchRequestData struct {
	url     string
	method  string
	headers *headersData
	body    []byte
	ctx     context.Context
	cancel  context.CancelFunc
	abort   *fetchAbortState
}

type fetchAbortState struct {
	mu     sync.Mutex
	reason goja.Value
}

func (s *fetchAbortState) set(reason goja.Value) {
	s.mu.Lock()
	s.reason = reason
	s.mu.Unlock()
}

func (s *fetchAbortState) get() goja.Value {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason
}

// EnableFetch registers the global fetch(url, init) -> Promise<Response>.
// opts configure the Go-side resty executor; none of them are exposed to JS.
func EnableFetch(rt *goja.Runtime, loop *eventloop.EventLoop, opts ...FetchOption) error {
	if loop == nil {
		return errors.New("JS event loop is required for fetch")
	}
	client := newClient(opts...)
	return rt.Set("fetch", newFetchFn(rt, loop, client))
}

func newFetchFn(rt *goja.Runtime, loop *eventloop.EventLoop, client *resty.Client) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := rt.NewPromise()

		requestData, err := parseFetchRequest(rt, call.Argument(0), call.Argument(1))
		if err != nil {
			_ = reject(rt.NewTypeError(err.Error()))
			return rt.ToValue(promise)
		}

		go func() {
			responseData, err := doFetchRequest(requestData, client)
			loop.RunOnLoop(func(loopRT *goja.Runtime) {
				switch {
				case err == nil:
					response := loopRT.NewObject()
					bindResponse(loopRT, response, responseData)
					_ = resolve(response)
				case requestData.abort.get() != nil:
					_ = reject(requestData.abort.get())
				default:
					_ = reject(loopRT.NewTypeError(err.Error()))
				}
			})
		}()

		return rt.ToValue(promise)
	}
}

func parseFetchRequest(rt *goja.Runtime, input goja.Value, init goja.Value) (*fetchRequestData, error) {
	if goja.IsUndefined(input) || goja.IsNull(input) || strings.TrimSpace(input.String()) == "" {
		return nil, errors.New("url parameter missing")
	}

	data := &fetchRequestData{
		url:     input.String(),
		method:  http.MethodGet,
		headers: newHeadersData(),
		abort:   &fetchAbortState{},
	}
	data.ctx, data.cancel = context.WithCancel(context.Background())

	if obj, ok := input.(*goja.Object); ok {
		if urlValue := obj.Get("url"); urlValue != nil && !goja.IsUndefined(urlValue) && !goja.IsNull(urlValue) {
			data.url = urlValue.String()
		}
		if err := applyFetchInit(rt, data, obj); err != nil {
			return nil, err
		}
	}

	if !goja.IsUndefined(init) && !goja.IsNull(init) {
		if obj, ok := init.(*goja.Object); ok {
			if err := applyFetchInit(rt, data, obj); err != nil {
				return nil, err
			}
		}
	}
	applyDefaultFetchHeaders(data.headers)

	encodedURL, err := encodeFetchURL(data.url)
	if err != nil {
		return nil, err
	}
	data.url = encodedURL
	return data, nil
}

func applyDefaultFetchHeaders(headers *headersData) {
	if !headers.has("user-agent") {
		headers.set("user-agent", "goja_nodejs-fetch/1.0")
	}
	if !headers.has("accept") {
		headers.set("accept", "*/*")
	}
}

func applyFetchInit(rt *goja.Runtime, data *fetchRequestData, obj *goja.Object) error {
	if method := obj.Get("method"); method != nil && !goja.IsUndefined(method) && !goja.IsNull(method) {
		data.method = strings.ToUpper(method.String())
	}
	if headers := obj.Get("headers"); headers != nil && !goja.IsUndefined(headers) && !goja.IsNull(headers) {
		fillHeaders(rt, data.headers, headers)
	}
	if body := obj.Get("body"); body != nil && !goja.IsUndefined(body) && !goja.IsNull(body) {
		bodyBytes, err := bytesFromBodyValue(data.headers, body)
		if err != nil {
			return err
		}
		data.body = bodyBytes
	}
	if signal := obj.Get("signal"); signal != nil && !goja.IsUndefined(signal) && !goja.IsNull(signal) {
		attachFetchAbortSignal(rt, data, signal)
	}
	return nil
}

func attachFetchAbortSignal(rt *goja.Runtime, data *fetchRequestData, signal goja.Value) {
	signalObj, ok := signal.(*goja.Object)
	if !ok {
		signalObj = signal.ToObject(rt)
	}
	if signalObj == nil {
		return
	}
	addEventListener, ok := goja.AssertFunction(signalObj.Get("addEventListener"))
	if !ok {
		return
	}
	cancelFn := rt.ToValue(func(call goja.FunctionCall) goja.Value {
		if eventObj, ok := call.Argument(0).(*goja.Object); ok {
			data.abort.set(eventObj.Get("reason"))
		} else {
			data.abort.set(call.Argument(0))
		}
		data.cancel()
		return goja.Undefined()
	})
	_, _ = addEventListener(signalObj, rt.ToValue("abort"), cancelFn)
}

func bytesFromBodyValue(headers *headersData, body goja.Value) ([]byte, error) {
	if obj, ok := body.(*goja.Object); ok {
		if formData := formDataFromObject(obj); formData != nil {
			return encodeMultipartFormData(headers, formData)
		}
		if isURLSearchParams(obj) {
			if !headers.has("content-type") {
				headers.set("content-type", "application/x-www-form-urlencoded;charset=UTF-8")
			}
			toString, ok := goja.AssertFunction(obj.Get("toString"))
			if !ok {
				return nil, errors.New("URLSearchParams body cannot be serialized")
			}
			encoded, err := toString(obj)
			if err != nil {
				return nil, err
			}
			return []byte(encoded.String()), nil
		}
	}
	return bytesFromValue(body), nil
}

func formDataFromObject(obj *goja.Object) *formDataData {
	internal := obj.Get(formDataDataKey)
	if internal == nil || goja.IsUndefined(internal) || goja.IsNull(internal) {
		return nil
	}
	data, _ := internal.Export().(*formDataData)
	return data
}

func encodeMultipartFormData(headers *headersData, formData *formDataData) ([]byte, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, entry := range formData.entries {
		var part io.Writer
		var err error
		if entry.isFile {
			part, err = writer.CreateFormFile(entry.name, entry.filename)
		} else {
			part, err = writer.CreateFormField(entry.name)
		}
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(part, entry.value); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if !headers.has("content-type") {
		headers.set("content-type", writer.FormDataContentType())
	}
	return buf.Bytes(), nil
}

func isURLSearchParams(obj *goja.Object) bool {
	ctor := obj.Get("constructor")
	if ctorObj, ok := ctor.(*goja.Object); ok {
		if name := ctorObj.Get("name"); name != nil && name.String() == "URLSearchParams" {
			return true
		}
	}
	for _, method := range []string{"append", "delete", "get", "getAll", "has", "set", "sort", "toString"} {
		value := obj.Get(method)
		if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
			return false
		}
	}
	return obj.String() != "[object Object]"
}

func encodeFetchURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parsed.RawQuery = parsed.Query().Encode()
	return parsed.String(), nil
}

func doFetchRequest(data *fetchRequestData, client *resty.Client) (*responseData, error) {
	req := client.R().SetContext(data.ctx)
	req.Header = data.headers.toHTTPHeader()
	if data.body != nil {
		req.SetBody(data.body)
	}
	resp, err := req.Execute(data.method, data.url)
	if err != nil {
		return nil, err
	}
	return &responseData{
		status:     resp.StatusCode(),
		statusText: resp.Status(),
		headers:    newHeadersDataFromHTTPHeader(resp.Header()),
		bodyBytes:  append([]byte(nil), resp.Body()...),
		url:        data.url,
		method:     data.method,
	}, nil
}

func newHeadersDataFromHTTPHeader(header http.Header) *headersData {
	data := newHeadersData()
	for key, values := range header {
		data.store[strings.ToLower(key)] = append([]string(nil), values...)
	}
	return data
}

func (h *headersData) toHTTPHeader() http.Header {
	header := make(http.Header)
	for key, values := range h.store {
		header[textproto.CanonicalMIMEHeaderKey(key)] = append([]string(nil), values...)
	}
	return header
}
