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
	appName := c.Param("app_name")
	kubeClient := client.InClusterClient()

	podsMap, errors := getKarmadaPods(appName)
	if len(podsMap) == 0 && len(errors) > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"errors": errors})
		return
	}

	var jsonMetrics string
	for _, pods := range podsMap {
		for _, pod := range pods {
			if appName == karmadaAgent {
				jsonMetrics, err := getKarmadaAgentMetrics(pod.Name)
				if err != nil {
					continue
				}
				c.Data(http.StatusOK, "application/json", []byte(jsonMetrics))
				err = saveToDB(appName, pod.Name, jsonMetrics)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save metrics to DB: %v", err)})
					return
				}
			} else {
				port := getAppPort(appName)
				if port == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid app name"})
					return
				}

				metricsOutput, err := kubeClient.CoreV1().RESTClient().Get().
					Namespace(namespace).
					Resource("pods").
					SubResource("proxy").
					Name(fmt.Sprintf("%s:%s", pod.Name, port)).
					Suffix("metrics").
					Do(context.TODO()).Raw()

				if err != nil {
					continue
				}

				jsonMetrics, err = parseMetricsToJSON(string(metricsOutput))
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse metrics to JSON"})
					return
				}

				err = saveToDB(appName, pod.Name, jsonMetrics)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save metrics to DB: %v", err)})
					return
				}
			}
		}
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

func getKarmadaAgentMetrics(podName string) (string, error) {
	kubeClient := client.InClusterKarmadaClient()
	clusters, err := kubeClient.ClusterV1alpha1().Clusters().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list clusters: %v", err)
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
		return "", fmt.Errorf("no cluster in 'Pull' mode found")
	}

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %v", err)
		}
		kubeconfigPath = filepath.Join(homeDir, ".kube", "karmada.config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to build config for cluster %s: %v", clusterName, err)
	}

	config.Host = fmt.Sprintf("%s/apis/cluster.karmada.io/v1alpha1/clusters/%s/proxy", config.Host, clusterName)

	restClient, err := kubeclient.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create REST client for cluster %s: %v", clusterName, err)
	}

	result := restClient.CoreV1().RESTClient().Get().
		Namespace("karmada-system").
		Resource("pods").
		SubResource("proxy").
		Name(fmt.Sprintf("%s:8080", podName)).
		Suffix("metrics").
		Do(context.TODO())

	if result.Error() != nil {
		return "", fmt.Errorf("failed to retrieve metrics: %v", result.Error())
	}

	metricsOutput, err := result.Raw()
	if err != nil {
		return "", fmt.Errorf("failed to decode metrics response: %v", err)
	}

	jsonMetrics, err := parseMetricsToJSON(string(metricsOutput))
	if err != nil {
		return "", fmt.Errorf("filed to parse metrics to JSON: %v", err)
	}

	return jsonMetrics, nil
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

func getKarmadaPods(appName string) (map[string][]PodInfo, []string) {
	kubeClient := client.InClusterClient()

	podsMap := make(map[string][]PodInfo)
	var errors []string

	if appName == karmadaAgent {
		karmadaClient := client.InClusterKarmadaClient()
		clusters, err := karmadaClient.ClusterV1alpha1().Clusters().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			errors = append(errors, fmt.Sprintf("Failed to list clusters: %v", err))
			return podsMap, errors
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
			errors = append(errors, fmt.Sprintf("failed to list pods: %v", err))
			return podsMap, errors
		}

		for _, pod := range pods.Items {
			podsMap[appName] = append(podsMap[appName], PodInfo{Name: pod.Name})
		}
	}

	return podsMap, errors
}

func init() {
	r := router.V1()
	r.GET("/metrics/:app_name", getMetrics)
	
}

