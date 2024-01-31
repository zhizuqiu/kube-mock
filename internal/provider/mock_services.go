package provider

import (
	"container/list"
	"context"
	"fmt"
	dto "github.com/prometheus/client_model/go"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"github.com/zhizuqiu/kube-mock/internal/provider/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"time"
)

const (
	delayRunningAnnotation   = "zhizuqiu.github.com/delayRunning"
	delaySucceededAnnotation = "zhizuqiu.github.com/delaySucceeded"
	delayFailedAnnotation    = "zhizuqiu.github.com/delayFailed"

	// todo implements
	mockStatusAnnotation = "zhizuqiu.github.com/mockStatus"

	podIPsAnnotation = "zhizuqiu.github.com/podIPs"

	PodCompleted       = "PodCompleted"
	ContainersNotReady = "ContainersNotReady"
	Error              = "Error"
	Completed          = "Completed"

	PodDesired v1.PodConditionType = "Desired"
)

var _delayAnnotations = []string{
	delayRunningAnnotation,
	delaySucceededAnnotation,
	delayFailedAnnotation,
}

// Capacity returns a resource list containing the capacity limits.
func (p *MockProvider) capacity() v1.ResourceList {
	rl := v1.ResourceList{
		"cpu":    resource.MustParse(p.config.CPU),
		"memory": resource.MustParse(p.config.Memory),
		"pods":   resource.MustParse(p.config.Pods),
	}
	for k, v := range p.config.Others {
		rl[v1.ResourceName(k)] = resource.MustParse(v)
	}
	return rl
}

// NodeConditions returns a list of conditions (Ready, OutOfDisk, etc), for updates to the node status
// within Kubernetes.
func (p *MockProvider) nodeConditions() []v1.NodeCondition {
	// TODO: Make this configurable
	return []v1.NodeCondition{
		{
			Type:               "Ready",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletPending",
			Message:            "kubelet is pending.",
		},
		{
			Type:               "OutOfDisk",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientDisk",
			Message:            "kubelet has sufficient disk space available",
		},
		{
			Type:               "MemoryPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientMemory",
			Message:            "kubelet has sufficient memory available",
		},
		{
			Type:               "DiskPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasNoDiskPressure",
			Message:            "kubelet has no disk pressure",
		},
		{
			Type:               "NetworkUnavailable",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "RouteCreated",
			Message:            "RouteController created a route",
		},
	}

}

// NodeAddresses returns a list of addresses for the node status
// within Kubernetes.
func (p *MockProvider) nodeAddresses() []v1.NodeAddress {
	return []v1.NodeAddress{
		{
			Type:    "InternalIP",
			Address: p.internalIP,
		},
	}
}

// NodeDaemonEndpoints returns NodeDaemonEndpoints for the node status
// within Kubernetes.
func (p *MockProvider) nodeDaemonEndpoints() v1.NodeDaemonEndpoints {
	return v1.NodeDaemonEndpoints{
		KubeletEndpoint: v1.DaemonEndpoint{
			Port: p.daemonEndpointPort,
		},
	}
}

func (p *MockProvider) generateMockMetrics(metricsMap map[string][]*dto.Metric, resourceType string, label []*dto.LabelPair) map[string][]*dto.Metric {
	var (
		cpuMetricSuffix    = "_cpu_usage_seconds_total"
		memoryMetricSuffix = "_memory_working_set_bytes"
		dummyValue         = float64(100)
	)

	if metricsMap == nil {
		metricsMap = map[string][]*dto.Metric{}
	}

	finalCpuMetricName := resourceType + cpuMetricSuffix
	finalMemoryMetricName := resourceType + memoryMetricSuffix

	newCPUMetric := dto.Metric{
		Label: label,
		Counter: &dto.Counter{
			Value: &dummyValue,
		},
	}
	newMemoryMetric := dto.Metric{
		Label: label,
		Gauge: &dto.Gauge{
			Value: &dummyValue,
		},
	}
	// if metric family exists add to metric array
	if cpuMetrics, ok := metricsMap[finalCpuMetricName]; ok {
		metricsMap[finalCpuMetricName] = append(cpuMetrics, &newCPUMetric)
	} else {
		metricsMap[finalCpuMetricName] = []*dto.Metric{&newCPUMetric}
	}
	if memoryMetrics, ok := metricsMap[finalMemoryMetricName]; ok {
		metricsMap[finalMemoryMetricName] = append(memoryMetrics, &newMemoryMetric)
	} else {
		metricsMap[finalMemoryMetricName] = []*dto.Metric{&newMemoryMetric}
	}

	return metricsMap
}

func (p *MockProvider) getMetricType(metricName string) *dto.MetricType {
	var (
		dtoCounterMetricType = dto.MetricType_COUNTER
		dtoGaugeMetricType   = dto.MetricType_GAUGE
		cpuMetricSuffix      = "_cpu_usage_seconds_total"
		memoryMetricSuffix   = "_memory_working_set_bytes"
	)
	if strings.HasSuffix(metricName, cpuMetricSuffix) {
		return &dtoCounterMetricType
	}
	if strings.HasSuffix(metricName, memoryMetricSuffix) {
		return &dtoGaugeMetricType
	}

	return nil
}

func buildKeyFromNames(namespace string, name string) (string, error) {
	return fmt.Sprintf("%s-%s", namespace, name), nil
}

// buildKey is a helper for building the "key" for the providers pod store.
func buildKey(pod *v1.Pod) (string, error) {
	if pod.ObjectMeta.Namespace == "" {
		return "", fmt.Errorf("pod namespace not found")
	}

	if pod.ObjectMeta.Name == "" {
		return "", fmt.Errorf("pod name not found")
	}

	return buildKeyFromNames(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
}

// addAttributes adds the specified attributes to the provided span.
// attrs must be an even-sized list of string arguments.
// Otherwise, the span won't be modified.
// TODO: Refactor and move to a "tracing utilities" package.
func addAttributes(ctx context.Context, span trace.Span, attrs ...string) context.Context {
	if len(attrs)%2 == 1 {
		return ctx
	}
	for i := 0; i < len(attrs); i += 2 {
		ctx = span.WithField(ctx, attrs[i], attrs[i+1])
	}
	return ctx
}

func (p *MockProvider) deleteContainerGroup(ctx context.Context, podNS, podName string) error {
	ctx, span := trace.StartSpan(ctx, "mock.deleteContainerGroup")
	defer span.End()
	// todo
	return nil
}

func (p *MockProvider) patchPodStatus(ctx context.Context, pod *v1.Pod) {

	nowStatus := p.getNowStatus(ctx, pod)
	if nowStatus == nil {
		return
	}

	pod.Status = *nowStatus
}

type DelayItem struct {
	Key       string
	Delay     time.Duration
	PodStatus *v1.PodStatus
}

func (p *MockProvider) getNowStatus(ctx context.Context, pod *v1.Pod) *v1.PodStatus {
	delayList := p.createDelayList(ctx, pod)
	if delayList.Len() < 1 {
		return p.createDefaultRunningStatus(pod)
	}
	for element := delayList.Front(); element != nil; element = element.Next() {
		delayItem := element.Value.(DelayItem)
		delay := delayItem.Delay
		creationTimestamp := pod.GetCreationTimestamp()
		startAt := creationTimestamp.Add(delay)
		now := metav1.NewTime(time.Now())
		next := element.Next()
		if now.Sub(startAt) <= 0 {
			return nil
		} else {
			if next == nil {
				return delayItem.PodStatus
			} else {
				nextItem := next.Value.(DelayItem)
				nextDelay := nextItem.Delay
				nextAt := creationTimestamp.Add(nextDelay)
				if now.Sub(nextAt) <= 0 {
					return delayItem.PodStatus
				} else {
					continue
				}
			}
		}
	}
	return nil
}

func (p *MockProvider) createDelayList(ctx context.Context, pod *v1.Pod) *list.List {
	delayList := list.New()
	for _, annotation := range _delayAnnotations {
		if durationStr, ok := pod.Annotations[annotation]; ok {
			duration := parseDuration(ctx, durationStr)
			delayItem := DelayItem{
				Key:       annotation,
				Delay:     duration,
				PodStatus: p.createStatus(ctx, annotation, pod),
			}
			if delayList.Len() < 1 {
				delayList.PushFront(delayItem)
			} else {
				for element := delayList.Front(); element != nil; element = element.Next() {
					item := element.Value.(DelayItem)
					if item.Delay >= duration {
						delayList.InsertBefore(delayItem, element)
					} else {
						next := element.Next()
						if next == nil {
							delayList.InsertAfter(delayItem, element)
						} else {
							nextItem := next.Value.(DelayItem)
							if nextItem.Delay > duration {
								delayList.InsertAfter(delayItem, element)
							} else {
								continue
							}
						}
					}
				}
			}
		}
	}
	return delayList
}

func parseDuration(ctx context.Context, value string) time.Duration {
	t, err := time.ParseDuration(value)
	if err != nil {
		log.G(ctx).Errorf("ParseDuration error, Annotation: %q", value)
	}
	return t
}

// todo QOSClass、initContainersStatus、restart
func (p *MockProvider) createStatus(ctx context.Context, annotation string, pod *v1.Pod) *v1.PodStatus {
	switch annotation {
	case delayRunningAnnotation:
		return p.createRunningStatus(ctx, pod)
	case delaySucceededAnnotation:
		return p.createSucceededStatus(ctx, pod)
	case delayFailedAnnotation:
		return p.createFailedStatus(ctx, pod)
	default:
		return p.createRunningStatus(ctx, pod)
	}
}

func (p *MockProvider) createDefaultRunningStatus(pod *v1.Pod) *v1.PodStatus {
	podIp, podIPs := p.getPodIps(pod)
	creationTimestamp := pod.GetCreationTimestamp()
	podStatus := &v1.PodStatus{
		Phase:     v1.PodRunning,
		HostIP:    p.internalIP,
		PodIP:     podIp,
		PodIPs:    podIPs,
		QOSClass:  v1.PodQOSBurstable,
		StartTime: &creationTimestamp,
		Conditions: []v1.PodCondition{
			{
				Type:               v1.PodInitialized,
				Status:             v1.ConditionTrue,
				LastTransitionTime: creationTimestamp,
			},
			{
				Type:               v1.PodReady,
				Status:             v1.ConditionTrue,
				LastTransitionTime: creationTimestamp,
			},
			{
				Type:               v1.ContainersReady,
				Status:             v1.ConditionTrue,
				LastTransitionTime: creationTimestamp,
			},
			{
				Type:               v1.PodScheduled,
				Status:             v1.ConditionTrue,
				LastTransitionTime: creationTimestamp,
			},
			createDesiredPodCondition(creationTimestamp, "defaultRunningStatus"),
		},
	}

	for _, container := range pod.Spec.Containers {
		podStatus.ContainerStatuses = append(podStatus.ContainerStatuses, v1.ContainerStatus{
			Name:         container.Name,
			Image:        container.Image,
			Ready:        true,
			Started:      boolP(true),
			RestartCount: 0,
			State: v1.ContainerState{
				Running: &v1.ContainerStateRunning{
					StartedAt: creationTimestamp,
				},
			},
		})
	}
	return podStatus
}

func getDelayDuration(ctx context.Context, annotation string, pod *v1.Pod) time.Duration {
	if durationStr, ok := pod.Annotations[annotation]; ok {
		duration := parseDuration(ctx, durationStr)
		return duration
	}
	return 0
}

func (p *MockProvider) createRunningStatus(ctx context.Context, pod *v1.Pod) *v1.PodStatus {
	podIp, podIPs := p.getPodIps(pod)

	creationTimestamp := pod.GetCreationTimestamp()
	delay := getDelayDuration(ctx, delayRunningAnnotation, pod)
	startTime := metav1.NewTime(creationTimestamp.Add(delay))

	podStatus := &v1.PodStatus{
		Phase:     v1.PodRunning,
		HostIP:    p.internalIP,
		PodIP:     podIp,
		PodIPs:    podIPs,
		QOSClass:  v1.PodQOSBurstable,
		StartTime: &startTime,
		Conditions: []v1.PodCondition{
			{
				Type:               v1.PodInitialized,
				Status:             v1.ConditionTrue,
				LastTransitionTime: startTime,
			},
			{
				Type:               v1.PodReady,
				Status:             v1.ConditionTrue,
				LastTransitionTime: startTime,
			},
			{
				Type:               v1.ContainersReady,
				Status:             v1.ConditionTrue,
				LastTransitionTime: startTime,
			},
			{
				Type:               v1.PodScheduled,
				Status:             v1.ConditionTrue,
				LastTransitionTime: startTime,
			},
			createDesiredPodCondition(startTime, delayRunningAnnotation),
		},
	}

	for _, container := range pod.Spec.Containers {
		podStatus.ContainerStatuses = append(podStatus.ContainerStatuses, v1.ContainerStatus{
			Name:         container.Name,
			Image:        container.Image,
			Ready:        true,
			Started:      boolP(true),
			RestartCount: 0,
			State: v1.ContainerState{
				Running: &v1.ContainerStateRunning{
					StartedAt: startTime,
				},
			},
		})
	}
	return podStatus
}

func (p *MockProvider) createSucceededStatus(ctx context.Context, pod *v1.Pod) *v1.PodStatus {
	creationTimestamp := pod.GetCreationTimestamp()
	delayRunning := getDelayDuration(ctx, delayRunningAnnotation, pod)
	startTime := metav1.NewTime(creationTimestamp.Add(delayRunning))

	delaySucceeded := getDelayDuration(ctx, delaySucceededAnnotation, pod)
	finishedAt := metav1.NewTime(creationTimestamp.Add(delaySucceeded))

	podIp, podIPs := p.getPodIps(pod)
	podStatus := &v1.PodStatus{
		Phase:     v1.PodSucceeded,
		HostIP:    p.internalIP,
		PodIP:     podIp,
		PodIPs:    podIPs,
		QOSClass:  v1.PodQOSBurstable,
		StartTime: &startTime,
		Conditions: []v1.PodCondition{
			{
				Type:               v1.PodInitialized,
				Status:             v1.ConditionTrue,
				LastTransitionTime: startTime,
				Reason:             PodCompleted,
			},
			{
				Type:               v1.PodReady,
				Status:             v1.ConditionFalse,
				LastTransitionTime: startTime,
				Reason:             PodCompleted,
			},
			{
				Type:               v1.ContainersReady,
				Status:             v1.ConditionFalse,
				LastTransitionTime: startTime,
				Reason:             PodCompleted,
			},
			{
				Type:               v1.PodScheduled,
				Status:             v1.ConditionTrue,
				LastTransitionTime: startTime,
			},
			createDesiredPodCondition(startTime, delaySucceededAnnotation),
		},
	}

	for _, container := range pod.Spec.Containers {
		podStatus.ContainerStatuses = append(podStatus.ContainerStatuses, v1.ContainerStatus{
			Name:         container.Name,
			Image:        container.Image,
			Ready:        true,
			RestartCount: 0,
			State: v1.ContainerState{
				Terminated: &v1.ContainerStateTerminated{
					ExitCode:   0,
					StartedAt:  startTime,
					FinishedAt: finishedAt,
					Reason:     Completed,
				},
			},
		})
	}
	return podStatus
}

func (p *MockProvider) createFailedStatus(ctx context.Context, pod *v1.Pod) *v1.PodStatus {
	creationTimestamp := pod.GetCreationTimestamp()
	delayRunning := getDelayDuration(ctx, delayRunningAnnotation, pod)
	startTime := metav1.NewTime(creationTimestamp.Add(delayRunning))

	delayFailed := getDelayDuration(ctx, delayFailedAnnotation, pod)
	finishedAt := metav1.NewTime(creationTimestamp.Add(delayFailed))

	podIp, podIPs := p.getPodIps(pod)
	podStatus := &v1.PodStatus{
		Phase:     v1.PodFailed,
		HostIP:    p.internalIP,
		PodIP:     podIp,
		PodIPs:    podIPs,
		QOSClass:  v1.PodQOSBurstable,
		StartTime: &startTime,
		Conditions: []v1.PodCondition{
			{
				Type:               v1.PodInitialized,
				Status:             v1.ConditionTrue,
				LastTransitionTime: startTime,
				Reason:             PodCompleted,
			},
			{
				Type:               v1.PodReady,
				Status:             v1.ConditionFalse,
				LastTransitionTime: startTime,
				Reason:             ContainersNotReady,
				Message:            "containers with unready status: [//todo]",
			},
			{
				Type:               v1.ContainersReady,
				Status:             v1.ConditionFalse,
				LastTransitionTime: startTime,
				Reason:             ContainersNotReady,
				Message:            "containers with unready status: [//todo]",
			},
			{
				Type:               v1.PodScheduled,
				Status:             v1.ConditionTrue,
				LastTransitionTime: startTime,
			},
			createDesiredPodCondition(startTime, delayFailedAnnotation),
		},
	}

	for _, container := range pod.Spec.Containers {
		podStatus.ContainerStatuses = append(podStatus.ContainerStatuses, v1.ContainerStatus{
			Name:         container.Name,
			Image:        container.Image,
			Ready:        false,
			Started:      boolP(false),
			RestartCount: 0,
			State: v1.ContainerState{
				Terminated: &v1.ContainerStateTerminated{
					ExitCode:   1,
					StartedAt:  startTime,
					FinishedAt: finishedAt,
					Reason:     Error,
				},
			},
		})
	}
	return podStatus
}

func (p *MockProvider) getPodIps(pod *v1.Pod) (string, []v1.PodIP) {
	podIp := pod.Annotations[podIPsAnnotation]
	if podIp == "" {
		podIp = "1.2.3.4"
	}
	if pod.Spec.HostNetwork {
		podIp = p.internalIP
	}
	podIps := strings.Split(podIp, ",")
	var podIPs []v1.PodIP
	for _, ip := range podIps {
		podIPs = append(podIPs, v1.PodIP{IP: ip})
	}
	return podIp, podIPs
}

func boolP(b bool) *bool {
	return &b
}

func createDesiredPodCondition(time metav1.Time, mess string) v1.PodCondition {
	return v1.PodCondition{
		Type:               PodDesired,
		Status:             v1.ConditionTrue,
		LastTransitionTime: time,
		Message:            util.MD5(mess),
	}
}

func IsDesiredPodCondition(podStatus *v1.PodStatus, pod *v1.Pod) bool {
	for _, condition := range podStatus.Conditions {
		if condition.Type == PodDesired {
			if pod.Status.Conditions != nil {
				for _, podCondition := range pod.Status.Conditions {
					if podCondition.Type == PodDesired {
						if condition.Message == podCondition.Message {
							return true
						}
					}
				}
			}
		}
	}
	return false
}
