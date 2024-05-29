/*
Copyright 2024 The Crossplane Authors.

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

package pipelinecomposition

import (
	"errors"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	commonv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"

	v1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
)

const (
	defaultFunctionRefName = "function-patch-and-transform"
	errNilComposition      = "provided Composition is empty"
)

// convertPnTToPipeline takes a patch-and-transform composition and returns
// a composition where the built-in patch & transform has been moved to a
// function. If the existing composition has PipelineMode enabled, it will
// not change anything.
func convertPnTToPipeline(c *v1.Composition, functionRefName string) (*v1.Composition, error) {
	if c == nil {
		return nil, errors.New(errNilComposition)
	}

	// If Composition is already set to run in a Pipeline, return immediately
	if c.Spec.Mode != nil && *c.Spec.Mode == v1.CompositionModePipeline {
		return c, nil
	}

	// prevent null timestamps in the output. k8s apply ignores this field
	if c.ObjectMeta.CreationTimestamp.IsZero() {
		c.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now())
	}

	cp := &v1.Composition{
		TypeMeta:   c.TypeMeta,
		ObjectMeta: c.ObjectMeta,
		Spec: v1.CompositionSpec{
			CompositeTypeRef:                           c.Spec.CompositeTypeRef,
			WriteConnectionSecretsToNamespace:          c.Spec.DeepCopy().WriteConnectionSecretsToNamespace,
			PublishConnectionDetailsWithStoreConfigRef: c.Spec.PublishConnectionDetailsWithStoreConfigRef.DeepCopy(),
		},
	}

	// Migrate existing input
	input := &Input{
		PatchSets: []v1.PatchSet{},
		Resources: []v1.ComposedTemplate{},
	}

	// Most EnvironmentConfig settings remain at the Composition Level, but
	// Environment Patches are handled at the Function level
	if c.Spec.Environment != nil {
		cp.Spec.Environment = &v1.EnvironmentConfiguration{
			DefaultData:        c.Spec.Environment.DefaultData,
			EnvironmentConfigs: c.Spec.Environment.EnvironmentConfigs,
			Policy:             c.Spec.Environment.Policy,
		}
		if len(c.Spec.Environment.Patches) > 0 {
			input.Environment = &v1.EnvironmentConfiguration{
				Patches: c.Spec.Environment.Patches,
			}
		}
	}

	if len(c.Spec.PatchSets) > 0 {
		input.PatchSets = c.Spec.PatchSets
	}
	if len(c.Spec.Resources) > 0 {
		input.Resources = c.Spec.Resources
	}

	// Override function name if provided
	fr := v1.FunctionReference{Name: defaultFunctionRefName}
	if functionRefName != "" {
		fr.Name = functionRefName
	}

	// Set up the pipeline
	pipelineMode := v1.CompositionModePipeline
	cp.Spec.Mode = &pipelineMode

	cp.Spec.Pipeline = []v1.PipelineStep{
		{
			Step:        "patch-and-transform",
			FunctionRef: fr,
			Input:       processFunctionInput(input),
		},
	}
	return cp, nil
}

// processFunctionInput populates any missing fields in the input to the function
// that are required by the function but were optional in the built-in engine.
func processFunctionInput(input *Input) *runtime.RawExtension {
	processedInput := &Input{}

	// process Environment Patches
	if input.Environment != nil && len(input.Environment.Patches) > 0 {
		processedEnvPatches := []v1.EnvironmentPatch{}
		for _, envPatch := range input.Environment.Patches {
			processedEnvPatches = append(processedEnvPatches, setMissingEnvironmentPatchFields(envPatch))
		}
		processedInput.Environment = &v1.EnvironmentConfiguration{
			Patches: processedEnvPatches,
		}
	}

	// process PatchSets
	processedPatchSet := []v1.PatchSet{}
	for _, patchSet := range input.PatchSets {
		processedPatchSet = append(processedPatchSet, setMissingPatchSetFields(patchSet))
	}
	processedInput.PatchSets = processedPatchSet

	// process Resources
	processedResources := []v1.ComposedTemplate{}
	for idx, resource := range input.Resources {
		processedResources = append(processedResources, setMissingResourceFields(idx, resource))
	}
	processedInput.Resources = processedResources

	// Wrap the input in a RawExtension
	inputType := map[string]any{
		"apiVersion":  "pt.fn.crossplane.io/v1beta1",
		"kind":        "Resources",
		"environment": MigratePatchPolicyInEnvironment(processedInput.Environment.DeepCopy()),
		"patchSets":   MigratePatchPolicyInPatchSets(processedInput.PatchSets),
		"resources":   MigratePatchPolicyInResources(processedInput.Resources),
	}

	return &runtime.RawExtension{
		Object: &unstructured.Unstructured{Object: inputType},
	}
}

// MigratePatchPolicyInResources processes all the patches in the given resources to migrate their patch policies.
func MigratePatchPolicyInResources(resources []v1.ComposedTemplate) []ComposedTemplate {
	composedTemplates := []ComposedTemplate{}

	for _, resource := range resources {
		composedTemplate := ComposedTemplate{}
		composedTemplate.ComposedTemplate = resource
		composedTemplate.Patches = migratePatches(resource.Patches)
		// Conversion function above overrides the patches in the new type,
		// so after conversion we set the underlying patches to nil to make sure
		// there's no conflict in the serialized output.
		composedTemplate.ComposedTemplate.Patches = nil
		composedTemplates = append(composedTemplates, composedTemplate)
	}
	return composedTemplates
}

// MigratePatchPolicyInPatchSets processes all the patches in the given patch set to migrate their patch policies.
func MigratePatchPolicyInPatchSets(patchset []v1.PatchSet) []PatchSet {
	newPatchSets := []PatchSet{}

	for _, patchSet := range patchset {
		newpatchset := PatchSet{}
		newpatchset.Name = patchSet.Name
		newpatchset.Patches = migratePatches(patchSet.Patches)

		newPatchSets = append(newPatchSets, newpatchset)
	}

	return newPatchSets
}

// MigratePatchPolicyInEnvironment processes all the patches in the given
// environment configuration to migrate their patch policies.
func MigratePatchPolicyInEnvironment(ec *v1.EnvironmentConfiguration) *Environment {
	if ec == nil || len(ec.Patches) == 0 {
		return nil
	}

	return &Environment{
		Patches: migrateEnvPatches(ec.Patches),
	}
}

func migratePatches(patches []v1.Patch) []Patch {
	newPatches := []Patch{}

	for _, patch := range patches {
		newpatch := Patch{}
		newpatch.Patch = patch

		if patch.Policy != nil {
			newpatch.Policy = migratePatchPolicy(patch.Policy)
			// Conversion function above overrides the patch policy in the new type,
			// so after conversion we set underlying policy to nil to make sure
			// there's no conflict in the serialized output.
			newpatch.Patch.Policy = nil
		}

		newPatches = append(newPatches, newpatch)
	}

	return newPatches
}

func migrateEnvPatches(envPatches []v1.EnvironmentPatch) []EnvironmentPatch {
	newEnvPatches := []EnvironmentPatch{}

	for _, envPatch := range envPatches {
		newEnvPatch := EnvironmentPatch{}
		newEnvPatch.EnvironmentPatch = envPatch

		if envPatch.Policy != nil {
			newEnvPatch.Policy = migratePatchPolicy(envPatch.Policy)
			// Conversion function above overrides the patch policy in the new type,
			// so after conversion we set underlying policy to nil to make sure
			// there's no conflict in the serialized output.
			newEnvPatch.EnvironmentPatch.Policy = nil
		}

		newEnvPatches = append(newEnvPatches, newEnvPatch)
	}

	return newEnvPatches
}

func migratePatchPolicy(policy *v1.PatchPolicy) *PatchPolicy {
	to := migrateMergeOptions(policy.MergeOptions)

	if to == nil && policy.FromFieldPath == nil {
		// neither To nor From has been set, just return nil to use defaults for
		// everything
		return nil
	}

	return &PatchPolicy{
		FromFieldPath: policy.FromFieldPath,
		ToFieldPath:   to,
	}
}

// migrateMergeOptions implements the conversion of mergeOptions to the new
// toFieldPath policy. The conversion logic is described in
// https://github.com/crossplane-contrib/function-patch-and-transform/?tab=readme-ov-file#mergeoptions-replaced-by-tofieldpath.
func migrateMergeOptions(mo *commonv1.MergeOptions) *ToFieldPathPolicy {
	if mo == nil {
		// No merge options at all, default to nil which will mean Replace
		return nil
	}

	if isTrue(mo.KeepMapValues) {
		if isNilOrFalse(mo.AppendSlice) {
			// { appendSlice: nil/false, keepMapValues: true}
			return ptr.To(ToFieldPathPolicyMergeObjects)
		}

		// { appendSlice: true, keepMapValues: true }
		return ptr.To(ToFieldPathPolicyMergeObjectsAppendArrays)
	}

	if isTrue(mo.AppendSlice) {
		// { appendSlice: true, keepMapValues: nil/false }
		return ptr.To(ToFieldPathPolicyForceMergeObjectsAppendArrays)
	}

	// { appendSlice: nil/false, keepMapValues: nil/false }
	return ptr.To(ToFieldPathPolicyForceMergeObjects)
}

func isNilOrFalse(b *bool) bool {
	return b == nil || !*b
}

func isTrue(b *bool) bool {
	return b != nil && *b
}

func setMissingPatchSetFields(patchSet v1.PatchSet) v1.PatchSet {
	p := []v1.Patch{}
	for _, patch := range patchSet.Patches {
		p = append(p, setMissingPatchFields(patch))
	}
	patchSet.Patches = p
	return patchSet
}

func setMissingEnvironmentPatchFields(patch v1.EnvironmentPatch) v1.EnvironmentPatch {
	if patch.Type == "" {
		patch.Type = v1.PatchTypeFromCompositeFieldPath
	}
	if len(patch.Transforms) == 0 {
		return patch
	}
	t := []v1.Transform{}
	for _, transform := range patch.Transforms {
		t = append(t, setTransformTypeRequiredFields(transform))
	}
	patch.Transforms = t
	return patch
}

func setMissingPatchFields(patch v1.Patch) v1.Patch {
	if patch.Type == "" {
		patch.Type = v1.PatchTypeFromCompositeFieldPath
	}
	if len(patch.Transforms) == 0 {
		return patch
	}
	t := []v1.Transform{}
	for _, transform := range patch.Transforms {
		t = append(t, setTransformTypeRequiredFields(transform))
	}
	patch.Transforms = t
	return patch
}

func setMissingResourceFields(idx int, rs v1.ComposedTemplate) v1.ComposedTemplate {
	if rs.Name == nil || *rs.Name == "" {
		rs.Name = ptr.To(strings.ToLower(fmt.Sprintf("resource-%d", idx)))
	}

	cd := []v1.ConnectionDetail{}
	for _, detail := range rs.ConnectionDetails {
		cd = append(cd, setMissingConnectionDetailFields(detail))
	}
	rs.ConnectionDetails = cd

	patches := []v1.Patch{}
	for _, patch := range rs.Patches {
		patches = append(patches, setMissingPatchFields(patch))
	}
	rs.Patches = patches
	return rs
}

// setTransformTypeRequiredFields sets fields that are required with
// function-patch-and-transform but were optional with the built-in engine.
func setTransformTypeRequiredFields(tt v1.Transform) v1.Transform {
	if tt.Type == "" {
		if tt.Math != nil {
			tt.Type = v1.TransformTypeMath
		}
		if tt.String != nil {
			tt.Type = v1.TransformTypeString
		}
	}
	if tt.Type == v1.TransformTypeMath && tt.Math.Type == "" {
		tt.Math.Type = getMathTransformType(tt)
	}

	if tt.Type == v1.TransformTypeString && tt.String.Type == "" {
		tt.String.Type = getStringTransformType(tt)
	}
	return tt
}

func getMathTransformType(tt v1.Transform) v1.MathTransformType {
	switch {
	case tt.Math.Type != "":
		return tt.Math.Type
	case tt.Math.ClampMin != nil:
		return v1.MathTransformTypeClampMin
	case tt.Math.ClampMax != nil:
		return v1.MathTransformTypeClampMax
	case tt.Math.Multiply != nil:
		return v1.MathTransformTypeMultiply
	}
	return ""
}

func getStringTransformType(tt v1.Transform) v1.StringTransformType {
	switch {
	case tt.String.Type != "":
		return tt.String.Type
	case tt.String.Format != nil:
		return v1.StringTransformTypeFormat
	case tt.String.Convert != nil:
		return v1.StringTransformTypeConvert
	case tt.String.Regexp != nil:
		return v1.StringTransformTypeRegexp
	}
	return ""
}

func setMissingConnectionDetailFields(sk v1.ConnectionDetail) v1.ConnectionDetail {
	// Only one of the values should be set, but we are not validating it here
	nsk := v1.ConnectionDetail{
		Name:                    sk.Name,
		Value:                   sk.Value,
		FromConnectionSecretKey: sk.FromConnectionSecretKey,
		FromFieldPath:           sk.FromFieldPath,
	}
	// Type is now required
	if nsk.Type == nil {
		switch {
		case sk.Value != nil:
			nsk.Type = ptr.To(v1.ConnectionDetailTypeFromValue)
		case sk.FromFieldPath != nil:
			nsk.Type = ptr.To(v1.ConnectionDetailTypeFromFieldPath)
		case sk.FromConnectionSecretKey != nil:
			nsk.Type = ptr.To(v1.ConnectionDetailTypeFromConnectionSecretKey)
		}
	}
	// Name is also required
	if nsk.Name == nil {
		switch { //nolint:gocritic // we could add more here in the future
		case ptr.Equal(nsk.Type, ptr.To(v1.ConnectionDetailTypeFromConnectionSecretKey)):
			nsk.Name = sk.FromConnectionSecretKey
		}
		// FromValue and FromFieldPath should have a name, skip implementation for now
	}
	return nsk
}
