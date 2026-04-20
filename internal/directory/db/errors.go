package db

import "errors"

// ErrNotFound is returned by CRUD operations when the requested record does
// not exist in the database.
var ErrNotFound = errors.New("not found")

// timeFormat is the canonical timestamp layout used for all TEXT-encoded
// timestamps in the directory database.
const timeFormat = "2006-01-02T15:04:05.000Z"
