## 相关命令

```shell
# 查询所有的 kube-mock 节点
kubectl get node -l type=virtual-kubelet

# 清理所有的 kube-mock 节点（通常情况下，在CR删除前，Operator 会自动删除相应的节点，但是在一下情况下，会导致清理失败）
kubectl delete node -l type=virtual-kubelet
```