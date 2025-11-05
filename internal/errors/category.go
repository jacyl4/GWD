package errors

// ErrorCategory groups related application errors for unified handling.
type ErrorCategory string

const (
	ErrCategorySystem     ErrorCategory = "SYSTEM"
	ErrCategoryNetwork    ErrorCategory = "NETWORK"
	ErrCategoryConfig     ErrorCategory = "CONFIG"
	ErrCategoryValidation ErrorCategory = "VALIDATION"
	ErrCategoryDependency ErrorCategory = "DEPENDENCY"
	ErrCategoryFirewall   ErrorCategory = "FIREWALL"
	ErrCategoryDeployment ErrorCategory = "DEPLOYMENT"
	ErrCategoryDatabase   ErrorCategory = "DATABASE"
)
