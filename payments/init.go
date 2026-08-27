package payments

// checkInit returns ErrUninitialized when the service is a zero value
// (created without [New]). It also returns the configured Lifecycle error.
func (s *Service) checkInit() error {
	if s == nil || s.filPayCall == nil {
		return ErrUninitialized
	}
	if s.lifecycle == nil {
		return nil
	}
	return s.lifecycle.CheckClosed()
}
