package common

import (
	"fmt"
	"os"
)

func Abort(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func Panic(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}
