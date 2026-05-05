package inbound

import "errors"

var (
	// ErrRunNotFound means the requested run does not exist.
	ErrRunNotFound = errors.New("run not found")
	// ErrArtifactNotFound means the requested run artifact does not exist.
	ErrArtifactNotFound = errors.New("artifact not found")
	// ErrInvalidInput means a use case command or query failed validation.
	ErrInvalidInput = errors.New("invalid input")
	// ErrConflict means the requested operation conflicts with current state.
	ErrConflict = errors.New("state conflict")
	// ErrApprovalNotImplemented is returned until approval modeling and an
	// inbound approval route exist.
	ErrApprovalNotImplemented = errors.New("approval use case is not implemented yet")
)

type useCaseError struct {
	kind error
	err  error
}

func (e useCaseError) Error() string {
	return e.err.Error()
}

func (e useCaseError) Is(target error) bool {
	return target == e.kind
}

func (e useCaseError) Unwrap() error {
	return e.err
}

// NewInvalidInputError marks err as a use case input error.
func NewInvalidInputError(err error) error {
	return newUseCaseError(ErrInvalidInput, err)
}

// NewConflictError marks err as a use case state conflict.
func NewConflictError(err error) error {
	return newUseCaseError(ErrConflict, err)
}

func newUseCaseError(kind error, err error) error {
	if err == nil {
		return kind
	}

	return useCaseError{
		kind: kind,
		err:  err,
	}
}
