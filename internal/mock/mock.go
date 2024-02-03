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
	dirPerm  = 0755
	filePerm = 0666
)

func Run(ctx context.Context, fromKubeconfig string, qps float32, burst int, labelSelector, fieldSelector, path string) (retErr error) {
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

	list := createNodes(nodes.Items)

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

func createNodes(nodes []corev1.Node) []v1alpha1.Node {
	var list []v1alpha1.Node
	for _, node := range nodes {
		n := v1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: node.Name,
			},
			Spec: v1alpha1.NodeSpec{
				Image:            "zhizuqiu/kube-mock-node:v1alpha1",
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
					Cpu:     node.Status.Capacity.Cpu().String(),
					Memory:  node.Status.Capacity.Memory().String(),
					Pods:    node.Status.Capacity.Pods().String(),
					Address: getNodeAddress(node),
					Label:   node.Labels,
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
