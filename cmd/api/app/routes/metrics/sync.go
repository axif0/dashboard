package metrics

import(
	"time"
	"database/sql"
	"log"
	"sync"
	"context"
)

var (
    requests chan saveRequest
    db       *sql.DB
    syncMap  sync.Map
    // Add contexts and cancel functions for each app
    appContexts     map[string]context.Context
    appCancelFuncs  map[string]context.CancelFunc
    contextMutex    sync.Mutex
)


func goroutine(){
    // Initialize contexts and cancel functions
    appContexts = make(map[string]context.Context)
    appCancelFuncs = make(map[string]context.CancelFunc)
    
    appNames := []string{
        karmadaScheduler,
        karmadaControllerManager,
        karmadaAgent,
        karmadaSchedulerEstimator + "-member1",
        karmadaSchedulerEstimator + "-member2",
        karmadaSchedulerEstimator + "-member3",
    }

    // Create database connection
    var err error
    db, err = sql.Open("sqlite", "app_sync.db")
    if err != nil {
        log.Fatalf("Error opening app_sync database: %v", err)
    }

    // Create the app_sync table
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS app_sync (
            app_name TEXT PRIMARY KEY,
            sync_trigger INTEGER DEFAULT 1
        )
    `)
    if err != nil {
        log.Fatalf("Error creating app_sync table: %v", err)
    }

    // Initialize contexts for each app
    for _, appName := range appNames {
        ctx, cancel := context.WithCancel(context.Background())
        contextMutex.Lock()
        appContexts[appName] = ctx
        appCancelFuncs[appName] = cancel
        contextMutex.Unlock()

        _, err = db.Exec("INSERT OR IGNORE INTO app_sync (app_name) VALUES (?)", appName)
        if err != nil {
            log.Printf("Error inserting app name into app_sync table: %v", err)
            continue
        }
        
        syncMap.Store(appName, 1)
    }

    requests = make(chan saveRequest, len(appNames))
    go startDatabaseWorker(requests)
    go refreshSyncTriggers()

    // Start metrics fetchers with context
    for _, app := range appNames {
        go startAppMetricsFetcher(app)
    }
}

func startAppMetricsFetcher(appName string) {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for {
        contextMutex.Lock()
        ctx, exists := appContexts[appName]
        contextMutex.Unlock()

        if !exists {
            log.Printf("Context not found for %s, stopping fetcher", appName)
            return
        }

        select {
        case <-ctx.Done():
            log.Printf("Stopping metrics fetcher for %s", appName)
            return
        case <-ticker.C:
            syncTriggerVal, ok := syncMap.Load(appName)
            if !ok {
                continue
            }

            syncTrigger, ok := syncTriggerVal.(int)
            if !ok || syncTrigger != 1 {
                return
            }

            go func(ctx context.Context) {
                _, errors, err := fetchMetrics(ctx, appName, requests)
                if err != nil {
                    log.Printf("Error fetching metrics for %s: %v, errors: %v\n", appName, err, errors)
                }
            }(ctx)
        }
    }
}


func refreshSyncTriggers() {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        rows, err := db.Query("SELECT app_name, sync_trigger FROM app_sync")
        if err != nil {
            log.Printf("Error refreshing sync triggers: %v", err)
            continue
        }
 
        func() {
            defer func() {
                if err := rows.Close(); err != nil {
                    log.Printf("Error closing rows: %v", err)
                }
            }()

            for rows.Next() {
                var appName string
                var syncTrigger int
                if err := rows.Scan(&appName, &syncTrigger); err != nil {
                    log.Printf("Error scanning sync trigger row: %v", err)
                    continue
                }
                syncMap.Store(appName, syncTrigger)
            }

            if err := rows.Err(); err != nil {
                log.Printf("Error iterating over rows: %v", err)
            }
        }()
    }
}

