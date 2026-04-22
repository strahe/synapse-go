package warmstorage

func (s *Service) checkInit() error {
	if s == nil || s.caller == nil {
		return ErrUninitialized
	}
	return s.lifecycle.CheckClosed()
}
