package gv8

import "fmt"

const (
	errMsgContextClosed = "gv8: context is closed"
	errMsgInvalidModule = "gv8: module is no longer valid"
	errMsgInvalidValue  = "gv8: value is no longer valid"
)

// JSError is the Go representation of a V8 exception.
type JSError struct {
	Message  string
	Location string
	Stack    string
}

func (e *JSError) Error() string {
	switch {
	case e == nil:
		return "<nil>"
	case e.Location == "":
		return e.Message
	default:
		return fmt.Sprintf("%s (%s)", e.Message, e.Location)
	}
}

func invalidValueError() *JSError {
	return &JSError{Message: errMsgInvalidValue}
}

func invalidModuleError() *JSError {
	return &JSError{Message: errMsgInvalidModule}
}
