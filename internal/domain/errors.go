package domain

import (
	"errors"
	"fmt"
)

// DomainError keeps backward compatibility for generic codes.
type DomainError struct {
	Code string
	Err  error
}

func (e DomainError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	if e.Code == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e DomainError) Unwrap() error {
	return e.Err
}

type NotFoundError struct {
	Resource string
	Err      error
}

func (e NotFoundError) Error() string {
	if e.Resource == "" {
		return "not found"
	}
	return fmt.Sprintf("%s not found", e.Resource)
}

func (e NotFoundError) Unwrap() error { return e.Err }

type ValidationError struct {
	Field string
	Msg   string
	Err   error
}

func (e ValidationError) Error() string {
	if e.Msg != "" && e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Msg)
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.Field != "" {
		return fmt.Sprintf("invalid %s", e.Field)
	}
	return "validation error"
}

func (e ValidationError) Unwrap() error { return e.Err }

type ConflictError struct {
	Resource string
	Msg      string
	Err      error
}

func (e ConflictError) Error() string {
	switch {
	case e.Msg != "" && e.Resource != "":
		return fmt.Sprintf("%s conflict: %s", e.Resource, e.Msg)
	case e.Msg != "":
		return e.Msg
	case e.Resource != "":
		return fmt.Sprintf("%s conflict", e.Resource)
	default:
		return "conflict"
	}
}

func (e ConflictError) Unwrap() error { return e.Err }

type InternalError struct {
	Msg string
	Err error
}

func (e InternalError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return "internal error"
}

func (e InternalError) Unwrap() error { return e.Err }

func IsNotFound(err error) bool {
	var target NotFoundError
	return errors.As(err, &target)
}

func IsValidation(err error) bool {
	var target ValidationError
	return errors.As(err, &target)
}

func IsConflict(err error) bool {
	var target ConflictError
	return errors.As(err, &target)
}

func IsInternal(err error) bool {
	var target InternalError
	return errors.As(err, &target)
}
