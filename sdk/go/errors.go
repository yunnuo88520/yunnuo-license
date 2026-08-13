package ynlicense

import (
	"errors"
	"fmt"
)

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
	RequestID  string
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("ynlicense: %s: %s (request_id=%s)", e.Code, e.Message, e.RequestID)
	}
	return fmt.Sprintf("ynlicense: %s: %s", e.Code, e.Message)
}

func IsAPIErrorCode(err error, code string) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

type VerificationError struct {
	Code    string
	Message string
}

func (e *VerificationError) Error() string {
	return fmt.Sprintf("ynlicense: offline verification %s: %s", e.Code, e.Message)
}

func IsVerificationErrorCode(err error, code string) bool {
	var verifyErr *VerificationError
	return errors.As(err, &verifyErr) && verifyErr.Code == code
}

func verificationError(code, message string) error {
	return &VerificationError{Code: code, Message: message}
}
