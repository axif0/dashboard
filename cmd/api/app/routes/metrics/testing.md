package metrics

import (
	"fmt"
	"log"
	"strings"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/karmada-io/dashboard/cmd/api/app/router"
	"github.com/karmada-io/dashboard/cmd/api/app/types/common"
	"github.com/karmada-io/dashboard/pkg/client"	
	"k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getMetrics(c *gin.Context) {
	appName := c.Param("app_name")
	podName := c.Param("pod_name")
	referenceName := c.Param("reference_name")  
	log.Printf("Requested metrics for app: %s, pod: %s, reference: %s", appName, podName, referenceName)

	kubeClient := client.InClusterClientForKarmadaApiServer()
	metrics, err := getMetricsFromPod(c, kubeClient, podName, referenceName)
	if err != nil {
		if strings.Contains(err.Error(), "pods not found") {
			availablePods, listErr := listAvailablePods(c, kubeClient)
			if listErr != nil {
				common.Fail(c, fmt.Errorf("pod not found and failed to list available pods: %v", listErr))
				return
			}
			c.JSON(http.StatusNotFound, gin.H{
				"error": err.Error(),
				"availablePods": availablePods,
			})
		} else if strings.Contains(err.Error(), "metric not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			common.Fail(c, err)
		}
		return
	}
	common.Success(c, metrics)
}

func getMetricsFromPod(c *gin.Context, kubeClient kubernetes.Interface, podName, referenceName string) (string, error) {
	pod, err := kubeClient.CoreV1().Pods("karmada-system").Get(c.Request.Context(), podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("pods \"%s\" not found", podName)
	}

	metricsURL := fmt.Sprintf("http://%s:8080/metrics", pod.Status.PodIP)
	log.Printf("Debugging: metricsURL=%s\n", metricsURL)

	result := kubeClient.CoreV1().RESTClient().Get().RequestURI(metricsURL).Do(c.Request.Context())
	metrics, err := result.Raw()
	if err != nil {
		log.Printf("Debugging: Error retrieving metrics: %v\n", err)
		return "", fmt.Errorf("error retrieving metrics: %v", err)
	}

	metricsStr := string(metrics)
	lines := strings.Split(metricsStr, "\n")
	var matchingLines []string
	for _, line := range lines {
		if strings.Contains(line, referenceName) {
			matchingLines = append(matchingLines, line)
		}
	}

	if len(matchingLines) == 0 {
		return "", fmt.Errorf("metric not found with reference name %s", referenceName)
	}

	return strings.Join(matchingLines, "\n"), nil
}

func listAvailablePods(c *gin.Context, kubeClient kubernetes.Interface) ([]string, error) {
	pods, err := kubeClient.CoreV1().Pods("karmada-system").List(c.Request.Context(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var podNames []string
	for _, pod := range pods.Items {
		podNames = append(podNames, pod.Name)
	}

	return podNames, nil
}

func init() {
	r := router.V1()
	r.GET("/metrics/:app_name/:pod_name/:reference_name", getMetrics)
}