package errors

import "time"

// New creates a generic AppError with the supplied metadata.
func New(code string, category ErrorCategory, message string, err error) *AppError {
	return &AppError{
		Code:      code,
		Category:  category,
		Message:   message,
		Err:       err,
		Timestamp: time.Now(),
	}
}

// SystemError creates a SYSTEM category error instance.
func SystemError(code, message string, err error) *AppError {
	return &AppError{
		Code:      code,
		Category:  ErrCategorySystem,
		Message:   message,
		Err:       err,
		Timestamp: time.Now(),
	}
}

// NetworkError creates a NETWORK category error instance.
func NetworkError(code, message string, err error) *AppError {
	return &AppError{
		Code:        code,
		Category:    ErrCategoryNetwork,
		Message:     message,
		Err:         err,
		Recoverable: true,
		Timestamp:   time.Now(),
	}
}

// ConfigError creates a CONFIG category error instance.
func ConfigError(code, message string, err error) *AppError {
	return &AppError{
		Code:      code,
		Category:  ErrCategoryConfig,
		Message:   message,
		Err:       err,
		Timestamp: time.Now(),
	}
}

// ValidationError creates a VALIDATION category error instance.
func ValidationError(code, message string, err error) *AppError {
	return &AppError{
		Code:      code,
		Category:  ErrCategoryValidation,
		Message:   message,
		Err:       err,
		Timestamp: time.Now(),
	}
}

// DependencyError creates a DEPENDENCY category error instance.
func DependencyError(code, message string, err error) *AppError {
	return &AppError{
		Code:      code,
		Category:  ErrCategoryDependency,
		Message:   message,
		Err:       err,
		Timestamp: time.Now(),
	}
}

// FirewallError creates a FIREWALL category error instance.
func FirewallError(code, message string, err error) *AppError {
	return &AppError{
		Code:      code,
		Category:  ErrCategoryFirewall,
		Message:   message,
		Err:       err,
		Timestamp: time.Now(),
	}
}

// DeploymentError creates a DEPLOYMENT category error instance.
func DeploymentError(code, message string, err error) *AppError {
	return &AppError{
		Code:      code,
		Category:  ErrCategoryDeployment,
		Message:   message,
		Err:       err,
		Timestamp: time.Now(),
	}
}

// DatabaseError creates a DATABASE category error instance.
func DatabaseError(code, message string, err error) *AppError {
	return &AppError{
		Code:      code,
		Category:  ErrCategoryDatabase,
		Message:   message,
		Err:       err,
		Timestamp: time.Now(),
	}
}
