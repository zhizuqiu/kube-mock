# kube-mock manager

## manager

`manager` 是个 Operator，负责调协 `Nodes.mock.zhizuqiu.cn` 自定义资源，进行相应 kube-mock kubelet 的创建和删除。

```shell
$ kubemock manager -h
Reconcile Node CRs

Usage:
  kubemock manager [flags]

Flags:
      --health-probe-bind-address string   The address the probe endpoint binds to. (default ":8081")
  -h, --help                               help for manager
      --leader-elect                       Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.
      --metrics-bind-address string        The address the metric endpoint binds to. (default ":8080")
```