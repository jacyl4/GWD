package nftables

import apperrors "GWD/internal/errors"

func wrapFirewallError(err error, operation, message string, metadata apperrors.Metadata) *apperrors.AppError {
	if err == nil {
		return newFirewallError(operation, message, nil, metadata)
	}

	if appErr, ok := apperrors.As(err); ok {
		if appErr.Module == "" {
			appErr.WithModule("firewall.nftables")
		}
		if appErr.Operation == "" {
			appErr.WithOperation(operation)
		}
		if metadata != nil {
			appErr.WithFields(metadata)
		}
		if appErr.Message == "" {
			appErr.Message = message
		}
		return appErr
	}

	return newFirewallError(operation, message, err, metadata)
}

func newFirewallError(operation, message string, err error, metadata apperrors.Metadata) *apperrors.AppError {
	appErr := apperrors.FirewallError(apperrors.CodeFirewallGeneric, message, err).
		WithModule("firewall.nftables").
		WithOperation(operation)
	if metadata != nil {
		appErr.WithFields(metadata)
	}
	return appErr
}
