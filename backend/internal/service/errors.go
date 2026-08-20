package service

import "net/http"

type AppError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e AppError) Error() string {
	return e.Message
}

func badRequest(code, message string) AppError {
	return AppError{Code: code, Message: message, HTTPStatus: http.StatusBadRequest}
}

func forbidden(code, message string) AppError {
	return AppError{Code: code, Message: message, HTTPStatus: http.StatusForbidden}
}

func unauthorized(code, message string) AppError {
	return AppError{Code: code, Message: message, HTTPStatus: http.StatusUnauthorized}
}

func notFound(code, message string) AppError {
	return AppError{Code: code, Message: message, HTTPStatus: http.StatusNotFound}
}

func tooManyRequests(code, message string) AppError {
	return AppError{Code: code, Message: message, HTTPStatus: http.StatusTooManyRequests}
}

var (
	ErrInvalidRequest           = badRequest("INVALID_REQUEST", "invalid request")
	ErrDatabaseConnection       = badRequest("DATABASE_CONNECTION_FAILED", "database connection failed")
	ErrDatabaseMigration        = badRequest("DATABASE_MIGRATION_FAILED", "database migration failed")
	ErrSetupDatabaseInitialized = forbidden("SETUP_DATABASE_INITIALIZED", "target database is already initialized")
	ErrInvalidAppKey            = notFound("INVALID_APP_KEY", "app key not found")
	ErrProductDisabled          = forbidden("PRODUCT_DISABLED", "product disabled")
	ErrProductNotFound          = notFound("PRODUCT_NOT_FOUND", "product not found")
	ErrCardInvalid              = notFound("CARD_INVALID", "card invalid")
	ErrCardVoided               = forbidden("CARD_VOIDED", "card voided")
	ErrCardUsed                 = forbidden("CARD_USED_BY_OTHER_BINDING", "card used by another binding")
	ErrCardNotFound             = notFound("CARD_NOT_FOUND", "card not found")
	ErrCardBatchNotFound        = notFound("CARD_BATCH_NOT_FOUND", "card batch not found")
	ErrCardCannotVoid           = forbidden("CARD_CANNOT_VOID", "only unused cards can be voided")
	ErrLicenseNotFound          = notFound("LICENSE_NOT_FOUND", "license not found")
	ErrAuthorizationNotFound    = notFound("AUTHORIZATION_NOT_FOUND", "authorization not found")
	ErrLicenseExpired           = forbidden("LICENSE_EXPIRED", "license expired")
	ErrLicenseRevoked           = forbidden("LICENSE_REVOKED", "license revoked")
	ErrBindingRequired          = badRequest("BINDING_REQUIRED", "binding required")
	ErrBindingMismatch          = forbidden("BINDING_MISMATCH", "binding mismatch")
	ErrDeviceLimitExceeded      = forbidden("DEVICE_LIMIT_EXCEEDED", "device limit exceeded")
	ErrLicensePermanent         = forbidden("LICENSE_PERMANENT", "license is permanent")
	ErrOfflineLicenseNotFound   = notFound("OFFLINE_LICENSE_NOT_FOUND", "offline license not found")
	ErrOfflineLicenseRevoked    = forbidden("OFFLINE_LICENSE_REVOKED", "offline license revoked")
	ErrOfflineLicenseExpired    = forbidden("OFFLINE_LICENSE_EXPIRED", "offline license expired")
	ErrUnbindCooldown           = forbidden("UNBIND_COOLDOWN", "unbind cooldown not finished")
	ErrAgentNotFound            = notFound("AGENT_NOT_FOUND", "agent not found")
	ErrAgentDisabled            = forbidden("AGENT_DISABLED", "agent disabled")
	ErrAgentSuspended           = forbidden("AGENT_SUSPENDED", "agent suspended")
	ErrAgentProductDenied       = forbidden("AGENT_PRODUCT_NOT_ALLOWED", "agent cannot sell this product")
	ErrAgentDurationDenied      = forbidden("AGENT_DURATION_NOT_ALLOWED", "agent cannot generate this duration")
	ErrAgentPermanentDenied     = forbidden("AGENT_PERMANENT_NOT_ALLOWED", "agent cannot generate permanent cards")
	ErrAgentBatchExceeded       = forbidden("AGENT_BATCH_LIMIT_EXCEEDED", "agent batch limit exceeded")
	ErrAgentQuotaNotEnough      = forbidden("AGENT_QUOTA_NOT_ENOUGH", "agent quota not enough")
	ErrAgentBatchNotFound       = notFound("AGENT_BATCH_NOT_FOUND", "agent card batch not found")
	ErrAgentExportDenied        = forbidden("AGENT_EXPORT_NOT_ALLOWED", "agent cannot export plaintext cards")
	ErrInvalidCredentials       = unauthorized("INVALID_CREDENTIALS", "invalid credentials")
	ErrInvalidAgentToken        = unauthorized("INVALID_AGENT_TOKEN", "invalid agent token")
	ErrAgentUserDisabled        = forbidden("AGENT_USER_DISABLED", "agent user disabled")
	ErrAgentUserNotFound        = notFound("AGENT_USER_NOT_FOUND", "agent user not found")
	ErrInvalidAdminToken        = unauthorized("INVALID_ADMIN_TOKEN", "invalid admin token")
	ErrAdminUserDisabled        = forbidden("ADMIN_USER_DISABLED", "admin user disabled")
	ErrPermissionDenied         = forbidden("PERMISSION_DENIED", "permission denied")
	ErrRateLimited              = tooManyRequests("RATE_LIMITED", "too many requests")
	ErrRiskBlockedIP            = forbidden("RISK_IP_BLOCKED", "this IP address is blocked")
	ErrRiskBlockedDevice        = forbidden("RISK_DEVICE_BLOCKED", "this device is blocked")
	ErrRiskBlockNotFound        = notFound("RISK_BLOCK_NOT_FOUND", "risk block not found")
	ErrRiskAlertNotFound        = notFound("RISK_ALERT_NOT_FOUND", "risk alert not found")
)
