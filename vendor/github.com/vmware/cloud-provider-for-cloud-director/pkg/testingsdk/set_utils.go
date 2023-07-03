package testingsdk

type Set struct {
	m map[interface{}]bool
}

func NewSet() *Set {
	s := &Set{}
	s.m = make(map[interface{}]bool)
	return s
}

func (s *Set) Add(value interface{}) {
	s.m[value] = true
}

func (s *Set) Contains(value interface{}) bool {
	_, c := s.m[value]
	return c
}

func (s *Set) Size() int {
	return len(s.m)
}
