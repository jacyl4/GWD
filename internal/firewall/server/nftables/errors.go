package nftables

import apperrors "GWD/internal/errors"

func firewallError(operation, message string, err error, metadata apperrors.Metadata) *apperrors.AppError {
	return apperrors.New(apperrors.ErrCategoryFirewall, apperrors.CodeFirewallGeneric, message, err).
		WithModule("firewall.nftables").
		WithOperation(operation).
		WithFields(metadata)
}
