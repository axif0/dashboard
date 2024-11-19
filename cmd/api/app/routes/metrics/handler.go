package metrics

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"encoding/json"
 
	v1 "github.com/karmada-io/dashboard/cmd/api/app/types/api/v1"
	"github.com/gin-gonic/gin"
	"github.com/karmada-io/dashboard/cmd/api/app/router"
	"github.com/karmada-io/dashboard/pkg/client"
	"github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"  
	"sync"
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

func fetchMetrics(appName string, requests chan saveRequest) (map[string]*v1.ParsedData, []string, error) {
    kubeClient := client.InClusterClient()
    podsMap, errors := getKarmadaPods(appName)
    if len(podsMap) == 0 && len(errors) > 0 {
        return nil, errors, fmt.Errorf("no pods found")
    }
    allMetrics := make(map[string]*v1.ParsedData)
    var mu sync.Mutex
    var wg sync.WaitGroup
    for clusterName, pods := range podsMap {
        for _, pod := range pods {
            wg.Add(1)
            go func(pod v1.PodInfo, clusterName string) {
                defer wg.Done()
                var jsonMetrics *v1.ParsedData
                var err error
                if appName == karmadaAgent {
                    jsonMetrics, err = getKarmadaAgentMetrics(pod.Name, clusterName, requests)
                    if err != nil {
                        mu.Lock()
                        errors = append(errors, err.Error())
                        mu.Unlock()
                        return
                    }
                    mu.Lock()
                    allMetrics[pod.Name] = jsonMetrics
                    mu.Unlock()
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
                        mu.Lock()
                        errors = append(errors, err.Error())
                        mu.Unlock()
                        return
                    }
                    jsonMetrics, err = parseMetricsToJSON(string(metricsOutput))
                    if err != nil {
                        mu.Lock()
                        errors = append(errors, "Failed to parse metrics to JSON")
                        mu.Unlock()
                        return
                    }
                    // Send save request without waiting
                    requests <- saveRequest{
                        appName: appName,
                        podName: pod.Name,
                        data:    jsonMetrics,
                        result:  nil, // Not waiting for result
                    }
                    mu.Lock()
                    allMetrics[pod.Name] = jsonMetrics
                    mu.Unlock()
                }
            }(pod, clusterName)
        }
    }
    wg.Wait()
    return allMetrics, errors, nil
}

func getMetrics(c *gin.Context) {
    appName := c.Param("app_name")
    queryType := c.Query("type")

    if queryType == "sync_on" || queryType == "sync_off" {
        syncValue := 0
        if queryType == "sync_on" {
            syncValue = 1
        }

        if appName == "" {
            // Stop all apps
            _, err := db.Exec("UPDATE app_sync SET sync_trigger = ?", syncValue)
            if err != nil {
                c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update sync_trigger for all apps: %v", err)})
                return
            }

            // Cancel all existing contexts and create new ones if turning on
            contextMutex.Lock()
            for app := range appContexts {
                currentSyncValue, _ := syncMap.Load(app)
                if currentSyncValue == syncValue {
                    continue // Skip if already in the desired state
                }

                if cancel, exists := appCancelFuncs[app]; exists {
                    cancel() // Cancel existing context
                }
                
                if syncValue == 1 {
                    // Create new context if turning on
                    ctx, cancel := context.WithCancel(context.Background())
                    appContexts[app] = ctx
                    appCancelFuncs[app] = cancel
                    go startAppMetricsFetcher(app)
                }
                
                syncMap.Store(app, syncValue)
            }
            contextMutex.Unlock()

            message := "Sync trigger updated successfully for all apps"
            if syncValue == 1 {
                message = "Sync turned on successfully for all apps"
            } else {
                message = "Sync turned off successfully for all apps"
            }
            c.JSON(http.StatusOK, gin.H{"message": message})
        } else {
            // Update specific app
            currentSyncValue, _ := syncMap.Load(appName)
            if currentSyncValue == syncValue {
                message := fmt.Sprintf("Sync is already %s for %s", queryType, appName)
                c.JSON(http.StatusOK, gin.H{"message": message})
                return
            }

            _, err := db.Exec("UPDATE app_sync SET sync_trigger = ? WHERE app_name = ?", syncValue, appName)
            if err != nil {
                c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update sync_trigger: %v", err)})
                return
            }

            contextMutex.Lock()
            if cancel, exists := appCancelFuncs[appName]; exists {
                cancel() // Cancel existing context
            }
            
            if syncValue == 1 {
                // Create new context if turning on
                ctx, cancel := context.WithCancel(context.Background())
                appContexts[appName] = ctx
                appCancelFuncs[appName] = cancel
                go startAppMetricsFetcher(appName)
            }
            
            syncMap.Store(appName, syncValue)
            contextMutex.Unlock()

            var message string
            if syncValue == 1 {
                message = fmt.Sprintf("Sync turned on successfully for %s", appName)
            } else {
                message = fmt.Sprintf("Sync turned off successfully for %s", appName)
            }
            c.JSON(http.StatusOK, gin.H{"message": message})
        }

        return
    }

    if queryType == "metricsdetails" {
        queryMetrics(c)
        return
    }

    if queryType == "sync_status" {
        statusMap := make(map[string]bool)
        
        // Get status for all registered apps
        for _, app := range []string{
            karmadaScheduler,
            karmadaControllerManager,
            karmadaAgent,
            karmadaSchedulerEstimator + "-member1",
            karmadaSchedulerEstimator + "-member2",
            karmadaSchedulerEstimator + "-member3",
        } {
            syncValue, exists := syncMap.Load(app)
            if !exists {
                statusMap[app] = false
                continue
            }
            
            if value, ok := syncValue.(int); ok {
                statusMap[app] = value == 1
            } else {
                statusMap[app] = false
            }
        }
        
        c.JSON(http.StatusOK, statusMap)
        return
    }

    allMetrics, errors, err := fetchMetrics(appName, requests)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"errors": errors, "error": err.Error()})
        return
    }
    if len(allMetrics) > 0 {
        c.JSON(http.StatusOK, allMetrics)
    } else {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "No metrics data found", "errors": errors})
    }
}


func getKarmadaAgentMetrics(podName string, clusterName string, requests chan saveRequest) (*v1.ParsedData, error) {
	kubeClient := client.InClusterKarmadaClient()
	clusters, err := kubeClient.ClusterV1alpha1().Clusters().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %v", err)
	}

	for _, cluster := range clusters.Items {
		if strings.EqualFold(string(cluster.Spec.SyncMode), "Pull") {
			clusterName = cluster.Name
			break
		}
	}

	if clusterName=="" {
		return nil, fmt.Errorf("no cluster in 'Pull' mode found")
	}

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
		return nil, fmt.Errorf("failed to build config for cluster %s: %v", clusterName, err)
	}

	config.Host = fmt.Sprintf("%s/apis/cluster.karmada.io/v1alpha1/clusters/%s/proxy", config.Host, clusterName)

	restClient, err := kubeclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client for cluster %s: %v", clusterName, err)
	}

	metricsOutput,err := restClient.CoreV1().RESTClient().Get().
		Namespace("karmada-system").
		Resource("pods").
		SubResource("proxy").
		Name(fmt.Sprintf("%s:8080", podName)).
		Suffix("metrics").
		Do(context.TODO()).Raw()

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve metrics: %v", err)
	}
	var parsedData *v1.ParsedData
    if isJSON(metricsOutput) {
        parsedData = &v1.ParsedData{}
        err = json.Unmarshal(metricsOutput, parsedData)
        if err != nil {
            return nil, fmt.Errorf("failed to unmarshal JSON metrics: %v", err)
        }
    } else {
        var parsedDataPtr *v1.ParsedData
        parsedDataPtr, err = parseMetricsToJSON(string(metricsOutput))
        if err != nil {
            return nil, fmt.Errorf("failed to parse metrics to JSON: %v", err)
        }
		parsedData = parsedDataPtr

    }
	
	// Send save request to the database worker
    // resultChan := make(chan error)
    requests <- saveRequest{
        appName: karmadaAgent,
        podName: podName,
        data:    parsedData,
        result:  nil, // Not waiting for result
    }
	return parsedData, nil
}

func isJSON(data []byte) bool {
    var js json.RawMessage
    return json.Unmarshal(data, &js) == nil
}

func getClusterPods(cluster *v1alpha1.Cluster) ([]v1.PodInfo, error) {
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

	var podInfos []v1.PodInfo
	for _, pod := range podList.Items {
		podInfos = append(podInfos, v1.PodInfo{
			Name: pod.Name,
		})
	}

	return podInfos, nil
}

func getKarmadaPods(appName string) (map[string][]v1.PodInfo, []string) {
	kubeClient := client.InClusterClient()

	podsMap := make(map[string][]v1.PodInfo)
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
			podsMap[appName] = append(podsMap[appName], v1.PodInfo{Name: pod.Name})
		}
	}

	return podsMap, errors
}
 
 func init() {  
    goroutine()
    // Initialize the router with modified endpoints
        r := router.V1()
        r.GET("/metrics", getMetrics)   
        r.GET("/metrics/:app_name", getMetrics)
        r.GET("/metrics/:app_name/:pod_name", queryMetrics)
    }


    // http://localhost:8000/api/v1/metrics/karmada-scheduler  //from terminal
	
	// http://localhost:8000/api/v1/metrics/karmada-scheduler?type=metricsdetails  //from sqlite details bar
	
	// http://localhost:8000/api/v1/metrics/karmada-scheduler/karmada-scheduler-7bd4659f9f-hh44f?type=details&mname=workqueue_queue_duration_seconds

	// http://localhost:8000/api/v1/metrics?type=sync_off // to skip all metrics

    // http://localhost:8000/api/v1/metrics/karmada-scheduler?type=sync_off // to skip specific metrics
