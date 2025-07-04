/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	"fmt"                                            // 引入 fmt 包
	"github.com/prometheus/client_golang/prometheus" // 引入 prometheus 客户端
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics" // SDK 的 metrics 包
)

// PodMonitorReconciler reconciles a PodMonitor object
type PodMonitorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PodMonitor object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile

// metrics
var (
	// 使用 GaugeVec，因为它既可以设置一个值（时间戳），又可以用标签来区分不同的 Pod 和容器
	podLastTerminationInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pod_monitor_container_last_termination_info",
			Help: "Exposes information about the last termination of a container. The value is the unix timestamp of the termination.",
		},
		[]string{
			"namespace", // Pod 所在命名空间
			"pod",       // Pod 名称
			"container", // 容器名称
			"reason",    // 终止原因 (e.g., OOMKilled)
			"exit_code", // 退出码
		},
	)

	// 用于存储我们已经观察到的容器重启次数，防止重复处理
	// key: "namespace/podName/containerName", value: restartCount
	// 注意：这是一个简单的内存存储，如果 Operator 重启，状态会丢失。
	// 生产环境可以考虑更持久化的方案。
	observedRestarts = make(map[string]int32)
)

func init() {
	metrics.Registry.MustRegister(podLastTerminationInfo)
}

//func (r *PodMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//	_ = logf.FromContext(ctx)
//
//	// TODO(user): your logic here
//
//	return ctrl.Result{}, nil
//}

func (r *PodMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// 使用您文件中已有的 logf 别名
	log := logf.FromContext(ctx)

	// 1. 获取 Pod 对象
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch Pod")
			return ctrl.Result{}, err
		}
		// 如果 Pod 已被删除，则忽略
		return ctrl.Result{}, nil
	}

	// 2. 遍历所有容器状态
	for _, cs := range pod.Status.ContainerStatuses {
		// 创建一个唯一的键来识别这个容器
		containerKey := fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, cs.Name)

		// 3. 检查重启条件
		// 条件 1: 容器重启次数 > 我们已记录的次数
		// 条件 2: 容器存在上一次终止的状态
		if cs.RestartCount > observedRestarts[containerKey] && cs.LastTerminationState.Terminated != nil {
			log.Info("Detected container restart", "pod", pod.Name, "container", cs.Name, "restartCount", cs.RestartCount)

			// 4. 提取信息并更新 Prometheus 指标
			lastState := cs.LastTerminationState.Terminated
			reason := lastState.Reason
			exitCode := fmt.Sprintf("%d", lastState.ExitCode)
			// 将完成时间转换为 Unix 时间戳 (float64)
			finishedAt := float64(lastState.FinishedAt.Time.Unix())

			// 使用提取的信息设置 Gauge 指标
			podLastTerminationInfo.With(prometheus.Labels{
				"namespace": pod.Namespace,
				"pod":       pod.Name,
				"container": cs.Name,
				"reason":    reason,
				"exit_code": exitCode,
			}).Set(finishedAt)

			// 5. 更新我们内存中记录的重启次数
			observedRestarts[containerKey] = cs.RestartCount
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Named("podmonitor").
		Complete(r)
}
