package filbeam

func (s *Service) checkInit() error {
	if s == nil || s.baseURL == "" || s.retrievalDomain == "" {
		return ErrUninitialized
	}
	return s.lifecycle.CheckClosed()
}
