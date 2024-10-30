package metrics

import (
    "fmt"
    "net/http"
    "time"
    "github.com/gin-gonic/gin"
)

func queryMetrics(c *gin.Context) {
    appName := c.Param("app_name")
    podName := c.Param("pod_name")
    queryType := c.Query("type")
    metricName := c.Query("mname")

    db, err := getDB(appName)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Database error: %v", err)})
        return
    }

    switch queryType {
    case "mname":
        var metricNames []string
        err := db.Model(&MetricEntry{}).
            Where("app_name = ? AND pod_name = ?", appName, podName).
            Distinct().
            Pluck("name", &metricNames).Error
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query metric names"})
            return
        }
        c.JSON(http.StatusOK, gin.H{"metricNames": metricNames})

    case "details":
        if metricName == "" {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Metric name required for details"})
            return
        }

        var metrics []MetricEntry
        err := db.Preload("Values.Labels").
            Where("name = ? AND app_name = ? AND pod_name = ?", metricName, appName, podName).
            Find(&metrics).Error
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query metric details"})
            return
        }

        detailsMap := make(map[string]interface{})
        for _, metric := range metrics {
            timeKey := metric.CurrentTime.Format(time.RFC3339)
            values := make([]map[string]interface{}, 0)
            
            for _, value := range metric.Values {
                labels := make(map[string]string)
                for _, label := range value.Labels {
                    labels[label.Key] = label.Value
                }
                
                values = append(values, map[string]interface{}{
                    "value":   value.Value,
                    "measure": value.Measure,
                    "labels":  labels,
                })
            }

            detailsMap[timeKey] = map[string]interface{}{
                "help":   metric.Help,
                "type":   metric.Type,
                "values": values,
            }
        }

        c.JSON(http.StatusOK, gin.H{"details": detailsMap})
    }
}