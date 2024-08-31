package metrics

import (
	"context"
	"fmt"

	"strings"
    "os/exec"
"net/http"
"log"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/karmada-io/dashboard/cmd/api/app/router"
	"github.com/karmada-io/dashboard/pkg/client"
	"github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	// "github.com/karmada-io/karmada/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
  
)
const (
	namespace = "karmada-system"
)
type PodInfo struct {
	Name        string `json:"name"`
}
func getMetrics(c *gin.Context) {
	appName := c.Param("app_name")
	podName := c.Param("pod_name")
	referenceName := c.Param("referenceName")

	if appName == "karmada-agent" {
        getKarmadaAgentMetrics(c, podName, referenceName)
        return
    }
	kubeClient := client.InClusterClient()
	
	// Get the specific pod
	pod, err := kubeClient.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error getting pod %s: %v\n", podName, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Pod not found"})
		return
	}

	// Check if the pod belongs to the specified app
	if !strings.HasPrefix(pod.Name, appName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pod does not belong to the specified app"})
		return
	}

	var port string
	if appName == "karmada-scheduler" || appName == "karmada-scheduler-estimator"  {
		port = "10351"
	} else if appName == "karmada-controller-manager" {
		port = "8080"
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid app name"})
		return
	}

	// Get metrics from the specific pod
	metricsOutput, err := kubeClient.CoreV1().RESTClient().Get().
		Namespace(namespace).
		Resource("pods").
		SubResource("proxy").
		Name(fmt.Sprintf("%s:%s", podName, port)).
		Suffix("metrics").
		Do(context.TODO()).Raw()

	if err != nil {
		log.Printf("Error executing metrics request for pod %s: %v\n", podName, err)
		c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve metrics from pod: %v", err))
		return
	}

	// Filter metrics based on referenceName
	filteredMetrics := filterMetrics(string(metricsOutput), referenceName)

	// Return raw metrics as plain text
	c.Data(http.StatusOK, "text/plain", []byte(filteredMetrics))
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

    // Construct the kubectl command
    cmdStr := fmt.Sprintf("kubectl get --kubeconfig ~/.kube/karmada.config --raw /apis/cluster.karmada.io/v1alpha1/clusters/%s/proxy/api/v1/namespaces/karmada-system/pods/", cluster.Name)
    cmd := exec.Command("sh", "-c", cmdStr)


    // Execute the command and capture both stdout and stderr
    output, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("failed to execute kubectl command for cluster %s: %v\nCommand: %s\nOutput: %s", 
            cluster.Name, err, cmdStr, string(output))
    }

    // Parse the output
    var podList corev1.PodList
    if err := json.Unmarshal(output, &podList); err != nil {
        return nil, fmt.Errorf("failed to unmarshal pod list for cluster %s: %v\nOutput: %s", 
            cluster.Name, err, string(output))
    }

    fmt.Printf("Found %d pods in cluster %s\n", len(podList.Items), cluster.Name)

    var podInfos []PodInfo
    for _, pod := range podList.Items {
        podInfos = append(podInfos, PodInfo{
            Name:        pod.Name,
        })
    }

    return podInfos, nil
}

func getKarmadaAgentPods(c *gin.Context) {
    kubeClient := client.InClusterKarmadaClient()

    // Get all clusters
    clusters, err := kubeClient.ClusterV1alpha1().Clusters().List(context.TODO(), metav1.ListOptions{})
    if err != nil {
        c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to list clusters: %v", err)})
        return
    }

    var agentPods []PodInfo
    var errors []string
	var pods string
    
	for _, cluster := range clusters.Items {
        // Check if the cluster is in Pull mode (case-insensitive)
        if strings.EqualFold(string(cluster.Spec.SyncMode), "Pull") {
            karmadaConfig, _, err := client.GetKarmadaConfig()
            if err != nil {
                errors = append(errors, fmt.Sprintf("Error getting Karmada config for cluster %s: %v", cluster.Name, err))
                continue
            }
            
            // Print debug information
            fmt.Printf("Karmada config for cluster %s: %+v\n", cluster.Name, karmadaConfig)
            
            pods, err := getClusterPods(&cluster)
            if err != nil {
                errors = append(errors, fmt.Sprintf("Cluster %s: %v", cluster.Name, err))
            } else {
                agentPods = append(agentPods, pods...)
            }
        }
    }

    if len(agentPods) == 0 {
		if len(errors) > 0 {
			c.JSON(500, gin.H{"errors": errors})
		} else {
			c.JSON(200, gin.H{
				"pods": pods,
				"errors": []string{},  
			})
		}
        return
    }

    c.JSON(200, gin.H{
        "pods":   agentPods,
    })
}
func init() {
	r := router.V1()
	r.GET("/metrics/:app_name/:pod_name/:referenceName", getMetrics)
	r.GET("/karmada-agent-pods", getKarmadaAgentPods)
}
