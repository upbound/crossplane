package revision

import (
	"github.com/aws/smithy-go/ptr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// DeploymentWithUpboundProviderIdentity mounts the Upbound Provider Identity
// CSI driver as a volume to the runtime container of a Deployment.
func DeploymentWithUpboundProviderIdentity() DeploymentOverride {
	proidcVolumeName := "proidc"
	proidcDriverName := "proidc.csi.upbound.io"
	proidcMountPath := "/var/run/secrets/upbound.io/provider"

	return func(d *appsv1.Deployment) {
		d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: proidcVolumeName,
			VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{
					Driver:   proidcDriverName,
					ReadOnly: ptr.Bool(true),
				},
			},
		})
		d.Spec.Template.Spec.Containers[0].VolumeMounts = append(d.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      proidcVolumeName,
			ReadOnly:  true,
			MountPath: proidcMountPath,
		})
	}
}
