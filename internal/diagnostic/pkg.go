package diagnostic

import "errors"

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type Code string

type Position struct {
	Line   int
	Column int
}

type Range struct {
	Start Position
	End   Position
}

type Fields struct {
	Name     string
	Field    string
	Schema   string
	Expected string
	Actual   string
	Index    string
	Level    int
	Operator string
	Path     string
	Details  map[string]string
}

type Error struct {
	Kind     string
	Code     Code
	Message  string
	Range    Range
	Severity Severity
	Fields   Fields
	Cause    error
}

func (err Error) Error() string {
	return err.Message
}

func (err Error) Unwrap() error {
	return err.Cause
}

func As(err error) (Error, bool) {
	var diagnosticError Error
	if !errors.As(err, &diagnosticError) {
		return Error{}, false
	}

	return diagnosticError, true
}
