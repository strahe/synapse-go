package spregistry

func (s *Service) checkInit() error {
	if s == nil || s.contract == nil {
		return ErrUninitialized
	}
	if s.lifecycle == nil {
		return nil
	}
	return s.lifecycle.CheckClosed()
}
