package support

import "errors"

var (
	ErrTicketNotFound   = errors.New("support: ticket not found")
	ErrCommentNotFound  = errors.New("support: comment not found")
	ErrProviderNotFound = errors.New("support: provider config not found")
)
