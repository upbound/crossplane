/*
Copyright 2023 The Crossplane Authors.

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

package revision

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgmetav1 "github.com/crossplane/crossplane/apis/pkg/meta/v1"
)

func TestUXPRuntimeManifestBuilderDeployment(t *testing.T) {
	type args struct {
		builder            ManifestBuilder
		overrides          []DeploymentOverride
		serviceAccountName string
	}
	type want struct {
		want *appsv1.Deployment
	}
	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ProviderDeploymentWithProviderIdentity": {
			reason: "If provider identity is enabled, a proidc volume should be added.",
			args: args{
				builder: &RuntimeManifestBuilder{
					revision:         providerRevision,
					namespace:        namespace,
					providerIdentity: true,
				},
				serviceAccountName: providerRevisionName,
				overrides:          providerDeploymentOverrides(&pkgmetav1.Provider{ObjectMeta: metav1.ObjectMeta{Name: providerMetaName}}, providerRevision, providerImage),
			},
			want: want{
				want: deploymentProvider(providerName, providerRevisionName, providerImage, DeploymentWithSelectors(map[string]string{
					"pkg.crossplane.io/provider": providerMetaName,
					"pkg.crossplane.io/revision": providerRevisionName,
				}), DeploymentWithUpboundProviderIdentity()),
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := tc.args.builder.Deployment(tc.args.serviceAccountName, tc.args.overrides...)
			if diff := cmp.Diff(tc.want.want, got); diff != "" {
				t.Errorf("\n%s\nDeployment(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}
