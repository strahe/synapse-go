package filbeam

func (s *Service) checkInit() error {
	if s == nil || s.baseURL == "" {
		return ErrUninitialized
	}
	return s.lifecycle.CheckClosed()
}
