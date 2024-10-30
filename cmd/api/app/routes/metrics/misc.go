package metrics

import (
    "encoding/json"
    "fmt"
      "time"
    "gorm.io/gorm"
    "strings"

    
    "github.com/prometheus/common/expfmt"
)
type MetricEntry struct {
    gorm.Model
    Name        string
    Help        string
    Type        string
    CurrentTime time.Time
    Values      []MetricValue
    AppName     string
    PodName     string
}

type MetricValue struct {
    gorm.Model
    MetricEntryID uint
    Value         string
    Measure       string
    Labels        []MetricLabel
}

type MetricLabel struct {
    gorm.Model
    MetricValueID uint
    Key           string
    Value         string
}

type TimeLoad struct {
    gorm.Model
    TimeEntry time.Time
    AppName   string
    PodName   string
}
type Metric struct {
    Name   string        `json:"name"`
    Help   string        `json:"help"`
    Type   string        `json:"type"`
    Values []MetricValue `json:"values,omitempty"`
}

// type MetricValue struct {
//     Labels  map[string]string `json:"labels,omitempty"`
//     Value   string            `json:"value"`
//     Measure string            `json:"measure"`
// } 

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
            labels := make([]MetricLabel, 0)
            for _, labelPair := range metric.Label {
                labels = append(labels, MetricLabel{
                    Key:   labelPair.GetName(),
                    Value: labelPair.GetValue(),
                })
            }

            if metric.Histogram != nil {
                for _, bucket := range metric.Histogram.Bucket {
                    bucketLabels := make([]MetricLabel, len(labels))
                    copy(bucketLabels, labels)
                    bucketLabels = append(bucketLabels, MetricLabel{
                        Key:   "le",
                        Value: fmt.Sprintf("%f", bucket.GetUpperBound()),
                    })
                    
                    m.Values = append(m.Values, MetricValue{
                        Labels:  bucketLabels,
                        Value:   fmt.Sprintf("%d", bucket.GetCumulativeCount()),
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
    db, err := getDB(appName)
    if err != nil {
        return err
    }

    var data struct {
        CurrentTime string             `json:"currentTime"`
        Metrics     map[string]*Metric `json:"metrics"`
    }
    if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
        return fmt.Errorf("error unmarshaling JSON data: %v", err)
    }

    currentTime, err := time.Parse(time.RFC3339, data.CurrentTime)
    if err != nil {
        return fmt.Errorf("error parsing time: %v", err)
    }

    // Begin transaction
    tx := db.Begin()
    if tx.Error != nil {
        return tx.Error
    }
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
        }
    }()

    // Save time load
    timeLoad := TimeLoad{
        TimeEntry: currentTime,
        AppName:   appName,
        PodName:   podName,
    }
    if err := tx.Create(&timeLoad).Error; err != nil {
        tx.Rollback()
        return err
    }

    // Delete old records
    var oldestTime TimeLoad
    if err := tx.Where("app_name = ? AND pod_name = ?", appName, podName).
        Order("time_entry desc").
        Offset(5).
        First(&oldestTime).Error; err == nil {
        // Delete old metrics
        if err := tx.Where("current_time <= ? AND app_name = ? AND pod_name = ?", 
            oldestTime.TimeEntry, appName, podName).
            Delete(&MetricEntry{}).Error; err != nil {
            tx.Rollback()
            return err
        }
    }

    // Save new metrics
    for metricName, metricData := range data.Metrics {
        metricEntry := MetricEntry{
            Name:        metricName,
            Help:        metricData.Help,
            Type:        metricData.Type,
            CurrentTime: currentTime,
            AppName:     appName,
            PodName:     podName,
        }

        if err := tx.Create(&metricEntry).Error; err != nil {
            tx.Rollback()
            return err
        }

        for _, value := range metricData.Values {
            metricValue := MetricValue{
                MetricEntryID: metricEntry.ID,
                Value:         value.Value,
                Measure:       value.Measure,
            }

            if err := tx.Create(&metricValue).Error; err != nil {
                tx.Rollback()
                return err
            }

            for _, labelValue := range value.Labels {
                label := MetricLabel{
                    MetricValueID: metricValue.ID,
                    Key:   labelValue.Key,
                    Value: labelValue.Value,
                }

                if err := tx.Create(&label).Error; err != nil {
                    tx.Rollback()
                    return err
                }
            }
        }
    }

    return tx.Commit().Error
}

