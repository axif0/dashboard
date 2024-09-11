package metrics

const (
    createMainTableSQL = `
        CREATE TABLE IF NOT EXISTS %s (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT,
            help TEXT,
            type TEXT,
            currentTime DATETIME
        )
    `

    createValuesTableSQL = `
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
    `

    createTimeLoadTableSQL = `
        CREATE TABLE IF NOT EXISTS %s (
            time_entry DATETIME PRIMARY KEY
        )
    `

    insertTimeLoadSQL = `
        INSERT OR REPLACE INTO %s (time_entry) VALUES (?)
    `

    getOldestTimeSQL = `
        SELECT time_entry FROM %s
        ORDER BY time_entry DESC
        LIMIT 1 OFFSET 5
    `

    deleteOldTimeSQL = `DELETE FROM %s WHERE time_entry <= ?`

    deleteAssociatedMetricsSQL = `
        DELETE FROM %s WHERE currentTime <= ?
    `

    deleteAssociatedValuesSQL = `
        DELETE FROM %s_values WHERE metric_id NOT IN (SELECT id FROM %s)
    `

    insertMainSQL = `
        INSERT INTO %s (name, help, type, currentTime) 
        VALUES (?, ?, ?, ?)
    `

    insertValueSQL = `
        INSERT INTO %s_values (metric_id, controller_key, controller_value, le_key, le_value, value, measure)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `
)
