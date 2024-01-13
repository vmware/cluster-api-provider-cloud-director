/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"strings"
)

// Str2Bool returns true if the string value is not false
func Str2Bool(val string) bool {
	return strings.ToLower(val) == "true"
}

// Keys takes a map as an input and returns a slice of keys of that map.
func Keys[M ~map[key]val, key comparable, val any](m M) []key {
	r := make([]key, len(m))
	idx := 0
	for k := range m {
		r[idx] = k
		idx = idx + 1
	}

	return r
}
