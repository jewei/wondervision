package documents

import "errors"

type Revision [32]byte

type saveState struct {
	saved   Revision
	pending *pendingSave
	next    uint64
}

type pendingSave struct {
	token  uint64
	source Revision
}

func (s *saveState) begin(source Revision) (uint64, error) {
	if s.pending != nil {
		return 0, errors.New("a save is already in progress")
	}
	s.next++
	s.pending = &pendingSave{token: s.next, source: source}
	return s.next, nil
}

func (s *saveState) complete(token uint64, success bool) bool {
	if s.pending == nil || token != s.pending.token {
		return false
	}
	if success {
		s.saved = s.pending.source
	}
	s.pending = nil
	return true
}

func (s *saveState) dirty(current Revision) bool { return current != s.saved }
