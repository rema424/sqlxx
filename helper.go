package sqlxx

import (
	"database/sql"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/jmoiron/sqlx"
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
	case *sqlx.Rows:
		var n int
		for obj.Next() {
			n++
		}
		return n
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

func writeArgs(w io.Writer, args []interface{}) {
	_, _ = w.Write([]byte("["))
	for i, arg := range args {
		if i != 0 {
			_, _ = w.Write([]byte(", "))
		}
		if arg == nil {
			fmt.Fprint(w, nil)
		} else {
			fmt.Fprint(w, arg)
		}
	}
	_, _ = w.Write([]byte("]"))
}

func writeArgsReflect(w io.Writer, args []interface{}) {
	_, _ = w.Write([]byte("["))
	for i, arg := range args {
		if i != 0 {
			_, _ = w.Write([]byte(", "))
		}
		if arg == nil {
			fmt.Fprint(w, nil)
		} else if v := reflect.ValueOf(arg); v.Kind() != reflect.Ptr {
			fmt.Fprint(w, arg)
		} else if v.IsNil() {
			fmt.Fprint(w, arg)
		} else {
			fmt.Fprint(w, reflect.Indirect(v).Interface())
		}
	}
	_, _ = w.Write([]byte("]"))
}

func toMillisec(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
