package streamclaims

import "context"

// contextBackground returns context.Background(). It is a tiny wrapper so
// that callers within this package do not need to import "context" separately,
// and to make the package easier to test (could be replaced with a test
// context in a future refactor). Unexported — do not expose context management
// to the package API.
func contextBackground() context.Context { return context.Background() }
