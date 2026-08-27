package warmstorage

func (s *Service) checkInit() error {
	if s == nil || s.caller == nil {
		return ErrUninitialized
	}
	if s.lifecycle == nil {
		return nil
	}
	return s.lifecycle.CheckClosed()
}
