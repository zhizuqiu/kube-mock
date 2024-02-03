# kube-mock node

## node

`node` 伪装成 Kubelet，并模拟 pod 运行状态的变化。

```shell
$ kubemock node -h   
Implements a node on a Kubernetes cluster to mock the running of pods

Usage:
  kubemock node [flags]

Flags:
      --authentication-token-webhook                            Use the TokenReview API to determine authentication for bearer tokens.
      --authentication-token-webhook-cache-ttl duration         The duration to cache responses from the webhook token authenticator.
      --authorization-webhook-cache-authorized-ttl duration     The duration to cache 'authorized' responses from the webhook authorizer.
      --authorization-webhook-cache-unauthorized-ttl duration   The duration to cache 'unauthorized' responses from the webhook authorizer.
      --client-verify-ca string                                 CA cert to use to verify client requests
      --cluster-domain string                                   kubernetes cluster-domain (default "cluster.local")
      --cpu string                                              kubernetes node cpu (default "20")
      --disable-taint                                           disable the node taint (default true)
      --full-resync-period duration                             how often to perform a full resync of pods between kubernetes and the provider
  -h, --help                                                    help for node
      --klog.add_dir_header                                     If true, adds the file directory to the header of the log messages
      --klog.alsologtostderr                                    log to standard error as well as files (no effect when -logtostderr=true)
      --klog.log_backtrace_at traceLocation                     when logging hits line file:N, emit a stack trace (default :0)
      --klog.log_dir string                                     If non-empty, write log files in this directory (no effect when -logtostderr=true)
      --klog.log_file string                                    If non-empty, use this log file (no effect when -logtostderr=true)
      --klog.log_file_max_size uint                             Defines the maximum size a log file can grow to (no effect when -logtostderr=true). Unit is megabytes. If the value is 0, the maximum file size is unlimited. (default 1800)
      --klog.logtostderr                                        log to standard error instead of files (default true)
      --klog.one_output                                         If true, only write logs to their native severity level (vs also writing to each lower severity level; no effect when -logtostderr=true)
      --klog.skip_headers                                       If true, avoid header prefixes in the log messages
      --klog.skip_log_headers                                   If true, avoid headers when opening log files (no effect when -logtostderr=true)
      --klog.stderrthreshold severity                           logs at or above this threshold go to stderr when writing to files and stderr (no effect when -logtostderr=true or -alsologtostderr=true) (default 2)
      --klog.v Level                                            number for the log level verbosity
      --klog.vmodule moduleSpec                                 comma-separated list of pattern=N settings for file-filtered logging
      --labels string                                           kubernetes node labels (key1=value1,key2=value2) (default "vk-kube-mock")
      --log-level string                                        log level. (default "info")
      --memory string                                           kubernetes node memory (default "100Gi")
      --no-verify-clients                                       Do not require client certificate validation
      --nodename string                                         kubernetes node name (default "vk-kube-mock")
      --os string                                               Operating System (Linux/Windows) (default "Linux")
      --pod-sync-workers int                                    set the number of pod synchronization workers (default 50)
      --pods string                                             kubernetes node podCapacity (default "20")
      --startup-timeout duration                                How long to wait for the virtual-kubelet to start (default 20s)
      --status-updates-interval duration                        status updates interval. Default is 5s. (default 5s)
      --taints string                                           kubernetes node taints (json) (default "vk-kube-mock")
```