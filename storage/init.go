package storage

func (s *Service) checkInit() error {
	if s == nil || s.httpClient == nil {
		return ErrUninitialized
	}
	return s.lifecycle.CheckClosed()
}
