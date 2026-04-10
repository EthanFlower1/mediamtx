package subscribers

import "errors"

var (
	ErrSubscriberNotFound = errors.New("subscribers: subscriber not found")
	ErrInvalidChannel     = errors.New("subscribers: invalid channel type")
)
