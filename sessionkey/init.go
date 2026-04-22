package sessionkey

func (s *Service) checkInit() error {
	if s == nil || s.registryCall == nil {
		return ErrUninitialized
	}
	return s.lifecycle.CheckClosed()
}
