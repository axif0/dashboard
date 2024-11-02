package metrics

import (
	"log"
	"net/http"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
)

func queryMetrics(c *gin.Context) {
	appName := c.Param("app_name")
	podName := c.Param("pod_name")
	queryType := c.Query("type")  // Use a query parameter to determine the action
	metricName := c.Query("mname")  // Optional: only needed for details

	sanitizedAppName := strings.ReplaceAll(appName, "-", "_")
	sanitizedPodName := strings.ReplaceAll(podName, "-", "_")

	db, err := getDB(sanitizedAppName)
	if err != nil {
		log.Printf("Error getting database: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get database"})
		return
	}
	defer db.Close()

	stmts, err := getStatements(db, sanitizedPodName)
	if err != nil {
		log.Printf("Error getting prepared statements: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get prepared statements"})
		return
	}

	switch queryType {
	case "mname":
		rows, err := stmts.queryMetricNames.Query()
		if err != nil {
			log.Printf("Error querying metric names: %v, SQL Error: %v", err, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query metric names"})
			return
		}
		defer rows.Close()

		var metricNames []string
		for rows.Next() {
			var metricName string
			if err := rows.Scan(&metricName); err != nil {
				log.Printf("Error scanning metric name: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan metric name"})
				return
			}
			metricNames = append(metricNames, metricName)
		}

		c.JSON(http.StatusOK, gin.H{ "metricNames": metricNames})
	case "tables":
		rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
		if err != nil {
			log.Printf("Error querying tables: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query tables"})
			return
		}
		defer rows.Close()

		var tables []string
		for rows.Next() {
			var tableName string
			if err := rows.Scan(&tableName); err != nil {
				log.Printf("Error scanning table name: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan table name"})
				return
			}
			tables = append(tables, tableName)
		}

		c.JSON(http.StatusOK, gin.H{ "tables": tables})
	case "details":
		tx, err := db.Begin()
		if err != nil {
			log.Printf("Error starting transaction: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Metric name required for details"})
			return
		}
		defer tx.Rollback()

		rows, err := tx.Stmt(stmts.queryMetricDetails).Query(metricName)
		if err != nil {
			log.Printf("Error querying metric details: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query metric details"})
			return
		}
		defer rows.Close()

		type MetricValue struct {
			Value   string            `json:"value"`
			Measure string            `json:"measure"`
			Labels  map[string]string `json:"labels"`
		}

		type MetricDetails struct {
			Help   string        `json:"help"`
			Type   string        `json:"type"`
			Values []MetricValue `json:"values"`
		}

		detailsMap := make(map[string]MetricDetails)

		for rows.Next() {
			var currentTime time.Time
			var help, mType, value, measure string
			var valueID int
			if err := rows.Scan(&currentTime, &help, &mType, &value, &measure, &valueID); err != nil {
				log.Printf("Error scanning metric details: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan metric details"})
				return
			}

			labelsRows, err := tx.Stmt(stmts.queryMetricLabels).Query(valueID)
			if err != nil {
				log.Printf("Error querying labels: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query labels"})
				return
			}
			defer labelsRows.Close()

			labels := make(map[string]string)
			for labelsRows.Next() {
				var labelKey, labelValue string
				if err := labelsRows.Scan(&labelKey, &labelValue); err != nil {
					log.Printf("Error scanning labels: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan labels"})
					return
				}
				labels[labelKey] = labelValue
			}

			timeKey := currentTime.Format(time.RFC3339)

			detail, exists := detailsMap[timeKey]
			if !exists {
				detail = MetricDetails{
					Help:   help,
					Type:   mType,
					Values: []MetricValue{},
				}
			}

			detail.Values = append(detail.Values, MetricValue{
				Value:   value,
				Measure: measure,
				Labels:  labels,
			})

			detailsMap[timeKey] = detail
		}

		if err := tx.Commit(); err != nil {
			log.Printf("Error committing transaction: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"details": detailsMap})
	}
}