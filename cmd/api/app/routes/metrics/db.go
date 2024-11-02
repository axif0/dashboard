package metrics

import (
    "database/sql"
    "sync"

	"time"
    _ "github.com/glebarez/sqlite"
)

var (
    dbConnections = make(map[string]*sql.DB)
    dbMutex      sync.RWMutex
)

func getDB(appName string) (*sql.DB, error) {
    dbMutex.RLock()
    db, exists := dbConnections[appName]
    dbMutex.RUnlock()
    
    if exists {
        return db, nil
    }

    dbMutex.Lock()
    defer dbMutex.Unlock()

    // Double-check after acquiring write lock
    if db, exists = dbConnections[appName]; exists {
        return db, nil
    }

    db, err := sql.Open("sqlite", appName+".db")
    if err != nil {
        return nil, err
    }

    // Configure connection pool
    db.SetMaxOpenConns(1)                // SQLite only supports one writer at a time
    db.SetMaxIdleConns(1)
    db.SetConnMaxLifetime(time.Hour)

    // Disable WAL mode, use rollback journal instead
    // if _, err := db.Exec("PRAGMA journal_mode=DELETE"); err != nil {
    //     db.Close()
    //     return nil, err
    // }

    // Enable WAL mode for better concurrent performance
    if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
        return nil, err
    }

    // Other PRAGMA settings for better performance
    pragmas := []string{
        "PRAGMA synchronous=NORMAL",
        "PRAGMA foreign_keys=ON",
        "PRAGMA cache_size=32768", // 32MB cache
        "PRAGMA temp_store=MEMORY",
    }

    for _, pragma := range pragmas {
        if _, err := db.Exec(pragma); err != nil {
            db.Close()
            return nil, err
        }
    }

    dbConnections[appName] = db
    return db, nil
}
  