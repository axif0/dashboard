package metrics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/karmada-io/dashboard/cmd/api/app/router"
	"github.com/karmada-io/dashboard/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	namespace = "karmada-system"
)

func getMetrics(c *gin.Context) {
	appName := c.Param("app_name")
	podName := c.Param("pod_name")
	referenceName := c.Param("referenceName")

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

func init() {
	r := router.V1()
	r.GET("/metrics/:app_name/:pod_name/:referenceName", getMetrics)
}