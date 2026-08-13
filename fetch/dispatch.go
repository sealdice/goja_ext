package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/go-resty/resty/v2"
	"github.com/sealdice/goja_ext/runtimehost"
)

type dispatchRequest struct {
	url        string
	method     string
	headers    http.Header
	body       []byte
	streamBody *uploadBody
	ctx        context.Context
	cancel     context.CancelFunc
	abort      *dispatchAbortState
	cleanup    func()
}

type dispatchAbortState struct {
	mu      sync.Mutex
	aborted bool
	reason  goja.Value
	handler func(goja.Value)
}

func (s *dispatchAbortState) set(reason goja.Value) {
	s.mu.Lock()
	s.aborted = true
	s.reason = reason
	handler := s.handler
	s.mu.Unlock()
	if handler != nil {
		handler(reason)
	}
}

func (s *dispatchAbortState) get() (goja.Value, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason, s.aborted
}

func (s *dispatchAbortState) setHandler(handler func(goja.Value)) {
	s.mu.Lock()
	s.handler = handler
	reason := s.reason
	aborted := s.aborted
	s.mu.Unlock()
	if aborted && handler != nil {
		handler(reason)
	}
}

func (s *dispatchAbortState) clearHandler() {
	s.mu.Lock()
	s.handler = nil
	s.mu.Unlock()
}

func (data *dispatchRequest) detachAbortListener() {
	if data.cleanup == nil {
		return
	}
	cleanup := data.cleanup
	data.cleanup = nil
	cleanup()
}

func (data *dispatchRequest) finish() {
	data.detachAbortListener()
	if data.streamBody != nil {
		_ = data.streamBody.Close()
	}
	data.cancel()
}

func (data *dispatchRequest) finishOffLoop() {
	if data.streamBody != nil {
		_ = data.streamBody.Close()
	}
	data.cancel()
}

func (data *dispatchRequest) abortOnLoop(reason goja.Value) {
	if data.streamBody != nil {
		data.streamBody.abortOnLoop(reason)
	}
	data.cancel()
}

type dispatchResponse struct {
	status         int
	statusText     string
	headers        http.Header
	body           io.ReadCloser
	urls           []string
	nullBody       bool
	cleanup        func()
	cleanupOffLoop func()
	abort          *dispatchAbortState
}

func newDispatchFn(
	rt *goja.Runtime,
	scheduler runtimehost.Scheduler,
	client *resty.Client,
) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := rt.NewPromise()
		request, err := parseDispatchRequest(rt, scheduler, call.Argument(0))
		if err != nil {
			_ = reject(rt.NewTypeError(err.Error()))
			return rt.ToValue(promise)
		}

		go func() {
			response, requestErr := doDispatchRequest(request, client)
			scheduled := scheduler.RunOnLoop(func(loopRT *goja.Runtime) {
				if requestErr != nil {
					request.finish()
					if reason, aborted := request.abort.get(); aborted {
						_ = reject(valueOrUndefined(reason))
					} else {
						_ = reject(loopRT.NewGoError(requestErr))
					}
					return
				}

				raw, responseErr := newDispatchResponseObject(loopRT, scheduler, response)
				if responseErr != nil {
					_ = reject(loopRT.NewGoError(responseErr))
					return
				}
				_ = resolve(raw)
			})
			if !scheduled {
				request.finishOffLoop()
				if response != nil && response.body != nil {
					_ = response.body.Close()
				}
			}
		}()

		return rt.ToValue(promise)
	}
}

func parseDispatchRequest(
	rt *goja.Runtime,
	scheduler runtimehost.Scheduler,
	value goja.Value,
) (*dispatchRequest, error) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, errors.New("request descriptor is required")
	}
	descriptor := value.ToObject(rt)
	rawURL := descriptor.Get("url").String()
	if strings.TrimSpace(rawURL) == "" {
		return nil, errors.New("request URL is required")
	}
	method := descriptor.Get("method").String()
	if method == "" {
		method = http.MethodGet
	}

	ctx, cancel := context.WithCancel(context.Background())
	request := &dispatchRequest{
		url:     rawURL,
		method:  method,
		headers: make(http.Header),
		ctx:     ctx,
		cancel:  cancel,
		abort:   &dispatchAbortState{},
	}

	if err := parseDispatchHeaders(rt, descriptor.Get("headers"), request.headers); err != nil {
		cancel()
		return nil, err
	}
	body := descriptor.Get("body")
	if body != nil && !goja.IsUndefined(body) && !goja.IsNull(body) {
		request.body = bytesFromValue(body)
	}
	bodyStream := descriptor.Get("bodyStream")
	if bodyStream != nil && !goja.IsUndefined(bodyStream) && !goja.IsNull(bodyStream) {
		streamBody, err := newUploadBody(rt, scheduler, bodyStream)
		if err != nil {
			cancel()
			return nil, err
		}
		request.streamBody = streamBody
	}
	if err := attachDispatchAbortSignal(rt, request, descriptor.Get("signal")); err != nil {
		request.detachAbortListener()
		if request.streamBody != nil {
			request.streamBody.abortOnLoop(rt.NewGoError(err))
		}
		cancel()
		return nil, err
	}
	return request, nil
}

func parseDispatchHeaders(rt *goja.Runtime, value goja.Value, header http.Header) error {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	list := value.ToObject(rt)
	length := int(list.Get("length").ToInteger())
	for i := range length {
		pair := list.Get(strconv.Itoa(i)).ToObject(rt)
		name := pair.Get("0").String()
		if name == "" {
			return errors.New("request header name must not be empty")
		}
		header.Add(name, pair.Get("1").String())
	}
	return nil
}

func attachDispatchAbortSignal(rt *goja.Runtime, data *dispatchRequest, signal goja.Value) error {
	if signal == nil || goja.IsUndefined(signal) || goja.IsNull(signal) {
		return nil
	}
	signalObject := signal.ToObject(rt)
	aborted, ok := signalObject.Get("aborted").Export().(bool)
	if !ok {
		return errors.New("signal.aborted must be a boolean")
	}
	addEventListener, ok := goja.AssertFunction(signalObject.Get("addEventListener"))
	if !ok {
		return errors.New("signal.addEventListener must be callable")
	}
	if aborted {
		reason := signalObject.Get("reason")
		data.abort.set(reason)
		data.abortOnLoop(reason)
		return nil
	}
	onAbort := rt.ToValue(func(goja.FunctionCall) goja.Value {
		reason := signalObject.Get("reason")
		data.abort.set(reason)
		data.abortOnLoop(reason)
		return goja.Undefined()
	})
	if removeEventListener, ok := goja.AssertFunction(signalObject.Get("removeEventListener")); ok {
		data.cleanup = func() {
			_, _ = removeEventListener(signalObject, rt.ToValue("abort"), onAbort)
		}
	}
	if _, err := addEventListener(signalObject, rt.ToValue("abort"), onAbort); err != nil {
		return fmt.Errorf("signal.addEventListener failed: %w", err)
	}
	return nil
}

func doDispatchRequest(data *dispatchRequest, client *resty.Client) (*dispatchResponse, error) {
	if data.streamBody != nil {
		return doStreamingDispatchRequest(data, client)
	}
	req := client.R().SetContext(data.ctx).SetDoNotParseResponse(true)
	req.Header = data.headers.Clone()
	if data.body != nil {
		req.SetBody(data.body)
	}
	resp, err := req.Execute(data.method, data.url)
	if err != nil {
		return nil, err
	}

	raw := resp.RawResponse
	finalURL := data.url
	redirected := false
	if raw != nil && raw.Request != nil {
		if raw.Request.URL != nil {
			finalURL = raw.Request.URL.String()
		}
		redirected = raw.Request.Response != nil
	}
	urls := []string{finalURL}
	if redirected {
		urls = []string{data.url, finalURL}
	}

	statusText := http.StatusText(resp.StatusCode())
	if raw != nil {
		if _, text, found := strings.Cut(raw.Status, " "); found {
			statusText = text
		}
	}
	nullBody := data.method == http.MethodHead || isNullBodyStatus(resp.StatusCode())
	return &dispatchResponse{
		status:         resp.StatusCode(),
		statusText:     statusText,
		headers:        resp.Header().Clone(),
		body:           resp.RawBody(),
		urls:           urls,
		nullBody:       nullBody,
		cleanup:        data.finish,
		cleanupOffLoop: data.finishOffLoop,
		abort:          data.abort,
	}, nil
}

func doStreamingDispatchRequest(data *dispatchRequest, client *resty.Client) (*dispatchResponse, error) {
	req, err := http.NewRequestWithContext(data.ctx, data.method, data.url, data.streamBody)
	if err != nil {
		return nil, err
	}
	req.Header = data.headers.Clone()
	raw, err := client.GetClient().Do(req)
	if err != nil {
		return nil, err
	}
	if (raw.StatusCode == http.StatusTemporaryRedirect || raw.StatusCode == http.StatusPermanentRedirect) &&
		raw.Header.Get("Location") != "" {
		_ = raw.Body.Close()
		return nil, fmt.Errorf(
			"fetch: cannot replay a streaming request body across %d redirect",
			raw.StatusCode,
		)
	}

	finalURL := data.url
	redirected := false
	if raw.Request != nil {
		if raw.Request.URL != nil {
			finalURL = raw.Request.URL.String()
		}
		redirected = raw.Request.Response != nil
	}
	urls := []string{finalURL}
	if redirected {
		urls = []string{data.url, finalURL}
	}
	statusText := http.StatusText(raw.StatusCode)
	if _, text, found := strings.Cut(raw.Status, " "); found {
		statusText = text
	}
	return &dispatchResponse{
		status:         raw.StatusCode,
		statusText:     statusText,
		headers:        raw.Header.Clone(),
		body:           raw.Body,
		urls:           urls,
		nullBody:       data.method == http.MethodHead || isNullBodyStatus(raw.StatusCode),
		cleanup:        data.finish,
		cleanupOffLoop: data.finishOffLoop,
		abort:          data.abort,
	}, nil
}

func isNullBodyStatus(status int) bool {
	switch status {
	case http.StatusSwitchingProtocols, http.StatusEarlyHints, http.StatusNoContent,
		http.StatusResetContent, http.StatusNotModified:
		return true
	default:
		return false
	}
}

func newDispatchResponseObject(
	rt *goja.Runtime,
	scheduler runtimehost.Scheduler,
	response *dispatchResponse,
) (*goja.Object, error) {
	raw := rt.NewObject()
	_ = raw.Set("status", response.status)
	_ = raw.Set("statusText", response.statusText)
	_ = raw.Set("headers", headerPairs(rt, response.headers))
	_ = raw.Set("urls", response.urls)

	if response.nullBody {
		if response.body != nil {
			_ = response.body.Close()
		}
		response.cleanup()
		_ = raw.Set("body", goja.Null())
	} else {
		cleanup := func() {
			if response.abort != nil {
				response.abort.clearHandler()
			}
			response.cleanup()
		}
		cleanupOffLoop := func() {
			if response.abort != nil {
				response.abort.clearHandler()
			}
			if response.cleanupOffLoop != nil {
				response.cleanupOffLoop()
			}
		}
		stream, err := fetchReadableStream(
			rt,
			scheduler,
			response.body,
			cleanup,
			cleanupOffLoop,
			response.abort,
		)
		if err != nil {
			return nil, err
		}
		_ = raw.Set("body", stream)
	}
	return raw, nil
}

func headerPairs(rt *goja.Runtime, header http.Header) *goja.Object {
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := rt.NewArray()
	index := 0
	for _, key := range keys {
		for _, value := range header.Values(key) {
			_ = pairs.Set(strconv.Itoa(index), rt.NewArray(key, value))
			index++
		}
	}
	return pairs
}

func valueOrUndefined(value goja.Value) goja.Value {
	if value == nil {
		return goja.Undefined()
	}
	return value
}

func bytesFromValue(value goja.Value) []byte {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	if object, ok := value.(*goja.Object); ok {
		if bytes, ok := bytesFromArrayBufferView(object); ok {
			return bytes
		}
	}
	switch exported := value.Export().(type) {
	case string:
		return []byte(exported)
	case []byte:
		return append([]byte(nil), exported...)
	case goja.ArrayBuffer:
		return append([]byte(nil), exported.Bytes()...)
	default:
		return []byte(value.String())
	}
}

func bytesFromArrayBufferView(object *goja.Object) ([]byte, bool) {
	bufferValue := object.Get("buffer")
	if bufferValue == nil || goja.IsUndefined(bufferValue) || goja.IsNull(bufferValue) {
		return nil, false
	}
	arrayBuffer, ok := bufferValue.Export().(goja.ArrayBuffer)
	if !ok {
		return nil, false
	}
	byteLengthValue := object.Get("byteLength")
	if byteLengthValue == nil || goja.IsUndefined(byteLengthValue) || goja.IsNull(byteLengthValue) {
		return nil, false
	}
	byteOffset := 0
	if value := object.Get("byteOffset"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
		byteOffset = int(value.ToInteger())
	}
	byteLength := int(byteLengthValue.ToInteger())
	bufferBytes := arrayBuffer.Bytes()
	if byteOffset < 0 || byteLength < 0 || byteOffset > len(bufferBytes) || byteOffset+byteLength > len(bufferBytes) {
		return nil, false
	}
	return append([]byte(nil), bufferBytes[byteOffset:byteOffset+byteLength]...), true
}
