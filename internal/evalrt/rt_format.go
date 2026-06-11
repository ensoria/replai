package evalrt

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Formatter limits, overridable via REPLAI_* environment variables.
var (
	replaiMaxDepth = 5
	replaiMaxItems = 50
	replaiMaxStr   = 2000
)

const replaiMaxHexBytes = 64

// replaiFormat renders a value as a Go-syntax-like repr per spec §4.3:
// pointers are dereferenced, cycles cut, and every truncation is marked
// together with the flag that lifts it.
func replaiFormat(v interface{}) string {
	if v == nil {
		return "nil"
	}
	var b strings.Builder
	replaiFormatValue(&b, reflect.ValueOf(v), 0, map[uintptr]bool{})
	return b.String()
}

// replaiFullTypeOf renders a fully package-qualified type name, e.g.
// "*github.com/ensoria/rest/pkg/rest.Server".
func replaiFullTypeOf(v interface{}) string {
	if v == nil {
		return "nil"
	}
	return replaiFullType(reflect.TypeOf(v))
}

func replaiFullType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Ptr:
		return "*" + replaiFullType(t.Elem())
	case reflect.Slice:
		return "[]" + replaiFullType(t.Elem())
	case reflect.Array:
		return fmt.Sprintf("[%d]%s", t.Len(), replaiFullType(t.Elem()))
	case reflect.Map:
		return "map[" + replaiFullType(t.Key()) + "]" + replaiFullType(t.Elem())
	case reflect.Chan:
		return "chan " + replaiFullType(t.Elem())
	default:
		if t.PkgPath() != "" {
			return t.PkgPath() + "." + t.Name()
		}
		return t.String()
	}
}

func replaiDepthMarker() string {
	return fmt.Sprintf("...(depth limit, use --depth=%d)", replaiMaxDepth*2)
}

func replaiFormatValue(b *strings.Builder, rv reflect.Value, depth int, visited map[uintptr]bool) {
	if !rv.IsValid() {
		b.WriteString("nil")
		return
	}
	t := rv.Type()

	// Special-cases with stable, AI-friendly renderings.
	if t == reflect.TypeOf(time.Time{}) && rv.CanInterface() {
		b.WriteString("time.Time(" + strconv.Quote(rv.Interface().(time.Time).Format(time.RFC3339Nano)) + ")")
		return
	}
	if t == reflect.TypeOf(time.Duration(0)) {
		d := time.Duration(rv.Int())
		fmt.Fprintf(b, "%s (%dns)", d.String(), int64(d))
		return
	}

	switch rv.Kind() {
	case reflect.Bool:
		b.WriteString(strconv.FormatBool(rv.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.WriteString(strconv.FormatInt(rv.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		b.WriteString(strconv.FormatUint(rv.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		b.WriteString(strconv.FormatFloat(rv.Float(), 'g', -1, 64))
	case reflect.Complex64, reflect.Complex128:
		fmt.Fprintf(b, "%v", rv.Complex())
	case reflect.String:
		replaiFormatString(b, rv.String())
	case reflect.Interface:
		if rv.IsNil() {
			b.WriteString("nil")
			return
		}
		replaiFormatValue(b, rv.Elem(), depth, visited)
	case reflect.Ptr:
		replaiFormatPointer(b, rv, depth, visited)
	case reflect.Struct:
		replaiFormatStruct(b, rv, depth, visited)
	case reflect.Slice, reflect.Array:
		replaiFormatList(b, rv, depth, visited)
	case reflect.Map:
		replaiFormatMap(b, rv, depth, visited)
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		if rv.IsNil() {
			b.WriteString("(" + t.String() + ")(nil)")
		} else {
			b.WriteString(t.String())
		}
	default:
		fmt.Fprintf(b, "%v", rv)
	}
}

func replaiFormatString(b *strings.Builder, s string) {
	if len(s) > replaiMaxStr {
		over := len(s) - replaiMaxStr
		b.WriteString(strconv.Quote(s[:replaiMaxStr]))
		fmt.Fprintf(b, "...(+%d chars, use --max-str=%d)", over, len(s))
		return
	}
	b.WriteString(strconv.Quote(s))
}

func replaiFormatPointer(b *strings.Builder, rv reflect.Value, depth int, visited map[uintptr]bool) {
	if rv.IsNil() {
		b.WriteString("(" + rv.Type().String() + ")(nil)")
		return
	}
	ptr := rv.Pointer()
	if visited[ptr] {
		b.WriteString("<cycle: " + rv.Type().String() + ">")
		return
	}
	visited[ptr] = true
	defer delete(visited, ptr)
	b.WriteString("&")
	replaiFormatValue(b, rv.Elem(), depth, visited)
}

func replaiFormatStruct(b *strings.Builder, rv reflect.Value, depth int, visited map[uintptr]bool) {
	t := rv.Type()
	b.WriteString(t.String())
	if depth >= replaiMaxDepth {
		b.WriteString("{" + replaiDepthMarker() + "}")
		return
	}
	b.WriteString("{")
	for i := 0; i < t.NumField(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(t.Field(i).Name + ": ")
		replaiFormatValue(b, rv.Field(i), depth+1, visited)
	}
	b.WriteString("}")
}

func replaiFormatList(b *strings.Builder, rv reflect.Value, depth int, visited map[uintptr]bool) {
	t := rv.Type()
	if rv.Kind() == reflect.Slice {
		if rv.IsNil() {
			b.WriteString("(" + t.String() + ")(nil)")
			return
		}
		if t.Elem().Kind() == reflect.Uint8 {
			replaiFormatBytes(b, rv.Bytes())
			return
		}
		ptr := rv.Pointer()
		if visited[ptr] {
			b.WriteString("<cycle: " + t.String() + ">")
			return
		}
		visited[ptr] = true
		defer delete(visited, ptr)
	}
	b.WriteString(t.String())
	if depth >= replaiMaxDepth {
		b.WriteString("{" + replaiDepthMarker() + "}")
		return
	}
	b.WriteString("{")
	n := rv.Len()
	limit := n
	if limit > replaiMaxItems {
		limit = replaiMaxItems
	}
	for i := 0; i < limit; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		replaiFormatValue(b, rv.Index(i), depth+1, visited)
	}
	if n > limit {
		fmt.Fprintf(b, ", ...(+%d items, use --max-items=%d)", n-limit, n)
	}
	b.WriteString("}")
}

func replaiFormatBytes(b *strings.Builder, data []byte) {
	if utf8.Valid(data) {
		b.WriteString("[]byte(")
		replaiFormatString(b, string(data))
		b.WriteString(")")
		return
	}
	limit := len(data)
	if limit > replaiMaxHexBytes {
		limit = replaiMaxHexBytes
	}
	fmt.Fprintf(b, "[]byte(0x%x", data[:limit])
	if len(data) > limit {
		fmt.Fprintf(b, "...(+%d bytes)", len(data)-limit)
	}
	b.WriteString(")")
}

func replaiFormatMap(b *strings.Builder, rv reflect.Value, depth int, visited map[uintptr]bool) {
	t := rv.Type()
	if rv.IsNil() {
		b.WriteString("(" + t.String() + ")(nil)")
		return
	}
	ptr := rv.Pointer()
	if visited[ptr] {
		b.WriteString("<cycle: " + t.String() + ">")
		return
	}
	visited[ptr] = true
	defer delete(visited, ptr)

	b.WriteString(t.String())
	if depth >= replaiMaxDepth {
		b.WriteString("{" + replaiDepthMarker() + "}")
		return
	}
	type kv struct {
		keyRepr string
		key     reflect.Value
	}
	keys := rv.MapKeys()
	pairs := make([]*kv, 0, len(keys))
	for _, k := range keys {
		var kb strings.Builder
		replaiFormatValue(&kb, k, depth+1, visited)
		pairs = append(pairs, &kv{keyRepr: kb.String(), key: k})
	}
	// Deterministic output: sort map entries by formatted key.
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].keyRepr < pairs[j].keyRepr })

	b.WriteString("{")
	n := len(pairs)
	limit := n
	if limit > replaiMaxItems {
		limit = replaiMaxItems
	}
	for i := 0; i < limit; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(pairs[i].keyRepr + ": ")
		replaiFormatValue(b, rv.MapIndex(pairs[i].key), depth+1, visited)
	}
	if n > limit {
		fmt.Fprintf(b, ", ...(+%d items, use --max-items=%d)", n-limit, n)
	}
	b.WriteString("}")
}
