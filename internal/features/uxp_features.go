package features

import "github.com/crossplane/crossplane-runtime/pkg/feature"

// Alpha Feature flags.
const (
	// EnableProviderIdentity enables alpha support for Provider identity. This
	// feature is only available when running on Upbound.
	EnableProviderIdentity feature.Flag = "EnableProviderIdentity"
)
