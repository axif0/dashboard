// statements.go
package metrics

import (
    "database/sql"
    "fmt"
    "sync"
)

type PreparedStatements struct {
    // Metric related statements
    insertMetric *sql.Stmt
    insertValue  *sql.Stmt
    insertLabel  *sql.Stmt
    
    // Time management statements
    insertTimeLoad *sql.Stmt
    getOldestTime  *sql.Stmt
    
    // Cleanup statements
    deleteOldTime    *sql.Stmt
    deleteOldMetrics *sql.Stmt
    deleteOldValues  *sql.Stmt
    deleteOldLabels  *sql.Stmt
    
    // Query statements
    queryMetricNames    *sql.Stmt
    queryMetricDetails  *sql.Stmt
    queryMetricLabels   *sql.Stmt
}

var (
    stmtCache = make(map[string]*PreparedStatements)
    stmtMutex sync.RWMutex
)

// getStatements returns cached prepared statements or creates new ones
func getStatements(db *sql.DB, podName string) (*PreparedStatements, error) {
    stmtMutex.RLock()
    stmts, exists := stmtCache[podName]
    stmtMutex.RUnlock()
    
    if exists {
        return stmts, nil
    }

    stmtMutex.Lock()
    defer stmtMutex.Unlock()

    // Double-check after acquiring write lock
    if stmts, exists = stmtCache[podName]; exists {
        return stmts, nil
    }

    stmts, err := prepareStatements(db, podName)
    if err != nil {
        return nil, err
    }

    stmtCache[podName] = stmts
    return stmts, nil
}

func prepareStatements(db *sql.DB, podName string) (*PreparedStatements, error) {
    stmts := &PreparedStatements{}
    var err error

    // Create tables first
    if err = createTables(db, podName); err != nil {
        return nil, fmt.Errorf("failed to create tables: %w", err)
    }

    // Create indexes
    if err = createIndexes(db, podName); err != nil {
        return nil, fmt.Errorf("failed to create indexes: %w", err)
    }

    // Prepare insert statements
    stmts.insertMetric, err = db.Prepare(fmt.Sprintf(`
        INSERT INTO %s (name, help, type, currentTime) 
        VALUES (?, ?, ?, ?)
    `, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare insertMetric: %w", err)
    }

    stmts.insertValue, err = db.Prepare(fmt.Sprintf(`
        INSERT INTO %s_values (metric_id, value, measure) 
        VALUES (?, ?, ?)
    `, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare insertValue: %w", err)
    }

    stmts.insertLabel, err = db.Prepare(fmt.Sprintf(`
        INSERT INTO %s_labels (value_id, key, value) 
        VALUES (?, ?, ?)
    `, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare insertLabel: %w", err)
    }

    // Time management statements
    stmts.insertTimeLoad, err = db.Prepare(fmt.Sprintf(`
        INSERT OR REPLACE INTO %s_time_load (time_entry) 
        VALUES (?)
    `, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare insertTimeLoad: %w", err)
    }

    stmts.getOldestTime, err = db.Prepare(fmt.Sprintf(`
        SELECT time_entry FROM %s_time_load
        ORDER BY time_entry DESC
        LIMIT 1 OFFSET 5
    `, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare getOldestTime: %w", err)
    }

    // Cleanup statements
    stmts.deleteOldTime, err = db.Prepare(fmt.Sprintf(`
        DELETE FROM %s_time_load 
        WHERE time_entry <= ?
    `, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare deleteOldTime: %w", err)
    }

    stmts.deleteOldMetrics, err = db.Prepare(fmt.Sprintf(`
        DELETE FROM %s 
        WHERE currentTime <= ?
    `, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare deleteOldMetrics: %w", err)
    }

    stmts.deleteOldValues, err = db.Prepare(fmt.Sprintf(`
        DELETE FROM %s_values 
        WHERE metric_id NOT IN (SELECT id FROM %s)
    `, podName, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare deleteOldValues: %w", err)
    }

    stmts.deleteOldLabels, err = db.Prepare(fmt.Sprintf(`
        DELETE FROM %s_labels 
        WHERE value_id NOT IN (SELECT id FROM %s_values)
    `, podName, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare deleteOldLabels: %w", err)
    }

    // Query statements
    stmts.queryMetricNames, err = db.Prepare(fmt.Sprintf(`
        SELECT DISTINCT name 
        FROM %s
    `, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare queryMetricNames: %w", err)
    }

    stmts.queryMetricDetails, err = db.Prepare(fmt.Sprintf(`
        SELECT 
            m.currentTime, 
            m.help, 
            m.type, 
            v.value, 
            v.measure, 
            v.id
        FROM %s m
        INNER JOIN %s_values v ON m.id = v.metric_id
        WHERE m.name = ?
    `, podName, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare queryMetricDetails: %w", err)
    }

    stmts.queryMetricLabels, err = db.Prepare(fmt.Sprintf(`
        SELECT key, value 
        FROM %s_labels 
        WHERE value_id = ?
    `, podName))
    if err != nil {
        return nil, fmt.Errorf("failed to prepare queryMetricLabels: %w", err)
    }

    return stmts, nil
}

func createTables(db *sql.DB, podName string) error {
    queries := []string{
        fmt.Sprintf(`
            CREATE TABLE IF NOT EXISTS %s (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                name TEXT,
                help TEXT,
                type TEXT,
                currentTime DATETIME
            )
        `, podName),
        fmt.Sprintf(`
            CREATE TABLE IF NOT EXISTS %s_values (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                metric_id INTEGER,
                value TEXT,
                measure TEXT,
                FOREIGN KEY (metric_id) REFERENCES %s(id) ON DELETE CASCADE
            )
        `, podName, podName),
        fmt.Sprintf(`
            CREATE TABLE IF NOT EXISTS %s_labels (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                value_id INTEGER,
                key TEXT,
                value TEXT,
                FOREIGN KEY (value_id) REFERENCES %s_values(id) ON DELETE CASCADE
            )
        `, podName, podName),
        fmt.Sprintf(`
            CREATE TABLE IF NOT EXISTS %s_time_load (
                time_entry DATETIME PRIMARY KEY
            )
        `, podName),
    }

    for _, query := range queries {
        if _, err := db.Exec(query); err != nil {
            return fmt.Errorf("failed to execute query %s: %w", query, err)
        }
    }

    return nil
}

func createIndexes(db *sql.DB, podName string) error {
    queries := []string{
        fmt.Sprintf(`
            CREATE INDEX IF NOT EXISTS idx_%s_currenttime 
            ON %s(currentTime)
        `, podName, podName),
        fmt.Sprintf(`
            CREATE INDEX IF NOT EXISTS idx_%s_values_metric_id 
            ON %s_values(metric_id)
        `, podName, podName),
        fmt.Sprintf(`
            CREATE INDEX IF NOT EXISTS idx_%s_labels_value_id 
            ON %s_labels(value_id)
        `, podName, podName),
        fmt.Sprintf(`
            CREATE INDEX IF NOT EXISTS idx_%s_name 
            ON %s(name)
        `, podName, podName),
    }

    for _, query := range queries {
        if _, err := db.Exec(query); err != nil {
            return fmt.Errorf("failed to execute query %s: %w", query, err)
        }
    }

    return nil
}

// Close closes all prepared statements
func (s *PreparedStatements) Close() error {
    stmts := []*sql.Stmt{
        s.insertMetric,
        s.insertValue,
        s.insertLabel,
        s.insertTimeLoad,
        s.getOldestTime,
        s.deleteOldTime,
        s.deleteOldMetrics,
        s.deleteOldValues,
        s.deleteOldLabels,
        s.queryMetricNames,
        s.queryMetricDetails,
        s.queryMetricLabels,
    }

    for _, stmt := range stmts {
        if stmt != nil {
            if err := stmt.Close(); err != nil {
                return fmt.Errorf("failed to close statement: %w", err)
            }
        }
    }

    return nil
}

// CleanupStatements removes all prepared statements for a given podName
func CleanupStatements(podName string) error {
    stmtMutex.Lock()
    defer stmtMutex.Unlock()

    if stmts, exists := stmtCache[podName]; exists {
        if err := stmts.Close(); err != nil {
            return err
        }
        delete(stmtCache, podName)
    }

    return nil
}