/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package util

import "sync"

type Set struct {
	lock     sync.RWMutex
	elements map[string]bool
}

func NewSet(elementList []string) Set {
	s := Set{
		elements: make(map[string]bool),
	}

	// No need for synchronization here since nobody else knows about `s` yet.
	for _, element := range elementList {
		s.elements[element] = true
	}

	return s
}

func (s Set) GetElements() []string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	elementList := make([]string, len(s.elements))
	idx := 0
	for key := range s.elements {
		elementList[idx] = key
		idx++
	}

	return elementList
}

func (s Set) Add(element string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, exists := s.elements[element]; exists {
		return
	}

	s.elements[element] = true
	return
}

func (s Set) Delete(element string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, exists := s.elements[element]; !exists {
		return
	}

	delete(s.elements, element)
	return
}

func (s Set) Contains(element string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, exists := s.elements[element]
	return exists
}
