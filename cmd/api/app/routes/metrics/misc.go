package metrics

import(
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
 
	_ "modernc.org/sqlite"
)

type Metric struct {
	Name   string            `json:"name"`
	Help   string            `json:"help"`
	Type   string            `json:"type"`
	Values []MetricValue     `json:"values,omitempty"`
}

type MetricValue struct {
	Labels map[string]string `json:"labels,omitempty"`
	Value  string             `json:"value"`
	Measure string `json:"measure"`
} 

func parseMetricsToJSON(metricsOutput string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(metricsOutput))
	metrics := make(map[string]*Metric)
	var currentMetric *Metric

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# HELP") {
			parts := strings.SplitN(line, " ", 4)
			if len(parts) >= 4 {
				currentMetric = &Metric{
					Name: parts[2],
					Help: parts[3],
				}
				metrics[currentMetric.Name] = currentMetric
			}
		} else if strings.HasPrefix(line, "# TYPE") {
			parts := strings.SplitN(line, " ", 4)
			if len(parts) >= 4 && currentMetric != nil {
				currentMetric.Type = parts[3]
			}
		} else if !strings.HasPrefix(line, "#") && currentMetric != nil {
			parts := strings.SplitN(line, " ", 2)
		 
			if len(parts) == 2 {
				labelsPart := strings.SplitN(parts[0], "{", 2)
				var labels map[string]string
				if len(labelsPart) > 1 {
					labels = parseLabels(labelsPart[1])
				}
				measureParts := strings.Split(labelsPart[0], "_")
				measureTerm := measureParts[len(measureParts)-1]  
				currentMetric.Values = append(currentMetric.Values, MetricValue{
					Labels: labels,
					Value:  parts[1],
					Measure: measureTerm, // Use the dynamically extracted term
				})
			}
		}
	}


	
	currentTime := time.Now().Format(time.RFC3339)
	output := map[string]interface{}{
		"metrics":     metrics,
		"currentTime": currentTime,
	}


	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

func parseLabels(labelsString string) map[string]string {
	labels := make(map[string]string)
	labelsString = strings.TrimRight(labelsString, "}")
	pairs := strings.Split(labelsString, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.Trim(strings.TrimSpace(kv[1]), "\"")
			labels[key] = value
		}
	}
	return labels
}

func saveToDB(appName, podName,   jsonData string) error {
    // Sanitize the identifiers for use in SQL
    sanitizedAppName := strings.ReplaceAll(appName, "-", "_")
    sanitizedPodName := strings.ReplaceAll(podName, "-", "_")

    // Open the SQLite database
    db, err := sql.Open("sqlite", sanitizedAppName+".db")
    if err != nil {
        log.Printf("Error opening database: %v", err)
        return err
    }
    defer db.Close()

    // Parse the jsonData
    var data struct {
        CurrentTime string             `json:"currentTime"`
        Metrics     map[string]*Metric `json:"metrics"`
    }
    if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
        log.Printf("Error unmarshaling JSON data: %v", err)
        return err
    }

    // Start a transaction
    tx, err := db.Begin()
    if err != nil {
        log.Printf("Error starting transaction: %v", err)
        return err
    }
    defer tx.Rollback()

    // Create tables if not exists
    createTableSQL := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s (
            name TEXT,
            help TEXT,
            type TEXT,
            value_data TEXT,
            currentTime DATETIME
        )
    `, sanitizedPodName)
    if _, err = tx.Exec(createTableSQL); err != nil {
        log.Printf("Error creating table: %v", err)
        return err
    }

    createTimeLoadTableSQL := `
        CREATE TABLE IF NOT EXISTS time_load (
            time_entry DATETIME PRIMARY KEY
        )
    `
    if _, err = tx.Exec(createTimeLoadTableSQL); err != nil {
        log.Printf("Error creating time_load table: %v", err)
        return err
    }

    // Insert new time entry
    insertTimeLoadSQL := `
        INSERT OR REPLACE INTO time_load (time_entry) VALUES (?)
    `
    if _, err = tx.Exec(insertTimeLoadSQL, data.CurrentTime); err != nil {
        log.Printf("Error inserting time entry: %v", err)
        return err
    }

    // Get the oldest time entry if there are more than 5
    var oldestTime string
    getOldestTimeSQL := `
        SELECT time_entry FROM time_load
        ORDER BY time_entry DESC
        LIMIT 1 OFFSET 5
    `
    err = tx.QueryRow(getOldestTimeSQL).Scan(&oldestTime)
    if err != nil && err != sql.ErrNoRows {
        log.Printf("Error getting oldest time entry: %v", err)
        return err
    }

    // If we have more than 5 entries, delete the oldest ones and their associated metrics
    if oldestTime != "" {
        // Delete old time entries
        deleteOldTimeSQL := `DELETE FROM time_load WHERE time_entry <= ?`
        result, err := tx.Exec(deleteOldTimeSQL, oldestTime)
        if err != nil {
            log.Printf("Error deleting old time entries: %v", err)
            return err
        }
        rowsAffected, _ := result.RowsAffected()
        log.Printf("Deleted %d old time entries", rowsAffected)

        // Delete associated metrics
        deleteAssociatedMetricsSQL := fmt.Sprintf(`
            DELETE FROM %s WHERE currentTime <= ?
        `, sanitizedPodName)
        result, err = tx.Exec(deleteAssociatedMetricsSQL, oldestTime)
        if err != nil {
            log.Printf("Error deleting associated metrics: %v", err)
            return err
        }
        rowsAffected, _ = result.RowsAffected()
        log.Printf("Deleted %d associated metrics", rowsAffected)
    }

    // Insert new metrics data
    insertSQL := fmt.Sprintf(`
        INSERT INTO %s (name, help, type, value_data, currentTime) 
        VALUES (?, ?, ?, ?, ?)
    `, sanitizedPodName)

    for metricName, metricData := range data.Metrics {
        // Marshal the entire values array to JSON
        valuesJSON, err := json.Marshal(metricData.Values)
        if err != nil {
            log.Printf("Error marshaling values for metric %s: %v", metricName, err)
            continue
        }

        _, err = tx.Exec(insertSQL, metricName, metricData.Help, metricData.Type, string(valuesJSON), data.CurrentTime)
        if err != nil {
            log.Printf("Error inserting data for metric %s: %v", metricName, err)
            return err
        }
    }

    // Commit the transaction
    if err = tx.Commit(); err != nil {
        log.Printf("Error committing transaction: %v", err)
        return err
    }

    log.Println("Data inserted successfully")
    return nil
}