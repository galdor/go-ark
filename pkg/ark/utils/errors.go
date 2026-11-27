package utils

import (
	"errors"
	"io/fs"
	"net"
	"slices"
)

func UnwrapOpError(err error, names ...string) error {
	var fsErr *fs.PathError
	var opErr *net.OpError

	if errors.As(err, &fsErr) && slices.Contains(names, fsErr.Op) {
		return fsErr.Err
	} else if errors.As(err, &opErr) && slices.Contains(names, opErr.Op) {
		return opErr.Err
	}

	return err
}
