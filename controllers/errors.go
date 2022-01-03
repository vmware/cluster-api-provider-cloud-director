package controllers

import (
	"fmt"
	"runtime/debug"
)

// NoRDEError is an error used when the InfraID value in the VCDCluster object does not point to a valid RDE in VCD
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
