package sqlxx

import "strings"

const (
	bindvar = "(?)"
)

// MakeBulkInsertBindVars ...
func MakeBulkInsertBindVars(recordNum int) string {
	if recordNum > 0 {
		return strings.Repeat(bindvar+",", recordNum-1) + bindvar
	}
	return ""
}
