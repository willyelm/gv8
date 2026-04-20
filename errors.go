package gv8

import "fmt"

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
