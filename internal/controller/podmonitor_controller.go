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
	"crypto/x509"
	"encoding/pem"
	"time"

	"fmt"                                            // 引入 fmt 包
	"github.com/prometheus/client_golang/prometheus" // 引入 prometheus 客户端
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics" // SDK 的 metrics 包
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// PodMonitorReconciler reconciles a PodMonitor object
type PodMonitorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

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

	// 证书过期时间监控指标
	certificateExpirationTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pod_monitor_certificate_expiration_timestamp_seconds",
			Help: "Unix timestamp in seconds indicating when the certificate will expire",
		},
		[]string{
			"namespace",   // Secret 所在命名空间
			"secret_name", // Secret 名称
			"cert_type",   // 证书类型 (ca-cert, issuer-cert, etc.)
		},
	)

	// 证书剩余有效天数
	certificateDaysUntilExpiration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pod_monitor_certificate_days_until_expiration",
			Help: "Number of days until the certificate expires",
		},
		[]string{
			"namespace",   // Secret 所在命名空间
			"secret_name", // Secret 名称
			"cert_type",   // 证书类型
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
	metrics.Registry.MustRegister(certificateExpirationTime)
	metrics.Registry.MustRegister(certificateDaysUntilExpiration)
}

//func (r *PodMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//	_ = logf.FromContext(ctx)
//
//	// TODO(user): your logic here
//
//	return ctrl.Result{}, nil
//}

func (r *PodMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// 判断是 Pod 还是 Secret 的事件
	if req.Namespace == "linkerd" && req.Name == "linkerd-identity-issuer" {
		// 处理 Secret 事件
		return r.reconcileSecret(ctx, req)
	}

	// 处理 Pod 事件
	return r.reconcilePod(ctx, req)
}

// reconcilePod 处理 Pod 相关的逻辑
func (r *PodMonitorReconciler) reconcilePod(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

// reconcileSecret 处理 Secret 相关的逻辑
func (r *PodMonitorReconciler) reconcileSecret(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 获取 Secret 对象
	var secret corev1.Secret
	if err := r.Get(ctx, req.NamespacedName, &secret); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch secret")
			return ctrl.Result{}, err
		}
		// 如果 Secret 已被删除，清理相关指标
		certificateExpirationTime.DeletePartialMatch(prometheus.Labels{
			"namespace":   req.Namespace,
			"secret_name": req.Name,
		})
		certificateDaysUntilExpiration.DeletePartialMatch(prometheus.Labels{
			"namespace":   req.Namespace,
			"secret_name": req.Name,
		})
		return ctrl.Result{}, nil
	}

	// 检查证书数据，支持 .crt 和 .pem 两种格式
	for key, data := range secret.Data {
		// Linkerd identity issuer secret 通常包含以下证书
		// 支持 .crt 和 .pem 两种扩展名
		// 注意：实际的 Linkerd 使用 crt.pem 作为证书文件名
		if key == "ca.crt" || key == "issuer.crt" || key == "ca.pem" || key == "issuer.pem" || key == "crt.pem" {
			if err := r.checkCertificateExpiration(ctx, req.Namespace, req.Name, key, data); err != nil {
				log.Error(err, "Failed to check certificate expiration", "key", key)
			}
		}
	}

	// 定期重新检查，每小时一次
	return ctrl.Result{RequeueAfter: time.Hour}, nil
}

// parseCertificateFromPEM parses a PEM encoded certificate and returns the x509 certificate
func parseCertificateFromPEM(pemData []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

// checkCertificateExpiration checks the certificate expiration and updates metrics
func (r *PodMonitorReconciler) checkCertificateExpiration(ctx context.Context, namespace, secretName, certType string, certData []byte) error {
	log := logf.FromContext(ctx)

	cert, err := parseCertificateFromPEM(certData)
	if err != nil {
		log.Error(err, "Failed to parse certificate", "namespace", namespace, "secret", secretName, "certType", certType)
		return err
	}

	// Calculate expiration time and days until expiration
	expirationTime := cert.NotAfter
	now := time.Now()
	daysUntilExpiration := expirationTime.Sub(now).Hours() / 24

	log.Info("Certificate expiration info",
		"namespace", namespace,
		"secret", secretName,
		"certType", certType,
		"expirationTime", expirationTime,
		"daysUntilExpiration", daysUntilExpiration)

	// Update metrics
	certificateExpirationTime.With(prometheus.Labels{
		"namespace":   namespace,
		"secret_name": secretName,
		"cert_type":   certType,
	}).Set(float64(expirationTime.Unix()))

	certificateDaysUntilExpiration.With(prometheus.Labels{
		"namespace":   namespace,
		"secret_name": secretName,
		"cert_type":   certType,
	}).Set(daysUntilExpiration)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		// 也监听 Secret 资源，特别是 linkerd 命名空间下的
		Watches(&corev1.Secret{}, &handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					// 只关注 linkerd 命名空间下的 linkerd-identity-issuer secret
					return e.Object.GetNamespace() == "linkerd" && e.Object.GetName() == "linkerd-identity-issuer"
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return e.ObjectNew.GetNamespace() == "linkerd" && e.ObjectNew.GetName() == "linkerd-identity-issuer"
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false // 不关注删除事件
				},
			})).
		Named("podmonitor").
		Complete(r)
}
