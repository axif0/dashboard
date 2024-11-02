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
	fmt.Println("Requested app name:", appName) // Debug: log requested app name

	kubeClient := client.InClusterClient()

	podsMap, errors := getKarmadaPods(appName)
	fmt.Println("Pods map:", podsMap) // Debug: log pods map
	fmt.Println("Errors:", errors) // Debug: log errors

	if len(podsMap) == 0 && len(errors) > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"errors": errors})
		return
	}

	var jsonMetrics string
	for clusterName, pods := range podsMap {
		for _, pod := range pods {
			fmt.Println("Processing pod:", pod.Name, "in cluster:", clusterName) // Debug: log pod and cluster being processed
			if appName == karmadaAgent {
				jsonMetrics, err := getKarmadaAgentMetrics(pod.Name, clusterName)
				if err != nil {
					fmt.Println("Error getting Karmada agent metrics:", err) // Debug: log error
					continue
				}
				c.Data(http.StatusOK, "application/json", []byte(jsonMetrics))
				err = saveToDB(appName, pod.Name, jsonMetrics)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save metrics to DB: %v", err)})
					return
				}
			} else {
				port := schedulerPort  
				if appName == karmadaControllerManager {
					port = controllerManagerPort
				}
				
				metricsOutput, err := kubeClient.CoreV1().RESTClient().Get().
					Namespace(namespace).
					Resource("pods").
					SubResource("proxy").
					Name(fmt.Sprintf("%s:%s", pod.Name, port)).
					Suffix("metrics").
					Do(context.TODO()).Raw()

				if err != nil {
					fmt.Println("Error retrieving metrics:", err) // Debug: log error
					continue
				}
	 
				jsonMetrics, err = parseMetricsToJSON(string(metricsOutput))
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse metrics to JSON"})
					return
				}
				// fmt.Println("jsonMetrics: ", jsonMetrics) // Debug: log JSON metrics
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

func getKarmadaAgentMetrics(podName string, clusterName string) (string, error) {
	kubeClient := client.InClusterKarmadaClient()
	clusters, err := kubeClient.ClusterV1alpha1().Clusters().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list clusters: %v", err)
	}

	for _, cluster := range clusters.Items {
		if strings.EqualFold(string(cluster.Spec.SyncMode), "Pull") {
			clusterName = cluster.Name
			break
		}
	}

	if clusterName=="" {
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
	r.GET("/metrics/:app_name", getMetrics) // get all metrics from karmada
	r.GET("/metrics/:app_name/:pod_name", queryMetrics) // get metrics from db
	// http://localhost:8000/api/v1/metrics/karmada_controller_manager/karmada-controller-manager-54d9c78689-qhgdl?type=details&mname=cluster_sync_status_duration_seconds
	// http://localhost:8000/api/v1/metrics/karmada_controller_manager/karmada-controller-manager-54d9c78689-qhgdl?type=tables
	// http://localhost:8000/api/v1/metrics/karmada_controller_manager/karmada-controller-manager-54d9c78689-k6pfq?type=mname
	// http://localhost:8000/api/v1/metrics/karmada_controller_manager/karmada-controller-manager-54d9c78689-qhgdl
	} 
	