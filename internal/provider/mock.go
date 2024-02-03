package provider

import (
	"context"
	dto "github.com/prometheus/client_model/go"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"github.com/zhizuqiu/kube-mock/internal/provider/util"
	"io"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/record"
	"math/rand"
	"path"
	"strings"
	"time"
)

const (
	// Provider configuration defaults.
	defaultCPUCapacity    = "20"
	defaultMemoryCapacity = "100Gi"
	defaultPodCapacity    = "20"

	// Values used in tracing as attribute keys.
	namespaceKey     = "namespace"
	nameKey          = "name"
	containerNameKey = "containerName"

	statusReasonPodDeleted            = "NotFound"
	statusMessagePodDeleted           = "The pod may have been deleted from the provider"
	containerExitCodePodDeleted int32 = 0
)

// MockProvider implements the virtual-kubelet provider interface and stores podsMock in memory.
type MockProvider struct { //nolint:golint

	podsApiserver corev1listers.PodLister
	eventRecorder record.EventRecorder

	nodeName           string
	operatingSystem    string
	internalIP         string
	daemonEndpointPort int32

	podsMock       *util.SafeMap
	config         MockConfig
	startTime      time.Time
	notifier       func(*v1.Pod)
	podTracker     *PodsTracker
	mockPodTracker *MockPodsTracker

	statusUpdatesInterval time.Duration
}

// MockConfig contains a mock virtual-kubelet's configurable parameters.
type MockConfig struct { //nolint:golint
	CPU        string            `json:"cpu,omitempty"`
	Memory     string            `json:"memory,omitempty"`
	Pods       string            `json:"pods,omitempty"`
	Others     map[string]string `json:"others,omitempty"`
	ProviderID string            `json:"providerID,omitempty"`
}

// NewMockProvider creates a new MockProvider, which implements the PodNotifier interface
func NewMockProvider(ctx context.Context, config MockConfig, cfg nodeutil.ProviderConfig,
	nodeName, operatingSystem string, internalIP string, daemonEndpointPort int32,
	kubeClient kubernetes.Interface, statusUpdatesInterval time.Duration) (*MockProvider, error) {
	// set defaults
	if config.CPU == "" {
		config.CPU = defaultCPUCapacity
	}
	if config.Memory == "" {
		config.Memory = defaultMemoryCapacity
	}
	if config.Pods == "" {
		config.Pods = defaultPodCapacity
	}
	provider := MockProvider{
		nodeName:              nodeName,
		operatingSystem:       operatingSystem,
		internalIP:            internalIP,
		daemonEndpointPort:    daemonEndpointPort,
		podsMock:              util.NewSafeMap(),
		config:                config,
		startTime:             time.Now(),
		podsApiserver:         cfg.Pods,
		statusUpdatesInterval: statusUpdatesInterval,
	}

	providerEB := record.NewBroadcaster()
	providerEB.StartLogging(log.G(ctx).Infof)
	providerEB.StartRecordingToSink(&corev1client.EventSinkImpl{Interface: kubeClient.CoreV1().Events(v1.NamespaceAll)})
	provider.eventRecorder = providerEB.NewRecorder(scheme.Scheme, v1.EventSource{Component: path.Join(nodeName, "pod-controller")})

	return &provider, nil
}

// CreatePod accepts a Pod definition and stores it in memory.
// 当 pod env 中有 Downward API 时，例如存在 valueFrom.fieldRef.fieldPath: status.podIP 时
// 则，不会触发 CreatePod ，见：https://github.com/virtual-kubelet/virtual-kubelet/issues/998
func (p *MockProvider) CreatePod(ctx context.Context, pod *v1.Pod) error {
	ctx, span := trace.StartSpan(ctx, "CreatePod")
	defer span.End()

	// Add the pod's coordinates to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, pod.Namespace, nameKey, pod.Name)

	log.G(ctx).Infof("receive CreatePod %q", pod.Name)

	key, err := buildKey(pod)
	if err != nil {
		return err
	}

	p.podsMock.Write(key, pod)

	return nil
}

// UpdatePod accepts a Pod definition and updates its reference.
func (p *MockProvider) UpdatePod(ctx context.Context, pod *v1.Pod) error {
	ctx, span := trace.StartSpan(ctx, "UpdatePod")
	defer span.End()

	// Add the pod's coordinates to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, pod.Namespace, nameKey, pod.Name)

	log.G(ctx).Infof("receive UpdatePod %q", pod.Name)

	key, err := buildKey(pod)
	if err != nil {
		return err
	}

	p.podsMock.Write(key, pod)
	p.notifier(pod)

	return nil
}

// DeletePod deletes the specified pod out of memory.
func (p *MockProvider) DeletePod(ctx context.Context, pod *v1.Pod) (err error) {
	ctx, span := trace.StartSpan(ctx, "DeletePod")
	defer span.End()

	// Add the pod's coordinates to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, pod.Namespace, nameKey, pod.Name)

	log.G(ctx).Infof("receive DeletePod %q", pod.Name)

	key, err := buildKey(pod)
	if err != nil {
		return err
	}

	if _, exists := p.podsMock.Read(key); !exists {
		return errdefs.NotFound("pod not found")
	}

	p.podsMock.Delete(key)

	if p.podTracker != nil {
		// Delete is not a sync API on [provider] yet, but will assume with current implementation that termination is completed. Also, till gracePeriod is supported.
		updateErr := p.podTracker.UpdatePodStatus(ctx,
			pod.Namespace,
			pod.Name,
			func(podStatus *v1.PodStatus) {
				now := metav1.NewTime(time.Now())
				for i := range podStatus.ContainerStatuses {
					if podStatus.ContainerStatuses[i].State.Running == nil {
						continue
					}

					// todo 确认停止docker后的状态应该是什么
					podStatus.ContainerStatuses[i].State.Terminated = &v1.ContainerStateTerminated{
						ExitCode:    containerExitCodePodDeleted,
						Reason:      statusReasonPodDeleted,
						Message:     statusMessagePodDeleted,
						FinishedAt:  now,
						StartedAt:   podStatus.ContainerStatuses[i].State.Running.StartedAt,
						ContainerID: podStatus.ContainerStatuses[i].ContainerID,
					}
					podStatus.ContainerStatuses[i].State.Running = nil
				}
			},
			false,
		)

		if updateErr != nil && !errdefs.IsNotFound(updateErr) {
			log.G(ctx).WithError(updateErr).Errorf("failed to update termination status for pod %v", key)
		}
	}
	return nil
}

// GetPod returns a pod by name that is stored in memory.
func (p *MockProvider) GetPod(ctx context.Context, namespace, name string) (pod *v1.Pod, err error) {
	ctx, span := trace.StartSpan(ctx, "GetPod")
	defer func() {
		span.SetStatus(err)
		span.End()
	}()

	// Add the pod's coordinates to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, name)

	log.G(ctx).Infof("receive GetPod %q", name)

	key, err := buildKeyFromNames(namespace, name)
	if err != nil {
		return nil, err
	}

	if pod, ok := p.podsMock.Read(key); ok {
		return pod, nil
	} else {
		// 如果 podsMock 中不存在，从 apiserver 中同步一下
		if p.podTracker != nil {
			podFromApiserver := p.podTracker.getPod(namespace, name)
			if podFromApiserver != nil && podFromApiserver.DeletionTimestamp == nil {
				p.podsMock.Write(key, podFromApiserver.DeepCopy())
				return podFromApiserver, nil
			}
		}
	}
	return nil, errdefs.NotFoundf("pod \"%s/%s\" is not known to the provider", namespace, name)
}

// GetContainerLogs retrieves the logs of a container by name from the provider.
func (p *MockProvider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	ctx, span := trace.StartSpan(ctx, "GetContainerLogs")
	defer span.End()

	// Add pod and container attributes to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, podName, containerNameKey, containerName)

	log.G(ctx).Infof("receive GetContainerLogs %q", podName)
	return io.NopCloser(strings.NewReader("")), nil
}

// RunInContainer executes a command in a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
func (p *MockProvider) RunInContainer(ctx context.Context, namespace, name, container string, cmd []string, attach api.AttachIO) error {
	log.G(context.TODO()).Infof("receive ExecInContainer %q", container)
	return nil
}

// AttachToContainer attaches to the executing process of a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
func (p *MockProvider) AttachToContainer(ctx context.Context, namespace, name, container string, attach api.AttachIO) error {
	log.G(ctx).Infof("receive AttachToContainer %q", container)
	return nil
}

// PortForward forwards a local port to a port on the pod
func (p *MockProvider) PortForward(ctx context.Context, namespace, pod string, port int32, stream io.ReadWriteCloser) error {
	log.G(ctx).Infof("receive PortForward %q", pod)
	return nil
}

// GetPodStatus returns the status of a pod by name that is "running".
// returns nil if a pod by that name is not found.
func (p *MockProvider) GetPodStatus(ctx context.Context, namespace, name string) (*v1.PodStatus, error) {
	ctx, span := trace.StartSpan(ctx, "GetPodStatus")
	defer span.End()

	// Add namespace and name as attributes to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, name)

	log.G(ctx).Infof("receive GetPodStatus %q", name)

	pod, err := p.GetPod(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	return &pod.Status, nil
}

// GetPods returns a list of all podsMock known to be "running".
func (p *MockProvider) GetPods(ctx context.Context) ([]*v1.Pod, error) {
	ctx, span := trace.StartSpan(ctx, "GetPods")
	defer span.End()

	log.G(ctx).Info("receive GetPods")

	var pods []*v1.Pod

	p.podsMock.RLock()
	defer p.podsMock.RUnlock()
	for _, pod := range p.podsMock.Map {
		pods = append(pods, pod)
	}

	return pods, nil
}

func (p *MockProvider) ConfigureNode(ctx context.Context, n *v1.Node) { //nolint:golint
	ctx, span := trace.StartSpan(ctx, "mock.ConfigureNode") //nolint:staticcheck,ineffassign
	defer span.End()

	if p.config.ProviderID != "" {
		n.Spec.ProviderID = p.config.ProviderID
	}
	n.Status.Capacity = p.capacity()
	n.Status.Allocatable = p.capacity()
	n.Status.Conditions = p.nodeConditions()
	n.Status.Addresses = p.nodeAddresses()
	n.Status.DaemonEndpoints = p.nodeDaemonEndpoints()
	os := p.operatingSystem
	if os == "" {
		os = "linux"
	}
	n.Status.NodeInfo.OperatingSystem = os

	if len(n.Labels) < 1 {
		p.patchDefaultLabels(n)
	}
}

func (p *MockProvider) patchDefaultLabels(n *v1.Node) {
	// report both old and new styles of OS information
	osLabelValue := strings.ToLower(p.operatingSystem)
	n.ObjectMeta.Labels["app.kubernetes.io/created-by"] = "kube-mock"
	n.ObjectMeta.Labels["beta.kubernetes.io/os"] = osLabelValue
	n.ObjectMeta.Labels["kubernetes.io/os"] = osLabelValue
	n.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"] = "true"
	n.ObjectMeta.Labels["node.kubernetes.io/exclude-from-external-load-balancers"] = "true"
}

// GetStatsSummary returns dummy stats for all podsMock known by this provider.
func (p *MockProvider) GetStatsSummary(ctx context.Context) (*stats.Summary, error) {
	var span trace.Span
	ctx, span = trace.StartSpan(ctx, "GetStatsSummary") //nolint: ineffassign,staticcheck
	defer span.End()

	// Grab the current timestamp so we can report it as the time the stats were generated.
	time := metav1.NewTime(time.Now())

	// Create the Summary object that will later be populated with node and pod stats.
	res := &stats.Summary{}

	// Populate the Summary object with basic node stats.
	res.Node = stats.NodeStats{
		NodeName:  p.nodeName,
		StartTime: metav1.NewTime(p.startTime),
	}

	// Populate the Summary object with dummy stats for each pod known by this provider.
	p.podsMock.RLock()
	defer p.podsMock.RUnlock()
	for _, pod := range p.podsMock.Map {
		var (
			// totalUsageNanoCores will be populated with the sum of the values of UsageNanoCores computes across all containers in the pod.
			totalUsageNanoCores uint64
			// totalUsageBytes will be populated with the sum of the values of UsageBytes computed across all containers in the pod.
			totalUsageBytes uint64
		)

		// Create a PodStats object to populate with pod stats.
		pss := stats.PodStats{
			PodRef: stats.PodReference{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				UID:       string(pod.UID),
			},
			StartTime: pod.CreationTimestamp,
		}

		// Iterate over all containers in the current pod to compute dummy stats.
		for _, container := range pod.Spec.Containers {
			// Grab a dummy value to be used as the total CPU usage.
			// The value should fit a uint32 in order to avoid overflows later on when computing pod stats.

			/* #nosec */
			dummyUsageNanoCores := uint64(rand.Uint32())
			totalUsageNanoCores += dummyUsageNanoCores
			// Create a dummy value to be used as the total RAM usage.
			// The value should fit a uint32 in order to avoid overflows later on when computing pod stats.

			/* #nosec */
			dummyUsageBytes := uint64(rand.Uint32())
			totalUsageBytes += dummyUsageBytes
			// Append a ContainerStats object containing the dummy stats to the PodStats object.
			pss.Containers = append(pss.Containers, stats.ContainerStats{
				Name:      container.Name,
				StartTime: pod.CreationTimestamp,
				CPU: &stats.CPUStats{
					Time:           time,
					UsageNanoCores: &dummyUsageNanoCores,
				},
				Memory: &stats.MemoryStats{
					Time:       time,
					UsageBytes: &dummyUsageBytes,
				},
			})
		}

		// Populate the CPU and RAM stats for the pod and append the PodsStats object to the Summary object to be returned.
		pss.CPU = &stats.CPUStats{
			Time:           time,
			UsageNanoCores: &totalUsageNanoCores,
		}
		pss.Memory = &stats.MemoryStats{
			Time:       time,
			UsageBytes: &totalUsageBytes,
		}
		res.Pods = append(res.Pods, pss)
	}

	// Return the dummy stats.
	return res, nil
}

func (p *MockProvider) GetMetricsResource(ctx context.Context) ([]*dto.MetricFamily, error) {
	var span trace.Span
	ctx, span = trace.StartSpan(ctx, "GetMetricsResource") //nolint: ineffassign,staticcheck
	defer span.End()

	var (
		nodeNameStr      = "NodeName"
		podNameStr       = "PodName"
		containerNameStr = "containerName"
	)
	nodeLabels := []*dto.LabelPair{
		{
			Name:  &nodeNameStr,
			Value: &p.nodeName,
		},
	}

	metricsMap := p.generateMockMetrics(nil, "node", nodeLabels)
	p.podsMock.RLock()
	for _, pod := range p.podsMock.Map {
		podLabels := []*dto.LabelPair{
			{
				Name:  &nodeNameStr,
				Value: &p.nodeName,
			},
			{
				Name:  &podNameStr,
				Value: &pod.Name,
			},
		}
		metricsMap = p.generateMockMetrics(metricsMap, "pod", podLabels)
		for _, container := range pod.Spec.Containers {
			containerLabels := []*dto.LabelPair{
				{
					Name:  &nodeNameStr,
					Value: &p.nodeName,
				},
				{
					Name:  &podNameStr,
					Value: &pod.Name,
				},
				{
					Name:  &containerNameStr,
					Value: &container.Name,
				},
			}
			metricsMap = p.generateMockMetrics(metricsMap, "container", containerLabels)
		}
	}
	p.podsMock.RUnlock()

	res := []*dto.MetricFamily{}
	for metricName := range metricsMap {
		tempName := metricName
		tempMetrics := metricsMap[tempName]

		metricFamily := dto.MetricFamily{
			Name:   &tempName,
			Type:   p.getMetricType(tempName),
			Metric: tempMetrics,
		}
		res = append(res, &metricFamily)
	}

	return res, nil
}

// NotifyPods is called to set a pod notifier callback function. This should be called before any operations are done
// within the provider.
func (p *MockProvider) NotifyPods(ctx context.Context, notifier func(*v1.Pod)) {
	p.notifier = notifier

	p.podTracker = &PodsTracker{
		pods:           p.podsApiserver,
		updateCb:       notifier,
		handler:        p,
		lastEventCheck: time.UnixMicro(0),
		eventRecorder:  p.eventRecorder,
		provider:       p,
	}
	go p.podTracker.StartTracking(ctx)

	p.mockPodTracker = &MockPodsTracker{
		handler: p,
	}
	go p.mockPodTracker.StartTracking(ctx)
}

// ListActivePods interface impl.
func (p *MockProvider) ListActivePods(ctx context.Context) ([]PodIdentifier, error) {
	ctx, span := trace.StartSpan(ctx, "MockProvider.ListActivePods")
	defer span.End()

	providerPods, err := p.GetPods(ctx)
	if err != nil {
		return nil, err
	}
	podsIdentifiers := make([]PodIdentifier, 0, len(providerPods))

	for _, pod := range providerPods {
		podsIdentifiers = append(
			podsIdentifiers,
			PodIdentifier{
				namespace: pod.Namespace,
				name:      pod.Name,
			})
	}

	return podsIdentifiers, nil
}

// FetchPodStatus interface impl
func (p *MockProvider) FetchPodStatus(ctx context.Context, ns, name string) (*v1.PodStatus, error) {
	ctx, span := trace.StartSpan(ctx, "MockProvider.FetchPodStatus")
	defer span.End()

	return p.GetPodStatus(ctx, ns, name)
}

func (p *MockProvider) FetchPodEvents(ctx context.Context, pod *v1.Pod, evtSink func(timestamp *time.Time, object runtime.Object, eventtype, reason, messageFmt string, args ...interface{})) error {
	ctx, span := trace.StartSpan(ctx, "MockProvider.FetchPodEvents")
	defer span.End()
	// todo
	return nil
}

// CleanupPod interface impl
func (p *MockProvider) CleanupPod(ctx context.Context, ns, name string) error {
	ctx, span := trace.StartSpan(ctx, "MockProvider.CleanupPod")
	defer span.End()
	return p.deleteContainerGroup(ctx, ns, name)
}

func (p *MockProvider) UpdateMockPodsStatus(ctx context.Context) {
	p.podsMock.RLock()
	defer p.podsMock.RUnlock()
	for _, pod := range p.podsMock.Map {
		p.patchPodStatus(ctx, pod)
	}
}
