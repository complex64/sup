package sup

import (
	"testing"
)

func Test_defaultLogger(t *testing.T) {
	SetLogger(DefaultLogger)
	log_(Debug, "testing debug log")
	log_(Info, "testing info log")
	log_(Error, "testing error log")
	SetLogger(nil)
}
