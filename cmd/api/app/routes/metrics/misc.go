package metrics

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "strings"
    "time"
    "github.com/prometheus/common/expfmt"
 
    _ "modernc.org/sqlite"
)

type Metric struct {
    Name   string        `json:"name"`
    Help   string        `json:"help"`
    Type   string        `json:"type"`
    Values []MetricValue `json:"values,omitempty"`
}

type MetricValue struct {
    Labels  map[string]string `json:"labels,omitempty"`
    Value   string            `json:"value"`
    Measure string            `json:"measure"`
} 

func parseMetricsToJSON(metricsOutput string) (string, error) {
    var parser expfmt.TextParser
    metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(metricsOutput))
    if err != nil {
        return "", fmt.Errorf("error parsing metrics: %w", err)
    }

    metrics := make(map[string]*Metric)

    for name, mf := range metricFamilies {
        m := &Metric{
            Name:   name,
            Help:   mf.GetHelp(),
            Type:   mf.GetType().String(),
            Values: []MetricValue{},
        }

        for _, metric := range mf.Metric {
            labels := make(map[string]string)
            for _, labelPair := range metric.Label {
                labels[labelPair.GetName()] = labelPair.GetValue()
            }

            if metric.Histogram != nil {
                for _, bucket := range metric.Histogram.Bucket {
                    bucketValue := fmt.Sprintf("%d", bucket.GetCumulativeCount()) 
                    bucketLabels := make(map[string]string)
                    for k, v := range labels {
                        bucketLabels[k] = v
                    }
                    bucketLabels["le"] = fmt.Sprintf("%f", bucket.GetUpperBound())
                    m.Values = append(m.Values, MetricValue{
                        Labels:  bucketLabels,
                        Value:   bucketValue,
                        Measure: "cumulative_count",
                    })
                }
                m.Values = append(m.Values, MetricValue{
                    Labels:  labels,
                    Value:   fmt.Sprintf("%f", metric.Histogram.GetSampleSum()),
                    Measure: "sum",
                })
                m.Values = append(m.Values, MetricValue{
                    Labels:  labels,
                    Value:   fmt.Sprintf("%d", metric.Histogram.GetSampleCount()),  
                    Measure: "count",
                })
            } else if metric.Counter != nil {
                value := fmt.Sprintf("%f", metric.Counter.GetValue())  
                m.Values = append(m.Values, MetricValue{
                    Labels:  labels,
                    Value:   value,
                    Measure: "total", 
                })
            } else if metric.Gauge != nil {
                value := fmt.Sprintf("%f", metric.Gauge.GetValue())
                m.Values = append(m.Values, MetricValue{
                    Labels:  labels,
                    Value:   value,
                    Measure: "current_value", 
                })
            } else {
                m.Values = append(m.Values, MetricValue{
                    Labels:  labels,
                    Value:   "",
                    Measure: "unhandled_metric_type",
                })
            }
        }

        metrics[name] = m
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

func saveToDB(appName, podName, jsonData string) error {
   
    sanitizedAppName := strings.ReplaceAll(appName, "-", "_")
    sanitizedPodName := strings.ReplaceAll(podName, "-", "_")

    db, err := sql.Open("sqlite", sanitizedAppName+".db")
    if err != nil {
        log.Printf("Error opening database: %v", err)
        return err
    }
    defer db.Close()

    var data struct {
        CurrentTime string             `json:"currentTime"`
        Metrics     map[string]*Metric `json:"metrics"`
    }
    if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
        log.Printf("Error unmarshaling JSON data: %v", err)
        return err
    }

    tx, err := db.Begin()
    if err != nil {
        log.Printf("Error starting transaction: %v", err)
        return err
    }
    defer tx.Rollback()

    createMainTableSQL := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT,
            help TEXT,
            type TEXT,
            currentTime DATETIME
        )
    `, sanitizedPodName)
    if _, err = tx.Exec(createMainTableSQL); err != nil {
        log.Printf("Error creating main table: %v", err)
        return err
    }

    createValuesTableSQL := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s_values (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            metric_id INTEGER,
            controller_key TEXT,
            controller_value TEXT,
            le_key TEXT,
            le_value TEXT,
            value TEXT,
            measure TEXT,
            FOREIGN KEY (metric_id) REFERENCES %s(id)
        )
    `, sanitizedPodName, sanitizedPodName)
    if _, err = tx.Exec(createValuesTableSQL); err != nil {
        log.Printf("Error creating values table: %v", err)
        return err
    }

    timeLoadTableName := fmt.Sprintf("%s_time_load", sanitizedPodName)
    createTimeLoadTableSQL := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s (
            time_entry DATETIME PRIMARY KEY
        )
    `, timeLoadTableName)
    if _, err = tx.Exec(createTimeLoadTableSQL); err != nil {
        log.Printf("Error creating %s table: %v", timeLoadTableName, err)
        return err
    }

    insertTimeLoadSQL := fmt.Sprintf(`
        INSERT OR REPLACE INTO %s (time_entry) VALUES (?)
    `, timeLoadTableName)
    if _, err = tx.Exec(insertTimeLoadSQL, data.CurrentTime); err != nil {
        log.Printf("Error inserting time entry: %v", err)
        return err
    }

    var oldestTime string
    getOldestTimeSQL := fmt.Sprintf(`
        SELECT time_entry FROM %s
        ORDER BY time_entry DESC
        LIMIT 1 OFFSET 5
    `, timeLoadTableName)
    err = tx.QueryRow(getOldestTimeSQL).Scan(&oldestTime)
    if err != nil && err != sql.ErrNoRows {
        log.Printf("Error getting oldest time entry: %v", err)
        return err
    }

    if oldestTime != "" {

        deleteOldTimeSQL := fmt.Sprintf(`DELETE FROM %s WHERE time_entry <= ?`, timeLoadTableName)
        result, err := tx.Exec(deleteOldTimeSQL, oldestTime)
        if err != nil {
            log.Printf("Error deleting old time entries: %v", err)
            return err
        }
        rowsAffected, _ := result.RowsAffected()
        log.Printf("Deleted %d old time entries from %s", rowsAffected, timeLoadTableName)

        deleteAssociatedMetricsSQL := fmt.Sprintf(`
            DELETE FROM %s WHERE currentTime <= ?
        `, sanitizedPodName)
        result, err = tx.Exec(deleteAssociatedMetricsSQL, oldestTime)
        if err != nil {
            log.Printf("Error deleting associated metrics: %v", err)
            return err
        }
        rowsAffected, _ = result.RowsAffected()
        log.Printf("Deleted %d associated metrics from %s", rowsAffected, sanitizedPodName)

        deleteAssociatedValuesSQL := fmt.Sprintf(`
            DELETE FROM %s_values WHERE metric_id NOT IN (SELECT id FROM %s)
        `, sanitizedPodName, sanitizedPodName)
        result, err = tx.Exec(deleteAssociatedValuesSQL)
        if err != nil {
            log.Printf("Error deleting associated values: %v", err)
            return err
        }
        rowsAffected, _ = result.RowsAffected()
        log.Printf("Deleted %d associated values from %s_values", rowsAffected, sanitizedPodName)
    }

    insertMainSQL := fmt.Sprintf(`
        INSERT INTO %s (name, help, type, currentTime) 
        VALUES (?, ?, ?, ?)
    `, sanitizedPodName)

    insertValueSQL := fmt.Sprintf(`
        INSERT INTO %s_values (metric_id, controller_key, controller_value, le_key, le_value, value, measure)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, sanitizedPodName)

    for metricName, metricData := range data.Metrics {

        result, err := tx.Exec(insertMainSQL, metricName, metricData.Help, metricData.Type, data.CurrentTime)
        if err != nil {
            log.Printf("Error inserting data for metric %s: %v", metricName, err)
            return err
        }

        metricID, err := result.LastInsertId()
        if err != nil {
            log.Printf("Error getting last insert ID for metric %s: %v", metricName, err)
            return err
        }

        for _, value := range metricData.Values {
            labelMap := make(map[string]string)
            for labelKey, labelValue := range value.Labels {
                labelMap[labelKey] = labelValue
            }
            
            controller := labelMap["controller"]
            le := labelMap["le"]
            
            _, err = tx.Exec(insertValueSQL, metricID, "controller", controller, "le", le, value.Value, value.Measure)
            if err != nil {
                log.Printf("Error inserting value for metric %s: %v", metricName, err)
                return err
            }
        }
    }

    if err = tx.Commit(); err != nil {
        log.Printf("Error committing transaction: %v", err)
        return err
    }

    log.Println("Data inserted successfully")
    return nil
}