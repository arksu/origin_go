package eventbus

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrShutdown     = errors.New("event bus is shutdown")
	ErrTimeout      = errors.New("handler timeout")
	ErrQueueFull    = errors.New("event queue is full")
	ErrInvalidTopic = errors.New("invalid topic")
	ErrHandlerPanic = errors.New("handler panicked")
)

type MultiError struct {
	Errors []error
}

func (e *MultiError) Error() string {
	if len(e.Errors) == 0 {
		return "no errors"
	}
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d errors occurred:\n", len(e.Errors)))
	for i, err := range e.Errors {
		sb.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, err.Error()))
	}
	return sb.String()
}

func (e *MultiError) Unwrap() []error {
	return e.Errors
}

type PanicError struct {
	Recovered any
	Stack     []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("handler panic: %v", e.Recovered)
}

func (e *PanicError) Unwrap() error {
	return ErrHandlerPanic
}

type HandlerError struct {
	Topic     string
	HandlerID string
	Err       error
}

func (e *HandlerError) Error() string {
	return fmt.Sprintf("handler %s for topic %s: %v", e.HandlerID, e.Topic, e.Err)
}

func (e *HandlerError) Unwrap() error {
	return e.Err
}
