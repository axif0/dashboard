package metrics

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/karmada-io/dashboard/cmd/api/app/router"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getMetrics retrieves metrics from the specified pod and returns it as a JSON response.
func getMetrics(c *gin.Context) {
	// Specify the kubeconfig file and context
	kubeconfig := "/home/ubuntu/.kube/karmada.config"
	context := "karmada-host"

	// Get the list of pods with the label app=karmada-controller-manager
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "--context", context, "-n", "karmada-system", "get", "po", "-l", "app=karmada-controller-manager")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error executing kubectl command: %v\n", err)
		log.Printf("Output: %s\n", output)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error executing kubectl command"})
		return
	}

	// Extract pod names from the output
	lines := strings.Split(string(output), "\n")
	podNames := []string{}

	if len(lines) > 1 {
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) == "" {
				continue // Skip empty lines
			}

			columns := strings.Fields(line)
			if len(columns) > 0 {
				podName := columns[0] // Select the first column as the pod name
				podNames = append(podNames, podName)
			}
		}
	}

	// Attempt to get metrics from each pod until successful
	for _, podName := range podNames {
		log.Printf("Attempting to get metrics from pod: %s\n", podName)

		metricsCmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "--context", context, "get", "--raw", fmt.Sprintf("/api/v1/namespaces/karmada-system/pods/%s:8080/proxy/metrics", podName))
		metricsOutput, err := metricsCmd.CombinedOutput()
		if err != nil {
			log.Printf("Error executing metrics command for pod %s: %v\n", podName, err)
			log.Printf("Output: %s\n", metricsOutput)
			continue // Try the next pod
		}

		// Filter metrics output
		metricsStr := string(metricsOutput)
		metricsMap, err := parseMetrics(metricsStr)
		if err != nil {
			log.Printf("Error parsing metrics: %v\n", err)
			continue // Try the next pod
		}
		if len(metricsMap) > 0 {
			log.Printf("Metrics from pod %s: %+v\n", podName, metricsMap)
			c.JSON(http.StatusOK, gin.H{"metrics": metricsMap})
			return
		}
	}

	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve metrics from any pod"})
}

func parseMetrics(metricsOutput string) (map[string]interface{}, error) {
	metricsLines := strings.Split(metricsOutput, "\n")
	clusterMetrics := make(map[string]interface{})

	for _, line := range metricsLines {
		if strings.Contains(line, "cluster_sync_status_duration_seconds") {
			// Extract cluster_name
			clusterNameStart := strings.Index(line, "cluster_name=\"")
			if clusterNameStart == -1 {
				continue // skip lines without cluster_name
			}
			clusterNameStart += len("cluster_name=\"")
			clusterNameEnd := strings.Index(line[clusterNameStart:], "\"")
			if clusterNameEnd == -1 {
				continue // skip lines with malformed cluster_name
			}
			clusterNameEnd += clusterNameStart
			clusterName := line[clusterNameStart:clusterNameEnd]

			// Initialize cluster data if not present
			if _, exists := clusterMetrics[clusterName]; !exists {
				clusterMetrics[clusterName] = map[string]interface{}{
					"buckets": make(map[string]int),
					"sum":     0.0,
					"count":   0,
				}
			}

			// Extract bucket values
			if strings.Contains(line, "_bucket") {
				leStart := strings.Index(line, "le=\"") + len("le=\"")
				leEnd := strings.Index(line[leStart:], "\"")
				if leStart == -1 || leEnd == -1 {
					continue // skip lines with malformed le values
				}
				leEnd += leStart
				le := line[leStart:leEnd]

				valueStart := strings.LastIndex(line, " ") + 1
				valueStr := line[valueStart:]
				var value int
				if _, err := fmt.Sscanf(valueStr, "%d", &value); err != nil {
					log.Printf("Failed to parse value: %s", valueStr)
					continue
				}

				clusterData := clusterMetrics[clusterName].(map[string]interface{})
				buckets := clusterData["buckets"].(map[string]int)
				buckets[le] = value
			}

			// Extract sum
			if strings.Contains(line, "_sum") {
				valueStart := strings.LastIndex(line, " ") + 1
				valueStr := line[valueStart:]
				var value float64
				if _, err := fmt.Sscanf(valueStr, "%f", &value); err != nil {
					log.Printf("Failed to parse sum value: %s", valueStr)
					continue
				}

				clusterData := clusterMetrics[clusterName].(map[string]interface{})
				clusterData["sum"] = value
			}

			// Extract count
			if strings.Contains(line, "_count") {
				valueStart := strings.LastIndex(line, " ") + 1
				valueStr := line[valueStart:]
				var value int
				if _, err := fmt.Sscanf(valueStr, "%d", &value); err != nil {
					log.Printf("Failed to parse count value: %s", valueStr)
					continue
				}

				clusterData := clusterMetrics[clusterName].(map[string]interface{})
				clusterData["count"] = value
			}
		}
	}

	return clusterMetrics, nil
}

func init() {
	r := router.V1()
	r.GET("/metrics", getMetrics)
}
