package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/benbjohnson/litestream"
	lss3 "github.com/benbjohnson/litestream/s3"
	"github.com/fsnotify/fsnotify"
	_ "github.com/mattn/go-sqlite3"
)

// addr is the bind address for the web server.
// addr will be set based on the port flag

// DatabaseManager gerencia múltiplas instâncias do Litestream
type DatabaseManager struct {
	databases   map[string]*litestream.DB
	configs     map[string]*DatabaseConfig
	watcher     *fsnotify.Watcher
	mutex       sync.RWMutex
	bucket      string
	watchDirs   []string
	ctx         context.Context
	cancel      context.CancelFunc
}

// DatabaseConfig configuração por banco/cliente
type DatabaseConfig struct {
	ClientID     string    `json:"clientId"`
	DatabasePath string    `json:"databasePath"`
	S3Path       string    `json:"s3Path"`
	CreatedAt    time.Time `json:"createdAt"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	// Parse command line flags.
	watchDir := flag.String("watch-dir", "", "directory to watch for GUID.db files (comma-separated for multiple)")
	bucket := flag.String("bucket", "", "s3 replica bucket")
	port := flag.String("port", "8080", "port for the web server (default: 8080)")
	

	
	flag.Parse()
	
	// Set address based on port flag
	addr := ":" + *port

	// Validate required parameters
	if *bucket == "" {
		flag.Usage()
		return fmt.Errorf("required: -bucket NAME")
	}
	
	if *watchDir == "" {
		flag.Usage()
		return fmt.Errorf("required: -watch-dir PATH")
	}

	// Run directory watching mode
	return runDirectoryMode(ctx, *watchDir, *bucket, addr)
}

// runDirectoryMode runs the new multi-database directory watching mode
func runDirectoryMode(ctx context.Context, watchDirStr, bucket, addr string) error {
	watchDirs := strings.Split(watchDirStr, ",")
	
	// Trim spaces
	for i, dir := range watchDirs {
		watchDirs[i] = strings.TrimSpace(dir)
	}

	fmt.Println("🏢 Litestream Multi-Client Manager (Directory Mode)")
	fmt.Println("===============================================")
	fmt.Printf("📦 S3 Bucket: %s\n", bucket)
	fmt.Printf("👀 Watching Directories: %v\n", watchDirs)
	fmt.Printf("🌐 Status Server: http://localhost%s\n", addr)
	fmt.Println()

	// Create and start database manager
	dm := NewDatabaseManager(bucket, watchDirs)
	defer dm.Stop()

	if err := dm.Start(); err != nil {
		return fmt.Errorf("failed to start database manager: %w", err)
	}

	// Start status web server
	go startStatusServer(dm, addr)

	// Wait for signal
	<-ctx.Done()
	log.Print("litestream manager received signal, shutting down")
	return nil
}



// extractClientID extracts GUID from database filename for S3 organization
// Expected format: /data/12345678-1234-5678-9abc-123456789012.db
func extractClientID(dbPath string) string {
	// Extract filename from path
	base := filepath.Base(dbPath)
	guid := strings.TrimSuffix(base, filepath.Ext(base))
	
	// Validate GUID format
	if isValidGUID(guid) {
		return guid
	}
	
	// Return empty string for invalid GUIDs - will be ignored
	return ""
}

// isValidGUID validates if string follows GUID pattern
func isValidGUID(s string) bool {
	// Basic GUID validation: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	if len(s) != 36 {
		return false
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	return true
}

// NewDatabaseManager cria novo gerenciador de bancos
func NewDatabaseManager(bucket string, watchDirs []string) *DatabaseManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Failed to create file watcher:", err)
	}

	return &DatabaseManager{
		databases: make(map[string]*litestream.DB),
		configs:   make(map[string]*DatabaseConfig),
		watcher:   watcher,
		bucket:    bucket,
		watchDirs: watchDirs,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start inicia o monitoramento de diretórios
func (dm *DatabaseManager) Start() error {
	// Adiciona diretórios para monitoramento
	for _, dir := range dm.watchDirs {
		if err := dm.addWatchDir(dir); err != nil {
			log.Printf("❌ Failed to watch directory %s: %v", dir, err)
			continue
		}
		log.Printf("👀 Watching directory: %s", dir)
	}

	// Inicia goroutine de monitoramento
	go dm.watchFiles()
	
	// Escaneia arquivos existentes
	return dm.scanExistingDatabases()
}

// Stop para o gerenciador
func (dm *DatabaseManager) Stop() {
	dm.cancel()
	dm.watcher.Close()
	
	dm.mutex.Lock()
	defer dm.mutex.Unlock()
	
	for path, db := range dm.databases {
		db.SoftClose()
		config := dm.configs[path]
		log.Printf("❌ Stopped replication: %s", config.ClientID)
	}
	
	log.Printf("📁 Database manager stopped")
}

// addWatchDir adiciona diretório para monitoramento
func (dm *DatabaseManager) addWatchDir(dir string) error {
	// Verificar se o diretório existe
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s (please create it first)", dir)
		}
		return fmt.Errorf("failed to access directory %s: %w", dir, err)
	}
	
	// Verificar se é realmente um diretório
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dir)
	}
	
	// Verificar se temos permissão de escrita (para criar arquivos de teste)
	testFile := filepath.Join(dir, ".litestream-access-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("directory is not writable: %s (error: %v)", dir, err)
	}
	os.Remove(testFile) // Limpar arquivo de teste
	
	return dm.watcher.Add(dir)
}

// watchFiles monitora mudanças nos arquivos
func (dm *DatabaseManager) watchFiles() {
	for {
		select {
		case <-dm.ctx.Done():
			return
		case event, ok := <-dm.watcher.Events:
			if !ok {
				return
			}
			dm.handleFileEvent(event)
		case err, ok := <-dm.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("⚠️  File watcher error: %v", err)
		}
	}
}

// handleFileEvent processa eventos de arquivo
func (dm *DatabaseManager) handleFileEvent(event fsnotify.Event) {
	if !dm.isDatabaseFile(event.Name) {
		return
	}

	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		log.Printf("📁 Database created: %s", event.Name)
		dm.registerDatabase(event.Name)
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		log.Printf("🗑️  Database removed: %s", event.Name)
		dm.unregisterDatabase(event.Name)
	case event.Op&fsnotify.Write == fsnotify.Write:
		// Arquivo modificado - já está sendo replicado
	}
}

// isDatabaseFile verifica se é arquivo de banco
func (dm *DatabaseManager) isDatabaseFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".db" || ext == ".sqlite" || ext == ".sqlite3"
}

// registerDatabase registra novo banco
func (dm *DatabaseManager) registerDatabase(dbPath string) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	// Verifica se já existe
	if _, exists := dm.databases[dbPath]; exists {
		return fmt.Errorf("database already registered: %s", dbPath)
	}

	// Extrai GUID do filename
	clientID := extractClientID(dbPath)
	if clientID == "" {
		return fmt.Errorf("invalid GUID format in filename: %s", filepath.Base(dbPath))
	}
	
	s3Path := fmt.Sprintf("databases/%s", clientID)
	
	// Cria configuração
	config := &DatabaseConfig{
		ClientID:     clientID,
		DatabasePath: dbPath,
		S3Path:       s3Path,
		CreatedAt:    time.Now(),
	}

	// Cria instância Litestream
	lsdb := litestream.NewDB(dbPath)
	
	// Configura S3
	client := lss3.NewReplicaClient()
	client.Bucket = dm.bucket
	client.Path = s3Path

	replica := litestream.NewReplica(lsdb, "s3")
	replica.Client = client
	lsdb.Replicas = append(lsdb.Replicas, replica)

	// Inicializa
	if err := lsdb.Open(); err != nil {
		return fmt.Errorf("failed to open database %s: %v", dbPath, err)
	}

	// Registra
	dm.databases[dbPath] = lsdb
	dm.configs[dbPath] = config

	log.Printf("✅ Client registered: %s -> s3://%s/%s/", 
		clientID, dm.bucket, s3Path)

	return nil
}

// unregisterDatabase remove banco
func (dm *DatabaseManager) unregisterDatabase(dbPath string) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	lsdb, exists := dm.databases[dbPath]
	if !exists {
		return fmt.Errorf("database not found: %s", dbPath)
	}

	config := dm.configs[dbPath]
	
	// Para replicação
	lsdb.SoftClose()
	
	// Remove dos mapas
	delete(dm.databases, dbPath)
	delete(dm.configs, dbPath)

	log.Printf("❌ Client unregistered: %s", config.ClientID)

	return nil
}

// scanExistingDatabases escaneia bancos existentes
func (dm *DatabaseManager) scanExistingDatabases() error {
	for _, watchDir := range dm.watchDirs {
		err := filepath.Walk(watchDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			
			if !info.IsDir() && dm.isDatabaseFile(path) {
				if err := dm.registerDatabase(path); err != nil {
					log.Printf("⚠️  Failed to register existing database %s: %v", path, err)
				}
			}
			return nil
		})
		
		if err != nil {
			log.Printf("⚠️  Failed to scan directory %s: %v", watchDir, err)
		}
	}
	
	dm.mutex.RLock()
	clientCount := len(dm.databases)
	dm.mutex.RUnlock()
	
	log.Printf("🎯 Monitoring %d clients across %d directories", clientCount, len(dm.watchDirs))
	return nil
}



func replicate(ctx context.Context, dsn, bucket, dbName string) (*litestream.DB, error) {
	// Create Litestream DB reference for managing replication.
	lsdb := litestream.NewDB(dsn)

	// Build S3 replica and attach to database.
	client := lss3.NewReplicaClient()
	client.Bucket = bucket
	client.Path = fmt.Sprintf("databases/%s", dbName) // Path: databases/{guid}/

	replica := litestream.NewReplica(lsdb, "s3")
	replica.Client = client

	lsdb.Replicas = append(lsdb.Replicas, replica)

	if err := restore(ctx, replica); err != nil {
		return nil, err
	}

	// Initialize database.
	if err := lsdb.Open(); err != nil {
		return nil, err
	}

	return lsdb, nil
}

func restore(ctx context.Context, replica *litestream.Replica) (err error) {
	// Skip restore if local database already exists.
	if _, err := os.Stat(replica.DB().Path()); err == nil {
		fmt.Println("local database already exists, skipping restore")
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	// Configure restore to write out to DSN path.
	opt := litestream.NewRestoreOptions()
	opt.OutputPath = replica.DB().Path()
	opt.Logger = log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds)

	// Determine the latest generation to restore from.
	if opt.Generation, _, err = replica.CalcRestoreTarget(ctx, opt); err != nil {
		return err
	}

	// Only restore if there is a generation available on the replica.
	// Otherwise we'll let the application create a new database.
	if opt.Generation == "" {
		fmt.Println("no generation found, creating new database")
		return nil
	}

	fmt.Printf("restoring replica for generation %s\n", opt.Generation)
	if err := replica.Restore(ctx, opt); err != nil {
		return err
	}
	fmt.Println("restore complete")
	return nil
}



// startStatusServer inicia servidor de status para modo diretório
func startStatusServer(dm *DatabaseManager, addr string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		dm.mutex.RLock()
		defer dm.mutex.RUnlock()
		
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Litestream Multi-Client Manager</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; margin: 40px; }
        .header { background: #f8f9fa; padding: 20px; border-radius: 8px; margin-bottom: 20px; }
        .client { background: #fff; border: 1px solid #dee2e6; border-radius: 8px; padding: 15px; margin: 10px 0; }
        .client-id { font-family: 'Monaco', 'Consolas', monospace; background: #e9ecef; padding: 4px 8px; border-radius: 4px; }
        .path { color: #6c757d; font-size: 0.9em; }
        .s3-path { color: #28a745; font-size: 0.9em; }
        .stats { display: flex; gap: 20px; margin: 20px 0; }
        .stat { background: #e3f2fd; padding: 15px; border-radius: 8px; text-align: center; }
    </style>
</head>
<body>
    <div class="header">
        <h1>🏢 Litestream Multi-Client Manager</h1>
        <p>📦 S3 Bucket: <strong>%s</strong></p>
        <p>👀 Watching: <strong>%v</strong></p>
    </div>
    
    <div class="stats">
        <div class="stat">
            <h3>%d</h3>
            <p>Active Clients</p>
        </div>
        <div class="stat">
            <h3>%d</h3>
            <p>Watch Directories</p>
        </div>
        <div class="stat">
            <h3>%s</h3>
            <p>Uptime</p>
        </div>
    </div>
    
    <h2>📊 Active Clients</h2>`, 
			dm.bucket, dm.watchDirs, len(dm.configs), len(dm.watchDirs), time.Since(time.Now().Add(-time.Hour)).Truncate(time.Second))
		
		if len(dm.configs) == 0 {
			fmt.Fprintf(w, `<p style="text-align: center; color: #6c757d; margin: 40px;">
				No clients found. Create a GUID.db file in the watched directories to get started.
			</p>`)
		} else {
			for path, config := range dm.configs {
				status := "🟢 Active"
				if _, exists := dm.databases[path]; !exists {
					status = "🔴 Inactive"
				}
				
				fmt.Fprintf(w, `
				<div class="client">
					<div style="display: flex; justify-content: space-between; align-items: center;">
						<span class="client-id">%s</span>
						<span>%s</span>
					</div>
					<div class="path">📁 %s</div>
					<div class="s3-path">☁️  s3://%s/%s/</div>
					<div style="color: #6c757d; font-size: 0.8em; margin-top: 8px;">
						⏰ Created: %s
					</div>
				</div>`,
					config.ClientID, status, config.DatabasePath, 
					dm.bucket, config.S3Path,
					config.CreatedAt.Format("2006-01-02 15:04:05"))
			}
		}
		
		fmt.Fprintf(w, `
		<div style="margin-top: 40px; padding: 20px; background: #f8f9fa; border-radius: 8px; color: #6c757d; font-size: 0.9em;">
			<h3>💡 Usage Tips</h3>
			<ul>
				<li>Create a new client: <code>touch /path/to/12345678-1234-5678-9abc-123456789012.db</code></li>
				<li>Remove a client: Delete the .db file from the filesystem</li>
				<li>GUID format: <code>xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx</code></li>
				<li>Refresh this page to see live updates</li>
			</ul>
		</div>
		</body>
		</html>`)
	})
	
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		dm.mutex.RLock()
		defer dm.mutex.RUnlock()
		
		w.Header().Set("Content-Type", "application/json")
		
		clients := make([]map[string]interface{}, 0, len(dm.configs))
		for path, config := range dm.configs {
			status := "active"
			if _, exists := dm.databases[path]; !exists {
				status = "inactive"
			}
			
			clients = append(clients, map[string]interface{}{
				"clientId":     config.ClientID,
				"databasePath": config.DatabasePath,
				"s3Path":       config.S3Path,
				"status":       status,
				"createdAt":    config.CreatedAt,
			})
		}
		
		response := map[string]interface{}{
			"bucket":          dm.bucket,
			"watchDirs":       dm.watchDirs,
			"totalClients":    len(dm.configs),
			"activeClients":   len(dm.databases),
			"clients":         clients,
		}
		
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	
	log.Fatal(http.ListenAndServe(addr, nil))
}
