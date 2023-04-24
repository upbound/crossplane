/*
Copyright 2019 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package features defines Crossplane feature flags.
package features

import "github.com/crossplane/crossplane-runtime/pkg/feature"

// Feature flags.
const (
	// EnableAlphaEnvironmentConfigs enables alpha support for composition
	// environments. See the below design for more details.
	// https://github.com/crossplane/crossplane/blob/c4bcbe/design/one-pager-composition-environment.md
	EnableAlphaEnvironmentConfigs feature.Flag = "EnableAlphaEnvironmentConfigs"

	// EnableAlphaExternalSecretStores enables alpha support for
	// External Secret Stores. See the below design for more details.
	// https://github.com/crossplane/crossplane/blob/390ddd/design/design-doc-external-secret-stores.md
	EnableAlphaExternalSecretStores feature.Flag = "EnableAlphaExternalSecretStores"

	// EnableAlphaCompositionFunctions enables alpha support for composition
	// functions. See the below design for more details.
	// https://github.com/crossplane/crossplane/blob/9ee7a2/design/design-doc-composition-functions.md
	EnableAlphaCompositionFunctions feature.Flag = "EnableAlphaCompositionFunctions"

	// EnableAlphaCompositionWebhookSchemaValidation enables alpha support for
	// composition webhook schema validation. See the below design for more
	// details.
	// https://github.com/crossplane/crossplane/blob/f32496bed53a393c8239376fd8266ddf2ef84d61/design/design-doc-composition-validating-webhook.md
	EnableAlphaCompositionWebhookSchemaValidation feature.Flag = "EnableAlphaCompositionWebhookSchemaValidation"

	// EnableProviderIdentity enables alpha support for Provider identity. This
	// feature is only available when running on Upbound.
	EnableProviderIdentity feature.Flag = "EnableProviderIdentity"
)
