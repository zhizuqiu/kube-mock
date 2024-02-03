# kube-mock tools

## mock

`mock` 命令用于读取 apiserver ，生成 kubernetes node 对应的 `Nodes.mock.zhizuziu.cn` yaml 资源文件

```shell
$ kubemock mock -h
Generate Node CRs form kubernetes

Usage:
kubemock mock --from-kubeconfig /path/to/kubeconfig.yaml /path/to/nodes.yaml

Usage:
  kubemock mock [flags]

Aliases:
  mock, mock

Flags:
      --field-selector string    Selector (field query) to filter on, supports '=', '==', and '!='.(e.g. --field-selector
                                         key1=value1,key2=value2). The server only supports a limited number of field queries per type.
  -f, --from-kubeconfig string   source kubernetes kubeconfig file path
  -h, --help                     help for mock
  -l, --selector string          Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2). Matching
                                         objects must satisfy all of the specified label constraints.
```