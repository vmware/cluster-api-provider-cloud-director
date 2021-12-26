package controllers

import (
	"fmt"
	"runtime/debug"
)

type NoRDEError struct {
	msg string
}

func (nre *NoRDEError) Error() string {
	if nre == nil {
		return fmt.Sprintf("error is unexpectedly nil at stack [%s]", string(debug.Stack()))
	}
	return nre.msg
}

func NewNoRDEError(message string) *NoRDEError {
	return &NoRDEError{msg: message}
}
