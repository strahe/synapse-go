package spregistry

func (s *Service) checkInit() error {
	if s == nil || s.contract == nil {
		return ErrUninitialized
	}
	return s.lifecycle.CheckClosed()
}
