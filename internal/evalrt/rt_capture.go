package evalrt

import (
	"encoding/json"
	"reflect"
)

// replaiMaxJSONBytes caps the raw JSON form of a captured value inside the
// child; the parent applies the user-facing --max-output budget afterwards.
const replaiMaxJSONBytes = 262144

// replaiCapture records the result of evaluating the current snippet as an
// expression. The evaluated expression must be the only argument: Go spreads
// a multi-value call across a variadic parameter list only in that form.
// The evaluation context is reached through replaiGlobalCtx.
//
// Trailing error heuristic: if the last value implements error it becomes
// value.err; if it is an untyped nil in a multi-value result it is assumed to
// be a nil error (the dominant (T, error) Go signature). Documented in README.
func replaiCapture(vs ...interface{}) {
	rc := replaiGlobalCtx
	if rc == nil || len(vs) == 0 {
		return
	}
	var errVal *ReplaiErrValue
	last := vs[len(vs)-1]
	if e, ok := last.(error); ok {
		rv := reflect.ValueOf(last)
		if rv.Kind() == reflect.Ptr && rv.IsNil() {
			errVal = &ReplaiErrValue{Nil: true, Type: replaiFullTypeOf(last)}
		} else {
			errVal = &ReplaiErrValue{Nil: false, Message: e.Error(), Type: replaiFullTypeOf(last)}
		}
		vs = vs[:len(vs)-1]
	} else if last == nil && len(vs) >= 2 {
		errVal = &ReplaiErrValue{Nil: true}
		vs = vs[:len(vs)-1]
	}
	values := make([]*ReplaiValue, 0, len(vs))
	for _, v := range vs {
		values = append(values, &ReplaiValue{
			Repr: replaiFormat(v),
			Type: replaiFullTypeOf(v),
			JSON: replaiJSON(v),
		})
	}
	if errVal != nil {
		if len(values) == 0 {
			values = append(values, &ReplaiValue{Err: errVal})
		} else {
			values[len(values)-1].Err = errVal
		}
	}
	rc.result.Values = append(rc.result.Values, values...)
}

// replaiDefined reports a variable newly defined or updated by the snippet.
func (rc *replaiCtx) replaiDefined(name string, v interface{}) {
	rc.result.Defined = append(rc.result.Defined, &ReplaiDefined{Name: name, Type: replaiFullTypeOf(v)})
}

// replaiUse keeps session variables compile-time "used"; it does nothing.
func replaiUse(vs ...interface{}) {}

// replaiDiscard evaluates a replayed expression entry for its side effects
// and discards the results.
func replaiDiscard(vs ...interface{}) {}

func replaiJSON(v interface{}) json.RawMessage {
	defer func() { _ = recover() }()
	data, err := json.Marshal(v)
	if err != nil || len(data) > replaiMaxJSONBytes {
		return nil
	}
	return json.RawMessage(data)
}
