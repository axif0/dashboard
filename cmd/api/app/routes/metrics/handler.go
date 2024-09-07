package metrics

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
 

	"github.com/gin-gonic/gin"
	"github.com/karmada-io/dashboard/cmd/api/app/router"
	"github.com/karmada-io/dashboard/pkg/client"
	"github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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
	appName, podName, referenceName := c.Param("app_name"), c.Param("pod_name"), c.Param("referenceName")
	kubeClient := client.InClusterClient()

	if appName == karmadaAgent {
		getKarmadaAgentMetrics(c, podName, referenceName)
		return
	}

	pod, err := kubeClient.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
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

	metricsOutput, err := fetchPodMetrics(kubeClient.(*kubeclient.Clientset), podName, port)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve metrics from pod: %v", err))
		return
	}

	jsonMetrics, err := parseMetricsToJSON(string(metricsOutput))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse metrics to JSON"})
		return
	}

	// Save the JSON metrics to the database
	err = saveToDB(appName, podName, jsonMetrics)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save metrics to DB: %v", err)})
		return
	}

	c.Data(http.StatusOK, "application/json", []byte(jsonMetrics))
}

func getAppPort(appName string) string {
	switch {
	case strings.HasPrefix(appName, karmadaScheduler):
		return schedulerPort
	case strings.HasPrefix(appName, karmadaSchedulerEstimator):
		return schedulerPort
	case appName == karmadaControllerManager:
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

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get user home directory: %v", err)})
			return
		}
		kubeconfigPath = filepath.Join(homeDir, ".kube", "karmada.config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to build config for cluster %s: %v", clusterName, err)})
		return
	}

	config.Host = fmt.Sprintf("%s/apis/cluster.karmada.io/v1alpha1/clusters/%s/proxy", config.Host, clusterName)

	// Create a REST client specifically for accessing the metrics endpoint
	restClient, err := kubeclient.NewForConfig(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create REST client for cluster %s: %v", clusterName, err)})
		return
	}

	// Fetch metrics directly using the REST client
	result := restClient.CoreV1().RESTClient().Get().
		Namespace("karmada-system").
		Resource("pods").
		SubResource("proxy").
		Name(fmt.Sprintf("%s:8080", podName)).
		Suffix("metrics").
		Do(context.TODO())

	if result.Error() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to retrieve metrics: %v", result.Error())})
		return
	}

	metricsOutput, err := result.Raw()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to decode metrics response: %v", err)})
		return
	}

	jsonMetrics, err := parseMetricsToJSON(string(metricsOutput))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse metrics to JSON"})
		return
	}
 
	err = saveToDB(karmadaAgent, podName, jsonMetrics)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save metrics to DB: %v", err)})
		return
	}

	c.Data(http.StatusOK, "application/json", []byte(jsonMetrics))
}

func getClusterPods(cluster *v1alpha1.Cluster) ([]PodInfo, error) {
	fmt.Printf("Getting pods for cluster: %s\n", cluster.Name)

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %v", err)
		}
		kubeconfigPath = filepath.Join(homeDir, ".kube", "karmada.config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config for cluster %s: %v", cluster.Name, err)
	}

	config.Host = fmt.Sprintf("%s/apis/cluster.karmada.io/v1alpha1/clusters/%s/proxy", config.Host, cluster.Name)

	kubeClient, err := kubeclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubeclient for cluster %s: %v", cluster.Name, err)
	}

	podList, err := kubeClient.CoreV1().Pods("karmada-system").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for cluster %s: %v", cluster.Name, err)
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
	kubeClient := client.InClusterClient()

	podsMap := make(map[string][]PodInfo)
	var errors []string

	if appName == karmadaAgent {
		karmadaClient := client.InClusterKarmadaClient()
		clusters, err := karmadaClient.ClusterV1alpha1().Clusters().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list clusters: %v", err)})
			return
		}

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
	} else {
		pods, err := kubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", appName),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list pods: %v", err)})
			return
		}

		for _, pod := range pods.Items {
			podsMap[appName] = append(podsMap[appName], PodInfo{Name: pod.Name})
		}
	}

	if len(podsMap) == 0 && len(errors) > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"errors": errors})
		return
	}

	c.JSON(http.StatusOK, gin.H{appName: podsMap})
}

 
func init() {
	r := router.V1()
	r.GET("/metrics/:app_name/:pod_name/:referenceName", getMetrics)

	r.GET("/pods/:app_name", getKarmadaPods)
}

