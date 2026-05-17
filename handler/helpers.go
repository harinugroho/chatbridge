package handler

import (
	"context"
	"encoding/json"
	"fmt"
)

// contextType is an alias for context.Context used in handler methods.
type contextType = context.Context

// contextBackground returns a background context.
func contextBackground() context.Context {
	return context.Background()
}

// toInt converts various numeric types to int.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case float32:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

// buildSessionID creates a session ID from account and inbox IDs.
func buildSessionID(accountID, inboxID int) string {
	return fmt.Sprintf("chatwoot-%d-%d", accountID, inboxID)
}
