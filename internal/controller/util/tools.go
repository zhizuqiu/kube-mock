package util

import (
	"fmt"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
	"time"
)

var (
	rootName = "kube-mock"
)

func NilError() error {
	return nil
}

// MergeLabels merges all the label maps received as argument into a single new label map.
func MergeLabels(allLabels ...map[string]string) map[string]string {
	res := map[string]string{}

	for _, labels := range allLabels {
		if labels != nil {
			for k, v := range labels {
				res[k] = v
			}
		}
	}
	return res
}

func GenerateLoopReconcileResult() reconcile.Result {
	return reconcile.Result{
		Requeue:      true,
		RequeueAfter: 2 * time.Second,
	}
}

// StripVersion the version part of gv
func StripVersion(gv string) string {
	if gv == "" {
		return gv
	}

	re := regexp.MustCompile(`^[vV][0-9].*`)
	// If it begins with only version, (group is nil), return empty string which maps to core group
	if re.MatchString(gv) {
		return ""
	}

	return strings.Split(gv, "/")[0]
}

// FilterActivePods returns pods that have not terminated.
func FilterActivePods(logger logr.Logger, pods []v1.Pod) []v1.Pod {
	var result []v1.Pod
	for _, p := range pods {
		if IsPodActive(p) {
			result = append(result, p)
		} else {
			logger.V(4).Info("Ignoring inactive pod", "pod", klog.KObj(&p), "phase", p.Status.Phase, "deletionTime", p.DeletionTimestamp)
		}
	}
	return result
}

func IsPodActive(p v1.Pod) bool {
	return v1.PodSucceeded != p.Status.Phase &&
		v1.PodFailed != p.Status.Phase &&
		p.DeletionTimestamp == nil
}

func GetPodsPrefix(controllerName string) string {
	// use the dash (if the name isn't too long) to make the pod name a bit prettier
	prefix := fmt.Sprintf("%s-", controllerName)
	return prefix
}

func defaultString(val string, defaultVal string) string {
	if val == "" {
		return defaultVal
	}
	return val
}
