package tide

import "fmt"

// NoaaAPIError represents an error from the NOAA API
type NoaaAPIError struct {
	Message string
	Err     error
}

func (e *NoaaAPIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("NOAA API error: %s: %v", e.Message, e.Err)
	}
	return fmt.Sprintf("NOAA API error: %s", e.Message)
}

func (e *NoaaAPIError) Unwrap() error {
	return e.Err
}

// NewNoaaAPIError creates a new NOAA API error
func NewNoaaAPIError(message string, err error) *NoaaAPIError {
	return &NoaaAPIError{
		Message: message,
		Err:     err,
	}
}

// Error when user requests data for too much data
type InvalidRangeError struct {
	Message string
}

func (e *InvalidRangeError) Error() string {
	return e.Message
}

func NewInvalidRangeError(message string) *InvalidRangeError {
	return &InvalidRangeError{
		Message: message,
	}
}
