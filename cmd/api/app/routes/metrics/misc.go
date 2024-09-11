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

    // Create tables
    if _, err = tx.Exec(fmt.Sprintf(createMainTableSQL, sanitizedPodName)); err != nil {
        log.Printf("Error creating main table: %v", err)
        return err
    }

    if _, err = tx.Exec(fmt.Sprintf(createValuesTableSQL, sanitizedPodName, sanitizedPodName)); err != nil {
        log.Printf("Error creating values table: %v", err)
        return err
    }

    timeLoadTableName := fmt.Sprintf("%s_time_load", sanitizedPodName)
    if _, err = tx.Exec(fmt.Sprintf(createTimeLoadTableSQL, timeLoadTableName)); err != nil {
        log.Printf("Error creating %s table: %v", timeLoadTableName, err)
        return err
    }

    // Insert time load
    if _, err = tx.Exec(fmt.Sprintf(insertTimeLoadSQL, timeLoadTableName), data.CurrentTime); err != nil {
        log.Printf("Error inserting time entry: %v", err)
        return err
    }

    // Get oldest time and delete old data
    var oldestTime string
    err = tx.QueryRow(fmt.Sprintf(getOldestTimeSQL, timeLoadTableName)).Scan(&oldestTime)
    if err != nil && err != sql.ErrNoRows {
        log.Printf("Error getting oldest time entry: %v", err)
        return err
    }

    if oldestTime != "" {
        result, err := tx.Exec(fmt.Sprintf(deleteOldTimeSQL, timeLoadTableName), oldestTime)
        if err != nil {
            log.Printf("Error deleting old time entries: %v", err)
            return err
        }
        rowsAffected, _ := result.RowsAffected()
        log.Printf("Deleted %d old time entries from %s", rowsAffected, timeLoadTableName)

        result, err = tx.Exec(fmt.Sprintf(deleteAssociatedMetricsSQL, sanitizedPodName), oldestTime)
        if err != nil {
            log.Printf("Error deleting associated metrics: %v", err)
            return err
        }
        rowsAffected, _ = result.RowsAffected()
        log.Printf("Deleted %d associated metrics from %s", rowsAffected, sanitizedPodName)

        result, err = tx.Exec(fmt.Sprintf(deleteAssociatedValuesSQL, sanitizedPodName, sanitizedPodName))
        if err != nil {
            log.Printf("Error deleting associated values: %v", err)
            return err
        }
        rowsAffected, _ = result.RowsAffected()
        log.Printf("Deleted %d associated values from %s_values", rowsAffected, sanitizedPodName)
    }

    // Insert metrics and values
    for metricName, metricData := range data.Metrics {
        result, err := tx.Exec(fmt.Sprintf(insertMainSQL, sanitizedPodName), metricName, metricData.Help, metricData.Type, data.CurrentTime)
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
            
            _, err = tx.Exec(fmt.Sprintf(insertValueSQL, sanitizedPodName), metricID, "controller", controller, "le", le, value.Value, value.Measure)
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