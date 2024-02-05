package mock

import (
	"bytes"
	"context"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/zhizuqiu/kube-mock/api/v1alpha1"
	"github.com/zhizuqiu/kube-mock/internal/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/printers"
	"os"
	"path/filepath"
)

var (
	dirPerm          = 0755
	filePerm         = 0666
	defaultNamespace = "kube-mock-system"
)

func Run(ctx context.Context, fromKubeconfig string, qps float32, burst int, labelSelector, fieldSelector string, disableFilterMaster bool, path string) (retErr error) {
	k8sClient, err := util.GetK8sClient(fromKubeconfig, qps, burst)
	if err != nil {
		log.G(ctx).Fatal(err)
	}

	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return err
	}

	nodeList := nodes.Items
	if !disableFilterMaster {
		nodeList = filterMaster(nodeList)
	}

	list := createNodes(nodeList)

	var printer printers.YAMLPrinter
	buffer := &bytes.Buffer{}
	tabWriter := printers.GetNewTabWriter(buffer)

	for _, node := range list {
		err = printer.PrintObj(node.DeepCopyObject(), tabWriter)
		if err != nil {
			return err
		}
	}

	err = writeFile(path, buffer.Bytes())
	if err != nil {
		return err
	}

	return nil
}

func filterMaster(nodes []corev1.Node) []corev1.Node {
	var nodeList []corev1.Node
	for _, node := range nodes {
		if !hasMasterLabel(node.Labels) {
			nodeList = append(nodeList, node)
		}
	}
	return nodeList
}

func hasMasterLabel(label map[string]string) bool {
	if label == nil {
		return false
	}
	if _, ok := label["node-role.kubernetes.io/control-plane"]; ok {
		return true
	}
	if _, ok := label["node-role.kubernetes.io/control-plane"]; ok {
		return true
	}
	if _, ok := label["node-role.kubernetes.io/master"]; ok {
		return true
	}
	if value, ok := label["node"]; ok {
		if value == "master" {
			return true
		}
		return true
	}
	return false
}

func createNodes(nodes []corev1.Node) []v1alpha1.Node {
	var list []v1alpha1.Node
	for _, node := range nodes {
		n := v1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      node.Name,
				Namespace: defaultNamespace,
			},
			Spec: v1alpha1.NodeSpec{
				Image:            "zhizuqiu/kube-mock:v1alpha1",
				ImagePullPolicy:  "Always",
				KubeconfigSecret: "kube-mock-node",
				NodeSelector: map[string]string{
					"node-role.kubernetes.io/control-plane": "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      "node-role.kubernetes.io/control-plane",
						Operator: "Exists",
						Effect:   "NoSchedule",
					},
				},
				NodeConfig: v1alpha1.NodeConfig{
					Cpu:     *node.Status.Capacity.Cpu(),
					Memory:  *node.Status.Capacity.Memory(),
					Pods:    *node.Status.Capacity.Pods(),
					Address: getNodeAddress(node),
					Port:    getNodePort(node),
					Labels:  node.Labels,
					Taints:  node.Spec.Taints,
				},
			},
		}
		n.GetObjectKind().SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("Node"))
		list = append(list, n)
	}
	return list
}

func getNodeAddress(node corev1.Node) string {
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeInternalIP {
			return address.Address
		}
	}
	return ""
}

func getNodePort(node corev1.Node) int32 {
	return node.Status.DaemonEndpoints.KubeletEndpoint.Port
}

func writeFile(path string, value []byte) error {
	err := os.MkdirAll(filepath.Dir(path), os.FileMode(dirPerm))
	if err != nil {
		return err
	}

	err = os.WriteFile(path, value, os.FileMode(filePerm))
	if err != nil {
		return err
	}
	return nil
}
