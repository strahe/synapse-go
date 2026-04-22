package payments

// checkInit returns ErrUninitialized when the service is a zero value
// (created without [New]). It also returns ErrClosed when the owning
// Client's Lifecycle has been closed. Every exported method calls this
// first to avoid nil-pointer panics and to refuse new work after close.
func (s *Service) checkInit() error {
	if s == nil || s.filPayCall == nil {
		return ErrUninitialized
	}
	return s.lifecycle.CheckClosed()
}
