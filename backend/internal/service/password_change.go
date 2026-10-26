package service

import "strings"

var (
	ErrCurrentPasswordIncorrect = badRequest("CURRENT_PASSWORD_INCORRECT", "当前密码不正确，请重新输入")
	ErrPasswordConfirmation     = badRequest("PASSWORD_CONFIRMATION_MISMATCH", "两次输入的新密码不一致")
)

func validatePasswordChange(input ChangeAdminPasswordInput) error {
	if input.CurrentPassword == "" {
		return badRequest("CURRENT_PASSWORD_REQUIRED", "请输入当前密码")
	}
	if len(input.NewPassword) < 8 || len(input.NewPassword) > 256 || strings.TrimSpace(input.NewPassword) == "" {
		return badRequest("NEW_PASSWORD_INVALID", "新密码至少 8 位，不能超过 256 字节或全为空白")
	}
	// Confirmation is always required by the website. Existing programmatic
	// callers may omit it; a supplied confirmation must match exactly.
	if input.ConfirmPassword != nil && *input.ConfirmPassword != input.NewPassword {
		return ErrPasswordConfirmation
	}
	return nil
}
