package utils

import (
	"github.com/kataras/iris/v12"
)

// Stable API error codes — keep in sync with apartmentsclone/constants/apiErrorCodes.ts
const (
	ErrCodeServerInternal       = "SERVER_INTERNAL"
	ErrCodeValidationFailed     = "VALIDATION_FAILED"
	ErrCodeNotFound             = "NOT_FOUND"
	ErrCodeUnauthorized         = "AUTH_UNAUTHORIZED"
	ErrCodeInvalidCredentials   = "AUTH_INVALID_CREDENTIALS"
	ErrCodeEmailExists          = "AUTH_EMAIL_EXISTS"
	ErrCodePhoneExists          = "AUTH_PHONE_EXISTS"
	ErrCodePhoneInvalid         = "AUTH_PHONE_INVALID"
	ErrCodeSocialLogin          = "AUTH_SOCIAL_LOGIN"
	ErrCodeRegistrationFailed   = "AUTH_REGISTRATION_FAILED"

	ErrCodeUserCheckInvalidBody = "USER_CHECK_INVALID_BODY"
	ErrCodeUserCheckNoIdentifier = "USER_CHECK_NO_IDENTIFIER"
	ErrCodeUserCheckBothFields  = "USER_CHECK_BOTH_IDENTIFIERS"
	ErrCodeUserCheckFailed      = "USER_CHECK_FAILED"

	ErrCodeInvalidPayload = "INVALID_PAYLOAD"
	ErrCodeDatabaseError  = "DATABASE_ERROR"
)

// RespondAPIError writes a consistent JSON body for the mobile app and admin tools.
func RespondAPIError(ctx iris.Context, status int, code, message string, detail ...string) {
	errObj := iris.Map{
		"code":    code,
		"message": message,
		"status":  status,
	}
	if len(detail) > 0 && detail[0] != "" {
		errObj["detail"] = detail[0]
	}
	ctx.StatusCode(status)
	ctx.JSON(iris.Map{
		"ok":    false,
		"error": errObj,
	})
}

// RespondAPIErrorFromTitle maps legacy CreateError titles to stable codes.
func RespondAPIErrorFromTitle(ctx iris.Context, status int, title, detail string) {
	code := inferErrorCode(title, status)
	RespondAPIError(ctx, status, code, detail, title)
}

func inferErrorCode(title string, status int) string {
	switch title {
	case "Validation Error", "Validation error":
		return ErrCodeValidationFailed
	case "Credentials Error":
		return ErrCodeInvalidCredentials
	case "Registration Error", "Conflict":
		if status == iris.StatusConflict {
			return ErrCodeRegistrationFailed
		}
		return ErrCodeRegistrationFailed
	case "Unauthorized":
		return ErrCodeUnauthorized
	case "Not Found":
		return ErrCodeNotFound
	case "Internal Server Error":
		return ErrCodeServerInternal
	default:
		if status >= 500 {
			return ErrCodeServerInternal
		}
		if status == iris.StatusUnauthorized {
			return ErrCodeUnauthorized
		}
		if status == iris.StatusNotFound {
			return ErrCodeNotFound
		}
		return ErrCodeValidationFailed
	}
}
