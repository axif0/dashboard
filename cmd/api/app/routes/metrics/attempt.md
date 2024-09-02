package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/karmada-io/dashboard/cmd/api/app/router"
	"github.com/karmada-io/dashboard/pkg/client"
	"github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
)

const (
	namespace                 = "karmada-system"
	karmadaAgent              = "karmada-agent"
	karmadaScheduler          = "karmada-scheduler"
	karmadaSchedulerEstimator = "karmada-scheduler-estimator"
	karmadaControllerManager  = "karmada-controller-manager"
	schedulerPort             = "10351"
	controllerManagerPort     = "8080"
)

type PodInfo struct {
	Name string `json:"name"`
}

func getMetrics(c *gin.Context) {
	appName := c.Param("app_name")
	podName := c.Param("pod_name")
	referenceName := c.Param("referenceName")

	if appName == karmadaAgent {
		getKarmadaAgentMetrics(c, podName, referenceName)
		return
	}
	kubeClient := client.InClusterClient()

	pod, err := kubeClient.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error getting pod %s: %v\n", podName, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Pod not found"})
		return
	}

	if !strings.HasPrefix(pod.Name, appName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pod does not belong to the specified app"})
		return
	}

	port := getAppPort(appName)
	if port == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid app name"})
		return
	}

	// Convert kubeClient to *kubeclient.Clientset before passing
	metricsOutput, err := fetchPodMetrics(kubeClient.(*kubeclient.Clientset), podName, port)
	if err != nil {
		log.Printf("Error executing metrics request for pod %s: %v\n", podName, err)
		c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve metrics from pod: %v", err))
		return
	}

	filteredMetrics := filterMetrics(string(metricsOutput), referenceName)
	c.Data(http.StatusOK, "text/plain", []byte(filteredMetrics))
}

func getAppPort(appName string) string {
	switch appName {
	case karmadaScheduler, karmadaSchedulerEstimator:
		return schedulerPort
	case karmadaControllerManager:
		return controllerManagerPort
	default:
		return ""
	}
}

func fetchPodMetrics(kubeClient *kubeclient.Clientset, podName, port string) ([]byte, error) {
	return kubeClient.CoreV1().RESTClient().Get().
		Namespace(namespace).
		Resource("pods").
		SubResource("proxy").
		Name(fmt.Sprintf("%s:%s", podName, port)).
		Suffix("metrics").
		Do(context.TODO()).Raw()
}

func filterMetrics(metricsOutput, referenceName string) string {
	if referenceName == "" {
		return metricsOutput
	}
	lines := strings.Split(metricsOutput, "\n")
	var filteredLines []string
	for _, line := range lines {
		if strings.Contains(line, referenceName) {
			filteredLines = append(filteredLines, line)
		}
	}
	return strings.Join(filteredLines, "\n")
}

func getKarmadaAgentMetrics(c *gin.Context, podName, referenceName string) {
	kubeClient := client.InClusterKarmadaClient()
	clusters, err := kubeClient.ClusterV1alpha1().Clusters().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list clusters: %v", err)})
		return
	}

	var clusterName string
	found := false
	for _, cluster := range clusters.Items {
		if strings.EqualFold(string(cluster.Spec.SyncMode), "Pull") {
			clusterName = cluster.Name
			found = true
			break
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "No cluster in 'Pull' mode found"})
		return
	}

	cmdStr := fmt.Sprintf("kubectl get --kubeconfig ~/.kube/karmada.config --raw /apis/cluster.karmada.io/v1alpha1/clusters/%s/proxy/api/v1/namespaces/karmada-system/pods/%s:8080/proxy/metrics | grep %s", clusterName, podName, referenceName)
	cmd := exec.Command("sh", "-c", cmdStr)

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to execute command: %v\nOutput: %s", err, string(output))})
		return
	}

	c.Data(http.StatusOK, "text/plain", output)
}

func getClusterPods(cluster *v1alpha1.Cluster) ([]PodInfo, error) {
	fmt.Printf("Getting pods for cluster: %s\n", cluster.Name)

	cmdStr := fmt.Sprintf("kubectl get --kubeconfig ~/.kube/karmada.config --raw /apis/cluster.karmada.io/v1alpha1/clusters/%s/proxy/api/v1/namespaces/karmada-system/pods/", cluster.Name)
	cmd := exec.Command("sh", "-c", cmdStr)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute kubectl command for cluster %s: %v\nCommand: %s\nOutput: %s",
			cluster.Name, err, cmdStr, string(output))
	}

	var podList corev1.PodList
	if err := json.Unmarshal(output, &podList); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pod list for cluster %s: %v\nOutput: %s",
			cluster.Name, err, string(output))
	}

	fmt.Printf("Found %d pods in cluster %s\n", len(podList.Items), cluster.Name)

	var podInfos []PodInfo
	for _, pod := range podList.Items {
		podInfos = append(podInfos, PodInfo{
			Name: pod.Name,
		})
	}

	return podInfos, nil
}

func getKarmadaPods(c *gin.Context) {
	appName := c.Param("app_name")

	// Initialize a map to hold the pods for all scenarios
	podsMap := make(map[string][]PodInfo)

	if appName == karmadaAgent {
		kubeClient := client.InClusterKarmadaClient()
		clusters, err := kubeClient.ClusterV1alpha1().Clusters().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list clusters: %v", err)})
			return
		}

		var errors []string

		for _, cluster := range clusters.Items {
			if strings.EqualFold(string(cluster.Spec.SyncMode), "Pull") {
				pods, err := getClusterPods(&cluster)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Cluster %s: %v", cluster.Name, err))
				} else {
					podsMap[cluster.Name] = pods
				}
			}
		}

		if len(podsMap) == 0 && len(errors) > 0 {
			c.JSON(http.StatusInternalServerError, gin.H{"errors": errors})
			return
		}

		c.JSON(http.StatusOK, gin.H{appName: podsMap})
	} else if appName == karmadaScheduler || appName == karmadaSchedulerEstimator || appName == karmadaControllerManager {
		kubeClient := client.InClusterClient()
		pods, err := kubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", appName),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list pods: %v", err)})
			return
		}

		var podInfos []PodInfo
		for _, pod := range pods.Items {
			podInfos = append(podInfos, PodInfo{Name: pod.Name})
		}

		podsMap[appName] = podInfos
		c.JSON(http.StatusOK, gin.H{appName: podsMap[appName]})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid app name"})
	}
}

func init() {
	r := router.V1()
	r.GET("/metrics/:app_name/:pod_name/:referenceName", getMetrics)
	r.GET("/pods/:app_name", getKarmadaPods)  
}
