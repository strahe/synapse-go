package storage

func (s *Service) checkInit() error {
	if s == nil || s.httpClient == nil {
		return ErrUninitialized
	}
	if s.lifecycle == nil {
		return nil
	}
	return s.lifecycle.CheckClosed()
}
