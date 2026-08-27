package sessionkey

func (s *Service) checkInit() error {
	if s == nil || s.registryCall == nil {
		return ErrUninitialized
	}
	if s.lifecycle == nil {
		return nil
	}
	return s.lifecycle.CheckClosed()
}
