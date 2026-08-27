package filbeam

func (s *Service) checkInit() error {
	if s == nil || s.baseURL == "" || s.retrievalDomain == "" {
		return ErrUninitialized
	}
	if s.lifecycle == nil {
		return nil
	}
	return s.lifecycle.CheckClosed()
}
