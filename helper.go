package sqlxx

import (
	"database/sql"
	"fmt"
	"io"
	"reflect"
)

func countRows(obj interface{}) int {
	if obj == nil {
		return 0
	}

	switch obj := obj.(type) {
	case []byte, string:
		return 1
	case *[]byte, *string:
		if reflect.ValueOf(obj).IsNil() {
			return 0
		}
		return 1
	case sql.Result:
		n, _ := obj.RowsAffected()
		return int(n)
	default:
		rv := reflect.ValueOf(obj)
		if rv.Kind() == reflect.Ptr {
			if rv.IsNil() {
				return 0
			}
			rv = reflect.Indirect(rv)
		}
		switch rv.Kind() {
		case reflect.Array:
			return rv.Len()
		case reflect.Slice:
			return rv.Len()
		}
	}

	return 1
}

func stringArgs(w io.Writer, args []interface{}) {
	w.Write([]byte("["))
	for i, arg := range args {
		if i != 0 {
			w.Write([]byte(", "))
		}
		if arg == nil {
			fmt.Fprint(w, nil)
		} else if v := reflect.ValueOf(arg); v.Kind() == reflect.Ptr && v.IsNil() {
			fmt.Fprint(w, arg)
		} else {
			fmt.Fprint(w, reflect.Indirect(v).Interface())
		}
	}
	w.Write([]byte("]"))
}
