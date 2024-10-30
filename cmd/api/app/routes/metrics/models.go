package metrics

// import (
//     "time"
//     "gorm.io/gorm"
// )

// type MetricEntry struct {
//     gorm.Model
//     Name        string
//     Help        string
//     Type        string
//     CurrentTime time.Time
//     Values      []MetricValue
//     AppName     string
//     PodName     string
// }

// type MetricValue struct {
//     gorm.Model
//     MetricEntryID uint
//     Value         string
//     Measure       string
//     Labels        []MetricLabel
// }

// type MetricLabel struct {
//     gorm.Model
//     MetricValueID uint
//     Key           string
//     Value         string
// }

// type TimeLoad struct {
//     gorm.Model
//     TimeEntry time.Time
//     AppName   string
//     PodName   string
// }