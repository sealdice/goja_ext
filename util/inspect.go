package util

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

const defaultDepth = 2

func Inspect(rt *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return rt.ToValue("undefined")
		}
		val := call.Argument(0)
		depth := defaultDepth
		if len(call.Arguments) >= 2 {
			if arg1 := call.Argument(1); !goja.IsUndefined(arg1) && !goja.IsNull(arg1) {
				opts := arg1.ToObject(rt)
				if d := opts.Get("depth"); d != nil && !goja.IsUndefined(d) {
					if goja.IsNull(d) {
						depth = 1 << 30
					} else {
						depth = int(d.ToInteger())
					}
				}
			}
		}
		seen := newSeenSet()
		s := inspectValue(rt, val, depth, seen)
		return rt.ToValue(s)
	}
}

type seenSet struct {
	ids map[string]bool
}

func newSeenSet() *seenSet {
	return &seenSet{ids: make(map[string]bool)}
}

func (s *seenSet) has(id string) bool { return s.ids[id] }
func (s *seenSet) mark(id string)     { s.ids[id] = true }
func (s *seenSet) unmark(id string)   { delete(s.ids, id) }

func inspectValue(rt *goja.Runtime, v goja.Value, depth int, seen *seenSet) string {
	if v == nil || goja.IsUndefined(v) {
		return "undefined"
	}
	if goja.IsNull(v) {
		return "null"
	}
	if _, ok := v.(*goja.Symbol); ok {
		return "Symbol(" + v.String() + ")"
	}
	if _, ok := v.Export().(*big.Int); ok {
		return v.String() + "n"
	}

	if obj, ok := v.(*goja.Object); ok {
		if depth < 0 {
			if isArray(obj) {
				return "[Array]"
			}
			return "[Object]"
		}
		return inspectObject(rt, obj, depth, seen)
	}

	switch v.ExportType().String() {
	case "string":
		return quoteString(v.String())
	case "bool":
		if v.ToBoolean() {
			return "true"
		}
		return "false"
	}
	s := v.String()
	if s == "[object Undefined]" {
		return "undefined"
	}
	if s == "[object Null]" {
		return "null"
	}
	return s
}

func inspectObject(rt *goja.Runtime, obj *goja.Object, depth int, seen *seenSet) string {
	id := fmt.Sprintf("%p", obj)
	if seen.has(id) {
		return "[Circular]"
	}
	seen.mark(id)
	defer seen.unmark(id)

	if isArray(obj) {
		return inspectArray(rt, obj, depth, seen)
	}

	if isFunction(obj) {
		name := obj.Get("name")
		nm := ""
		if name != nil {
			nm = name.String()
		}
		if nm != "" {
			return fmt.Sprintf("[Function: %s]", nm)
		}
		return "[Function (anonymous)]"
	}

	switch objectTypeName(rt, obj) {
	case "Date":
		return callStringMethod(obj, "toISOString")
	case "RegExp":
		return obj.String()
	case "Map":
		return inspectMap(rt, obj, depth, seen)
	case "Set":
		return inspectSet(rt, obj, depth, seen)
	case "Error":
		return inspectError(obj)
	}

	keys := obj.Keys()
	if len(keys) == 0 {
		return "{}"
	}

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := obj.Get(k)
		parts = append(parts, fmt.Sprintf("%s: %s", formatKey(k), inspectValue(rt, v, depth-1, seen)))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func objectTypeName(rt *goja.Runtime, obj *goja.Object) string {
	className := obj.ClassName()
	if className != "Object" {
		return className
	}
	for _, name := range []string{"Map", "Set"} {
		constructor, ok := rt.Get(name).(*goja.Object)
		if !ok {
			continue
		}
		prototype, ok := constructor.Get("prototype").(*goja.Object)
		if ok && hasPrototype(obj, prototype) {
			return name
		}
	}
	return className
}

func hasPrototype(obj, expected *goja.Object) bool {
	for prototype := obj.Prototype(); prototype != nil; prototype = prototype.Prototype() {
		if prototype.SameAs(expected) {
			return true
		}
	}
	return false
}

func callStringMethod(obj *goja.Object, name string) string {
	fn, ok := goja.AssertFunction(obj.Get(name))
	if !ok {
		return obj.String()
	}
	result, err := fn(obj)
	if err != nil {
		return obj.String()
	}
	return result.String()
}

func inspectMap(rt *goja.Runtime, obj *goja.Object, depth int, seen *seenSet) string {
	parts := make([]string, 0, int(obj.Get("size").ToInteger()))
	forEach, ok := goja.AssertFunction(obj.Get("forEach"))
	if !ok {
		return "Map(0) {}"
	}
	callback := rt.ToValue(func(call goja.FunctionCall) goja.Value {
		value := inspectValue(rt, call.Argument(0), depth-1, seen)
		key := inspectValue(rt, call.Argument(1), depth-1, seen)
		parts = append(parts, key+" => "+value)
		return goja.Undefined()
	})
	if _, err := forEach(obj, callback); err != nil {
		return obj.String()
	}
	if len(parts) == 0 {
		return "Map(0) {}"
	}
	return fmt.Sprintf("Map(%d) { %s }", len(parts), strings.Join(parts, ", "))
}

func inspectSet(rt *goja.Runtime, obj *goja.Object, depth int, seen *seenSet) string {
	parts := make([]string, 0, int(obj.Get("size").ToInteger()))
	forEach, ok := goja.AssertFunction(obj.Get("forEach"))
	if !ok {
		return "Set(0) {}"
	}
	callback := rt.ToValue(func(call goja.FunctionCall) goja.Value {
		parts = append(parts, inspectValue(rt, call.Argument(0), depth-1, seen))
		return goja.Undefined()
	})
	if _, err := forEach(obj, callback); err != nil {
		return obj.String()
	}
	if len(parts) == 0 {
		return "Set(0) {}"
	}
	return fmt.Sprintf("Set(%d) { %s }", len(parts), strings.Join(parts, ", "))
}

func inspectError(obj *goja.Object) string {
	if stack := obj.Get("stack"); stack != nil && !goja.IsUndefined(stack) {
		return stack.String()
	}
	return obj.String()
}

func quoteString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, "\r", `\r`)
	value = strings.ReplaceAll(value, "\t", `\t`)
	return "'" + value + "'"
}

func formatKey(key string) string {
	if isIdentifier(key) {
		return key
	}
	return quoteString(key)
}

func isIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if r != '_' && r != '$' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
				return false
			}
			continue
		}
		if r != '_' && r != '$' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func inspectArray(rt *goja.Runtime, obj *goja.Object, depth int, seen *seenSet) string {
	if depth < 0 {
		return "[Array]"
	}
	lenVal := obj.Get("length")
	if lenVal == nil || goja.IsUndefined(lenVal) {
		return "[]"
	}
	n := int(lenVal.ToInteger())
	if n == 0 {
		return "[]"
	}
	parts := make([]string, n)
	for i := range n {
		v := obj.Get(strconv.Itoa(i))
		parts[i] = inspectValue(rt, v, depth-1, seen)
	}
	return "[ " + strings.Join(parts, ", ") + " ]"
}

func isArray(obj *goja.Object) bool {
	return obj.ClassName() == "Array"
}

func isFunction(obj *goja.Object) bool {
	cn := obj.ClassName()
	return cn == "Function" || cn == "ArrowFunction" || cn == "GeneratorFunction"
}
