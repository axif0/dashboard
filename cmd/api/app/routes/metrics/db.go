package metrics

import (
    "fmt"
 
    "strings"
    "gorm.io/gorm"
    "gorm.io/driver/sqlite"
)

var dbMap = make(map[string]*gorm.DB)

func getDB(appName string) (*gorm.DB, error) {
    sanitizedAppName := strings.ReplaceAll(appName, "-", "_")
    dbPath := fmt.Sprintf("%s.db", sanitizedAppName)
    
    if db, exists := dbMap[dbPath]; exists {
        return db, nil
    }

    db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
    if err != nil {
        return nil, fmt.Errorf("failed to connect to database: %v", err)
    }

    // Auto Migrate the schemas
    err = db.AutoMigrate(&MetricEntry{}, &MetricValue{}, &MetricLabel{}, &TimeLoad{})
    if err != nil {
        return nil, fmt.Errorf("failed to migrate database: %v", err)
    }

    dbMap[dbPath] = db
    return db, nil
}