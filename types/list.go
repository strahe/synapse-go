package types

// ListOptions configures a paginated list call.
//
// Limit == 0 means "use the service default". Each List* method documents
// its own default (for example, "0 means all remaining" or "0 means the
// first 50"). Callers that want every remaining item should loop on Offset
// rather than relying on a universal "no cap" sentinel, because the
// contracts and services the SDK talks to do not agree on one.
type ListOptions struct {
	Offset uint64
	Limit  uint64
}
