package metrics

import (
 
    "encoding/json"
    "fmt"
    
    "strings"
    "time"
    "github.com/prometheus/common/expfmt"
    
    _ "github.com/glebarez/sqlite"
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

    db, err := getDB(sanitizedAppName)
    if err != nil {
        return fmt.Errorf("failed to get database connection: %w", err)
      
    }
    

    // Get cached prepared statements
    stmts, err := getStatements(db, sanitizedPodName)
    if err != nil {
        return fmt.Errorf("failed to get prepared statements: %w", err)
    }

    var data struct {
        CurrentTime string             `json:"currentTime"`
        Metrics     map[string]*Metric `json:"metrics"`
    }
    if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
        return fmt.Errorf("failed to unmarshal JSON: %w", err)
    }

    tx, err := db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    for metricName, metricData := range data.Metrics {
        result, err := tx.Stmt(stmts.insertMetric).Exec(
            metricName, 
            metricData.Help, 
            metricData.Type, 
            data.CurrentTime,
        )
        if err != nil {
            return fmt.Errorf("failed to insert metric %s: %w", metricName, err)
        }

        metricID, err := result.LastInsertId()
        if err != nil {
            return fmt.Errorf("failed to get last insert ID for metric %s: %w", metricName, err)
        }

        for _, value := range metricData.Values {
            valueResult, err := tx.Stmt(stmts.insertValue).Exec(
                metricID, 
                value.Value, 
                value.Measure,
            )
            if err != nil {
                return fmt.Errorf("failed to insert value: %w", err)
            }

            valueID, err := valueResult.LastInsertId()
            if err != nil {
                return fmt.Errorf("failed to get value ID: %w", err)
            }

            for labelKey, labelValue := range value.Labels {
                _, err = tx.Stmt(stmts.insertLabel).Exec(valueID, labelKey, labelValue)
                if err != nil {
                    return fmt.Errorf("failed to insert label: %w", err)
                }
            }
        }
    }

    if err = tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }

    return nil
}