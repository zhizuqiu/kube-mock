package util

import (
	"fmt"
	"github.com/zhizuqiu/kube-mock/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
)

var (
	appNameLabelKey      = "app.kubernetes.io/name"
	appComponentLabelKey = "app.kubernetes.io/component"
	appPartOfLabelKey    = "app.kubernetes.io/part-of"

	appLabel = "node"

	nodeContainerName = "kube-mock"

	cpu                   = "20"
	memory                = "100Gi"
	pods                  = "200"
	startupTimeout        = "10s"
	statusUpdatesInterval = "1s"
	logLevel              = "debug"
	KLogV                 = "2"
	address               = "192.168.1.1"
	port                  = "10250"
	kubeconfigPath        = "/app/conf/config"

	volumeNameKubeconfig    = "volume-kubeconfig"
	volumeNameApiserverCert = "volume-apiserver-cert"
	volumeNameApiserverKey  = "volume-apiserver-key"

	NodeFinalizer = "kueb-mock.zhizuqiu.github.com/finalizer"
)

func GetNodeLabels(node *v1alpha1.Node) map[string]string {
	return GenerateSelectorLabels(rootName, node)
}

func GenerateSelectorLabels(component string, node *v1alpha1.Node) map[string]string {
	return MergeLabels(map[string]string{
		appNameLabelKey:      node.Name,
		appComponentLabelKey: component,
		appPartOfLabelKey:    appLabel,
	})
}

func CreateNodePodObj(node *v1alpha1.Node, ownerRefs []metav1.OwnerReference) *v1.Pod {
	prefix := GetPodsPrefix(node.Name)
	namespace := node.Namespace
	labels := GetNodeLabels(node)
	volumes := createNodeVolumes(node)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
			GenerateName:    prefix,
		},
		Spec: v1.PodSpec{
			Affinity:         node.Spec.Affinity,
			Tolerations:      node.Spec.Tolerations,
			NodeSelector:     node.Spec.NodeSelector,
			SecurityContext:  createSecurityContext(node.Spec.SecurityContext),
			ImagePullSecrets: node.Spec.ImagePullSecrets,
			Volumes:          volumes,
			Containers:       []v1.Container{createContainer(node)},
		},
	}

	return pod
}

func createSecurityContext(secctx *v1.PodSecurityContext) *v1.PodSecurityContext {
	if secctx != nil {
		return secctx
	}

	defaultUserAndGroup := int64(1000)
	runAsNonRoot := true

	return &v1.PodSecurityContext{
		RunAsUser:    &defaultUserAndGroup,
		RunAsGroup:   &defaultUserAndGroup,
		RunAsNonRoot: &runAsNonRoot,
		FSGroup:      &defaultUserAndGroup,
	}
}

// equal or a contain b
func containerEqual(aContainer v1.Container, bContainer v1.Container) bool {
	if !reflect.DeepEqual(aContainer.Name, bContainer.Name) {
		return false
	}
	if !reflect.DeepEqual(aContainer.Image, bContainer.Image) {
		return false
	}
	if !reflect.DeepEqual(aContainer.ImagePullPolicy, bContainer.ImagePullPolicy) {
		return false
	}
	if !reflect.DeepEqual(aContainer.Command, bContainer.Command) {
		return false
	}
	if !reflect.DeepEqual(aContainer.Resources, bContainer.Resources) {
		return false
	}
	if !reflect.DeepEqual(aContainer.Env, bContainer.Env) {
		return false
	}
	if !containsVolumeMounts(aContainer.VolumeMounts, bContainer.VolumeMounts) {
		return false
	}
	return true
}

func createContainer(node *v1alpha1.Node) v1.Container {
	return v1.Container{
		Name:            nodeContainerName,
		Image:           node.Spec.Image,
		ImagePullPolicy: pullPolicy(node.Spec.ImagePullPolicy),
		Command:         createNodeCommand(node),
		Resources:       node.Spec.Resources,
		Env:             createNodeEnv(node),
		VolumeMounts:    createNodeVolumeMounts(node),
	}
}

func getContainer(pod v1.Pod) v1.Container {
	container := pod.Spec.Containers[0]
	return v1.Container{
		Name:            container.Name,
		Image:           container.Image,
		ImagePullPolicy: pullPolicy(container.ImagePullPolicy),
		Command:         container.Command,
		Resources:       container.Resources,
		Env:             container.Env,
		VolumeMounts:    container.VolumeMounts,
	}
}

func pullPolicy(specPolicy v1.PullPolicy) v1.PullPolicy {
	if specPolicy == "" {
		return v1.PullIfNotPresent
	}
	return specPolicy
}

func createNodeCommand(node *v1alpha1.Node) []string {
	return []string{
		"/app/virtual-kubelet",
		fmt.Sprintf("--nodename"),
		fmt.Sprintf(node.Name),
		fmt.Sprintf("--cpu"),
		fmt.Sprintf(defaultString(node.Spec.NodeConfig.Cpu, cpu)),
		fmt.Sprintf("--memory"),
		fmt.Sprintf(defaultString(node.Spec.NodeConfig.Memory, memory)),
		fmt.Sprintf("--pods"),
		fmt.Sprintf(defaultString(node.Spec.NodeConfig.Pods, pods)),
		fmt.Sprintf("--startup-timeout"),
		fmt.Sprintf(defaultString(node.Spec.NodeConfig.StartupTimeout, startupTimeout)),
		fmt.Sprintf("--status-updates-interval"),
		fmt.Sprintf(defaultString(node.Spec.StatusUpdatesInterval, statusUpdatesInterval)),
		fmt.Sprintf("--log-level"),
		fmt.Sprintf(defaultString(node.Spec.NodeConfig.LogLevel, logLevel)),
		fmt.Sprintf("--klog.v"),
		fmt.Sprintf(defaultString(node.Spec.NodeConfig.KLogV, KLogV)),
	}
}

func createNodeEnv(node *v1alpha1.Node) []v1.EnvVar {
	env := []v1.EnvVar{
		{
			Name:  "VKUBELET_POD_IP",
			Value: defaultString(node.Spec.NodeConfig.Address, address),
		},
		{
			Name:  "KUBELET_PORT",
			Value: defaultString(node.Spec.NodeConfig.Pods, port),
		},
	}
	if node.Spec.KubeconfigSecret != "" {
		env = append(env, v1.EnvVar{
			Name:  "KUBECONFIG",
			Value: kubeconfigPath,
		})
	}
	return env
}

func createNodeVolumes(node *v1alpha1.Node) []v1.Volume {
	var volumes []v1.Volume
	if node.Spec.KubeconfigSecret != "" || node.Spec.ApiserverCertSecret != "" || node.Spec.ApiserverKeySecret != "" {
		volumes = append(volumes, createVolume(volumeNameKubeconfig, node.Spec.KubeconfigSecret, "config"))
	}
	if node.Spec.ApiserverCertSecret != "" {
		volumes = append(volumes, createVolume(volumeNameApiserverCert, node.Spec.ApiserverCertSecret, "crt.pem"))
	}
	if node.Spec.ApiserverKeySecret != "" {
		volumes = append(volumes, createVolume(volumeNameApiserverKey, node.Spec.ApiserverKeySecret, "key.pem"))
	}
	return volumes
}

func createVolume(name, secretName, item string) v1.Volume {
	return v1.Volume{
		Name: name,
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: secretName,
				Items: []v1.KeyToPath{
					{
						Key:  item,
						Path: item,
					},
				},
				DefaultMode: int32P(420),
			},
		},
	}
}

func int32P(i int32) *int32 {
	return &i
}

func createNodeVolumeMounts(node *v1alpha1.Node) []v1.VolumeMount {
	var volumeMounts []v1.VolumeMount
	if node.Spec.KubeconfigSecret != "" || node.Spec.ApiserverCertSecret != "" || node.Spec.ApiserverKeySecret != "" {
		volumeMounts = append(volumeMounts, createVolumeMounts(volumeNameKubeconfig, "/app/conf/config", "config"))
	}
	if node.Spec.ApiserverCertSecret != "" {
		volumeMounts = append(volumeMounts, createVolumeMounts(volumeNameApiserverCert, "/app/conf/crt.pem", "crt.pem"))
	}
	if node.Spec.ApiserverKeySecret != "" {
		volumeMounts = append(volumeMounts, createVolumeMounts(volumeNameApiserverKey, "/app/conf/key.pem", "key.pem"))
	}
	return volumeMounts
}

func createVolumeMounts(name, mountPath, subPath string) v1.VolumeMount {
	return v1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
		SubPath:   subPath,
	}
}

func IsDesired(node *v1alpha1.Node, pod v1.Pod) bool {
	namespace := node.Namespace
	labels := GetNodeLabels(node)
	if pod.Namespace != namespace {
		return false
	}
	if !reflect.DeepEqual(pod.Labels, labels) {
		return false
	}
	if !reflect.DeepEqual(pod.Spec.Affinity, node.Spec.Affinity) {
		return false
	}
	if !containsTolerations(pod.Spec.Tolerations, node.Spec.Tolerations) {
		return false
	}

	if !reflect.DeepEqual(pod.Spec.NodeSelector, node.Spec.NodeSelector) {
		return false
	}
	if !reflect.DeepEqual(pod.Spec.SecurityContext, createSecurityContext(node.Spec.SecurityContext)) {
		return false
	}
	if !reflect.DeepEqual(pod.Spec.ImagePullSecrets, node.Spec.ImagePullSecrets) {
		return false
	}
	if !containsVolumes(pod.Spec.Volumes, createNodeVolumes(node)) {
		return false
	}
	if !containerEqual(getContainer(pod), createContainer(node)) {
		return false
	}
	return true
}

func containsTolerations(all []v1.Toleration, item []v1.Toleration) bool {
	for _, toleration := range item {
		in := false
		for _, v := range all {
			if reflect.DeepEqual(toleration, v) {
				in = true
			}
		}
		if !in {
			return false
		}
	}
	return true
}

func containsVolumes(all []v1.Volume, item []v1.Volume) bool {
	for _, volume := range item {
		in := false
		for _, v := range all {
			if reflect.DeepEqual(volume, v) {
				in = true
			}
		}
		if !in {
			return false
		}
	}
	return true
}

func containsVolumeMounts(all []v1.VolumeMount, item []v1.VolumeMount) bool {
	for _, volumeMount := range item {
		in := false
		for _, v := range all {
			if reflect.DeepEqual(volumeMount, v) {
				in = true
			}
		}
		if !in {
			return false
		}
	}
	return true
}
