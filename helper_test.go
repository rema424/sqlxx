package sqlxx

import (
	"bytes"
	"io/ioutil"
	"regexp"
	"testing"
	"time"
)

type sqlResultMock struct{ lastInsertID, rowsAffected int64 }

func (m *sqlResultMock) LastInsertId() (int64, error) { return m.lastInsertID, nil }
func (m *sqlResultMock) RowsAffected() (int64, error) { return m.rowsAffected, nil }

func TestCountRows(t *testing.T) {
	type Person struct {
		Name string
	}

	var (
		nilPtrInt            *int       = (*int)(nil)
		nilPtrInt64          *int64     = (*int64)(nil)
		nilPtrStr            *string    = (*string)(nil)
		nilPtrBytes          *[]byte    = (*[]byte)(nil)
		nilPtrStruct         *Person    = (*Person)(nil)
		nilPtrStructSlice    *[]Person  = (*[]Person)(nil)
		nilPtrStructPtrSlice *[]*Person = (*[]*Person)(nil)

		valInt            int       = 111
		valInt64          int64     = 222
		valStr            string    = "333"
		valBytes          []byte    = []byte("asf")
		valStruct         Person    = Person{"Alice"}
		valStructSlice    []Person  = []Person{Person{"Alice"}, Person{"Bob"}}
		valStructPtrSlice []*Person = []*Person{&Person{"Alice"}, &Person{"Bob"}}

		ptrInt            *int       = &valInt
		ptrInt64          *int64     = &valInt64
		ptrStr            *string    = &valStr
		ptrBytes          *[]byte    = &valBytes
		ptrStruct         *Person    = &valStruct
		ptrStructSlice    *[]Person  = &valStructSlice
		ptrStructPtrSlice *[]*Person = &valStructPtrSlice

		ptrZeroInt         *int      = new(int)
		ptrZeroInt64       *int64    = new(int64)
		ptrZeroStr         *string   = new(string)
		ptrZeroBytes       *[]byte   = new([]byte)
		ptrZeroStruct      *Person   = new(Person)
		ptrZeroStructSlice *[]Person = new([]Person)
	)

	tests := []struct {
		input interface{}
		want  int
	}{
		{nil, 0},

		{nilPtrInt, 0},
		{nilPtrInt64, 0},
		{nilPtrStr, 0},
		{nilPtrBytes, 0},
		{nilPtrStruct, 0},
		{nilPtrStructSlice, 0},
		{nilPtrStructPtrSlice, 0},

		{valInt, 1},
		{valInt64, 1},
		{valStr, 1},
		{valBytes, 1},
		{valStruct, 1},
		{valStructSlice, 2},
		{valStructPtrSlice, 2},

		{ptrInt, 1},
		{ptrInt64, 1},
		{ptrStr, 1},
		{ptrBytes, 1},
		{ptrStruct, 1},
		{ptrStructSlice, 2},
		{ptrStructPtrSlice, 2},

		{ptrZeroInt, 1},
		{ptrZeroInt64, 1},
		{ptrZeroStr, 1},
		{ptrZeroBytes, 1},
		{ptrZeroStruct, 1},
		{ptrZeroStructSlice, 0},

		{[]string{}, 0},
		{[]string{"aaa"}, 1},
		{[]string{"aaa", "bbb"}, 2},
		{[]string{"aaa", "bbb", "ccc"}, 3},
		{make([]string, 100), 100},
		{make([]string, 0, 100), 0},
		{make([]string, 50, 100), 50},
		{make([]string, 100), 100},

		{[...]string{}, 0},
		{[...]string{"aaa"}, 1},
		{[...]string{"aaa", "bbb"}, 2},
		{[...]string{"aaa", "bbb", "ccc"}, 3},

		{&sqlResultMock{0, 45}, 45},
		{&sqlResultMock{0, 50}, 50},
	}

	for i, tt := range tests {
		got := countRows(tt.input)
		if got != tt.want {
			t.Errorf("[#%d] countRows(%v %T): want %v, got %v ", i+1, tt.input, tt.input, tt.want, got)
		}
		// t.Logf("[#%d] countRows(%v %T): want %v, got %v ", i+1, tt.input, tt.input, tt.want, got)
	}
}

func TestWriteArgs(t *testing.T) {
	tests := []struct {
		args []interface{}
		want string
	}{
		{nil, "[]"},
		{[]interface{}{}, "[]"},
		{[]interface{}{""}, "[]"},
		{[]interface{}{"aa"}, "[aa]"},
		{[]interface{}{"aa", "bb"}, "[aa, bb]"},
		{[]interface{}{"aa", "bb", "cc"}, "[aa, bb, cc]"},
		{[]interface{}{"aa", 22, true}, "[aa, 22, true]"},
		{[]interface{}{nil}, "[<nil>]"},
		{[]interface{}{(*int)(nil), (*string)(nil)}, "[<nil>, <nil>]"},
	}

	for i, tt := range tests {
		var buf bytes.Buffer
		writeArgs(&buf, tt.args)
		got := buf.String()
		if got != tt.want {
			t.Errorf("#%d: want %s, got %s", i, tt.want, got)
		}
		// t.Logf("#%d: want %s, got %s", i, tt.want, got)
	}

	var buf bytes.Buffer
	writeArgs(&buf, []interface{}{new(int), new(string)})
	got := buf.String()
	r := regexp.MustCompile(`^\[(0xc[\da-f]{9}(, )?)*\]$`)
	if !r.MatchString(got) {
		t.Errorf("#%d: want ^\\[(0xc[\\da-f]{9}(, )?)*\\]$, got %s", len(tests), got)
	}
	// t.Logf("#%d: want ^\\[(0xc[\\da-f]{9}(, )?)*\\]$, got %s", len(tests), got)
}

func TestWriteArgsReflect(t *testing.T) {
	tests := []struct {
		args []interface{}
		want string
	}{
		{nil, "[]"},
		{[]interface{}{}, "[]"},
		{[]interface{}{""}, "[]"},
		{[]interface{}{"aa"}, "[aa]"},
		{[]interface{}{"aa", "bb"}, "[aa, bb]"},
		{[]interface{}{"aa", "bb", "cc"}, "[aa, bb, cc]"},
		{[]interface{}{"aa", 22, true}, "[aa, 22, true]"},
		{[]interface{}{nil}, "[<nil>]"},
		{[]interface{}{(*int)(nil), (*string)(nil)}, "[<nil>, <nil>]"},
		{[]interface{}{new(int), new(string)}, "[0, ]"},
	}

	for i, tt := range tests {
		var buf bytes.Buffer
		writeArgsReflect(&buf, tt.args)
		got := buf.String()
		if got != tt.want {
			t.Errorf("#%d: want %s, got %s", i, tt.want, got)
		}
		// t.Logf("#%d: want %s, got %s", i, tt.want, got)
	}
}

func BenchmarkWriteArgsPointer(b *testing.B) {
	for i := 0; i < b.N; i++ {
		writeArgs(ioutil.Discard, []interface{}{(*int)(nil), (*uint8)(nil), (*string)(nil)})
	}
}

func BenchmarkWriteArgsReflectPointer(b *testing.B) {
	for i := 0; i < b.N; i++ {
		writeArgsReflect(ioutil.Discard, []interface{}{(*int)(nil), (*uint8)(nil), (*string)(nil)})
	}
}

func BenchmarkWriteArgsValue(b *testing.B) {
	for i := 0; i < b.N; i++ {
		writeArgs(ioutil.Discard, []interface{}{int(100), uint8(100), string("100")})
	}
}

func BenchmarkWriteArgsReflectValue(b *testing.B) {
	for i := 0; i < b.N; i++ {
		writeArgsReflect(ioutil.Discard, []interface{}{int(100), uint8(100), string("100")})
	}
}

func TestToMillisec(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want float64
	}{
		{time.Second, 1000},
		{2 * time.Second, 2000},
		{time.Minute, 1000 * 60},
		{time.Microsecond, 0.001},
	}

	for i, tt := range tests {
		got := toMillisec(tt.d)
		if got != tt.want {
			t.Errorf("%d: want %f, got %f", i, tt.want, got)
		}
	}
}
