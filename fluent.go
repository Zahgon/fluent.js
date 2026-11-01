// Package fluent holds what every package of the Project Fluent port shares: its exception type.
package fluent

// Error is a JavaScript exception of the original: Name is the constructor name and Message the
// message. Exceptions are raised as panic(*Error) and collected as error values of this type.
type Error struct {
	Name    string
	Message string
}

// Error returns the name and the message, as the original's toString does.
func (e *Error) Error() string {
	if e.Message == "" {
		return e.Name
	}
	return e.Name + ": " + e.Message
}
