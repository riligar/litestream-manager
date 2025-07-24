package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/benbjohnson/litestream"
	lss3 "github.com/benbjohnson/litestream/s3"
	"github.com/fsnotify/fsnotify"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed template.html
var templateContent string

// Logger personalizado que filtra mensagens técnicas do Litestream
type filteredWriter struct {
	writer io.Writer
}

func (fw *filteredWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	
	// Permite logs importantes de snapshot, generation e backup
	if strings.Contains(msg, "snapshot") || 
		strings.Contains(msg, "generation") || 
		strings.Contains(msg, "backup") ||
		strings.Contains(msg, "replicate") {
		return fw.writer.Write(p) // Permite logs de backup/snapshot/generation
	}
	
	// Filtra apenas mensagens técnicas realmente desnecessárias
	if strings.Contains(msg, "wal header mismatch") ||
		strings.Contains(msg, "cannot determine last wal position") ||
		strings.Contains(msg, "sync error") ||
		strings.Contains(msg, "init:") ||
		strings.Contains(msg, ".db-litestream/") ||
		strings.Contains(msg, "/wal/") {
		return len(p), nil // Descarta mensagem técnica
	}
	
	return fw.writer.Write(p)
}

// addr is the bind address for the web server.
// addr will be set based on the port flag

// startTime armazena quando o servidor foi iniciado
var startTime time.Time

// formatUptime formata o uptime de forma amigável
func formatUptime() string {
	duration := time.Since(startTime)
	
	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60
	
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}

// DatabaseManager gerencia instâncias do Litestream (1 banco por cliente)
type DatabaseManager struct {
	databases   map[string]*litestream.DB  // clientID -> litestream.DB
	clients     map[string]*ClientConfig   // clientID -> config  
	pathIndex   map[string]string          // dbPath -> clientID (index para lookups)
	watcher     *fsnotify.Watcher
	mutex       sync.RWMutex
	bucket      string
	watchDirs   []string
	ctx         context.Context
	cancel      context.CancelFunc
}

// ClientConfig configuração otimizada para 1:1 cliente:banco
type ClientConfig struct {
	ClientID     string    `json:"clientId"`
	DatabasePath string    `json:"databasePath"`
	CreatedAt    time.Time `json:"createdAt"`
}

// DashboardData dados para o template HTML
type DashboardData struct {
	Bucket        string       `json:"bucket"`
	WatchDirCount int          `json:"watchDirCount"`
	ClientCount   int          `json:"clientCount"`
	Uptime        string       `json:"uptime"`
	Clients       []ClientData `json:"clients"`
}

// ClientData dados de cada cliente para o template
type ClientData struct {
	ClientID     string `json:"clientId"`
	DatabasePath string `json:"databasePath"`
	StatusClass  string `json:"statusClass"`
	StatusText   string `json:"statusText"`
	CreatedAt    string `json:"createdAt"`
	Generations  []GenerationData `json:"generations,omitempty"`
}

// GenerationData informações de uma geração de backup
type GenerationData struct {
	ID       string        `json:"id"`
	Created  string        `json:"created"`
	Updated  string        `json:"updated"`
	Source   string        `json:"source"`    // "s3" ou "local"
	Snapshots []SnapshotData `json:"snapshots,omitempty"`
}

// SnapshotData informações de um snapshot
type SnapshotData struct {
	ID      string `json:"id"`
	Created string `json:"created"`
	Size    string `json:"size"`
	Source  string `json:"source"`    // "s3" ou "local"
}

// RestoreOption representa uma opção específica de restore
type RestoreOption struct {
	ID          string `json:"id"`
	Type        string `json:"type"`        // "generation", "snapshot", "wal"
	Timestamp   string `json:"timestamp"`
	Size        string `json:"size"`
	Description string `json:"description"`
	Command     string `json:"command"`     // Comando litestream para restaurar
}

// RestoreOptionsData todas as opções de restore disponíveis para um cliente
type RestoreOptionsData struct {
	ClientID       string          `json:"clientId"`
	TotalOptions   int            `json:"totalOptions"`
	LatestBackup   string         `json:"latestBackup"`
	RestoreOptions []RestoreOption `json:"restoreOptions"`
}

// getClientGenerations obtém gerações disponíveis para um cliente lendo dados reais dos arquivos
func (dm *DatabaseManager) getClientGenerations(clientID string) ([]GenerationData, error) {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()
	
	// Busca a instância do litestream.DB para o cliente
	lsdb, exists := dm.databases[clientID]
	if !exists {
		return nil, fmt.Errorf("client not found: %s", clientID)
	}
	
	// Caminho para o diretório .db-litestream (note o ponto no início)
	litestreamDir := fmt.Sprintf(".%s-litestream", filepath.Base(lsdb.Path()))
	litestreamFullPath := filepath.Join(filepath.Dir(lsdb.Path()), litestreamDir)
	generationsDir := filepath.Join(litestreamFullPath, "generations")
	
	// Verificar se o diretório existe
	if _, err := os.Stat(generationsDir); os.IsNotExist(err) {
		return []GenerationData{}, nil // Retorna vazio se não há generations
	}
	
	var generations []GenerationData
	
	// Ler diretórios de generations
	entries, err := os.ReadDir(generationsDir)
	if err != nil {
		log.Printf("⚠️  Error reading generations directory for client %s: %v", clientID, err)
		return []GenerationData{}, nil
	}
	
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		generationID := entry.Name()
		generationPath := filepath.Join(generationsDir, generationID)
		
		// Obter informações da generation
		info, err := entry.Info()
		if err != nil {
			continue
		}
		
		// Buscar o WAL mais recente para obter timestamp atualizado
		walDir := filepath.Join(generationPath, "wal")
		var latestWALTime time.Time = info.ModTime()
		
		if walEntries, err := os.ReadDir(walDir); err == nil {
			for _, walEntry := range walEntries {
				if strings.HasSuffix(walEntry.Name(), ".wal") {
					if walInfo, err := walEntry.Info(); err == nil {
						if walInfo.ModTime().After(latestWALTime) {
							latestWALTime = walInfo.ModTime()
						}
					}
				}
			}
		}
		
		generation := GenerationData{
			ID:      generationID,
			Created: info.ModTime().Format("2006-01-02 15:04:05"),
			Updated: latestWALTime.Format("2006-01-02 15:04:05"),
			Source:  "local", // Indicando que os dados vêm dos arquivos locais
		}
		
		generations = append(generations, generation)
	}
	
	// Ordenar por data de criação (mais recente primeiro)
	sort.Slice(generations, func(i, j int) bool {
		return generations[i].Created > generations[j].Created
	})
	
	return generations, nil
}

// getClientSnapshots obtém snapshots de uma geração específica lendo dados reais dos arquivos WAL
func (dm *DatabaseManager) getClientSnapshots(clientID, generationID string) ([]SnapshotData, error) {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()
	
	// Busca a instância do litestream.DB para o cliente
	lsdb, exists := dm.databases[clientID]
	if !exists {
		return nil, fmt.Errorf("client not found: %s", clientID)
	}
	
	// Caminho para o diretório WAL da generation específica (note o ponto no início)
	litestreamDir := fmt.Sprintf(".%s-litestream", filepath.Base(lsdb.Path()))
	litestreamFullPath := filepath.Join(filepath.Dir(lsdb.Path()), litestreamDir)
	walDir := filepath.Join(litestreamFullPath, "generations", generationID, "wal")
	
	// Verificar se o diretório existe
	if _, err := os.Stat(walDir); os.IsNotExist(err) {
		return []SnapshotData{}, nil // Retorna vazio se não há WAL files
	}
	
	var snapshots []SnapshotData
	
	// Ler arquivos WAL
	entries, err := os.ReadDir(walDir)
	if err != nil {
		log.Printf("⚠️  Error reading WAL directory for client %s generation %s: %v", clientID, generationID, err)
		return []SnapshotData{}, nil
	}
	
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".wal") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			
			// Converter bytes para formato amigável
			size := info.Size()
			var sizeStr string
			if size < 1024 {
				sizeStr = fmt.Sprintf("%dB", size)
			} else if size < 1024*1024 {
				sizeStr = fmt.Sprintf("%.1fKB", float64(size)/1024)
			} else {
				sizeStr = fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
			}
			
			snapshot := SnapshotData{
				ID:      strings.TrimSuffix(entry.Name(), ".wal"),
				Created: info.ModTime().Format("2006-01-02 15:04:05"),
				Size:    sizeStr,
				Source:  "local", // Indicando que os dados vêm dos arquivos locais
			}
			
			snapshots = append(snapshots, snapshot)
		}
	}
	
	// Ordenar por nome (ordem cronológica dos WAL files)
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].ID < snapshots[j].ID
	})
	
	return snapshots, nil
}

// getClientRestoreOptions lista todas as opções de restore disponíveis para um cliente
// Tenta S3 primeiro, depois fallback para dados locais
func (dm *DatabaseManager) getClientRestoreOptions(clientID string) (*RestoreOptionsData, error) {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()
	
	// Busca a instância do litestream.DB para o cliente
	lsdb, exists := dm.databases[clientID]
	if !exists {
		return nil, fmt.Errorf("client not found: %s", clientID)
	}
	
	var restoreOptions []RestoreOption
	var latestTimestamp time.Time
	var s3Available bool = false
	
	// Tentar buscar dados do S3 primeiro usando a biblioteca litestream
	if len(lsdb.Replicas) > 0 {
		replica := lsdb.Replicas[0]
		ctx := context.Background()
		
		// Tentar usar CalcRestoreTarget para verificar se S3 está acessível
		opt := litestream.NewRestoreOptions()
		if generation, _, err := replica.CalcRestoreTarget(ctx, opt); err == nil && generation != "" {
			s3Available = true
			log.Printf("🌐 S3 available for client %s, generation: %s", clientID, generation)
			
			// Adicionar opção de restore S3 (mais recente disponível)
			restoreOptions = append(restoreOptions, RestoreOption{
				ID:          generation,
				Type:        "generation",
				Timestamp:   time.Now().Format("2006-01-02 15:04:05"), // Timestamp aproximado
				Size:        "-",
				Description: fmt.Sprintf("Latest S3 generation %s", generation[:8]),
				Command:     fmt.Sprintf("litestream restore -o restored.db s3://%s/databases/%s", dm.bucket, clientID),
			})
			
			// Adicionar opção específica de generation
			restoreOptions = append(restoreOptions, RestoreOption{
				ID:          generation + "-specific",
				Type:        "generation",
				Timestamp:   time.Now().Add(-time.Hour).Format("2006-01-02 15:04:05"), // Timestamp aproximado
				Size:        "-",
				Description: fmt.Sprintf("S3 generation %s (specific)", generation[:8]),
				Command:     fmt.Sprintf("litestream restore -generation %s -o restored.db s3://%s/databases/%s", generation, dm.bucket, clientID),
			})
			
			latestTimestamp = time.Now()
		} else {
			log.Printf("⚠️  S3 not available for client %s: %v", clientID, err)
		}
	}
	
	// Buscar dados locais como fallback/complemento
	litestreamDir := fmt.Sprintf(".%s-litestream", filepath.Base(lsdb.Path()))
	litestreamFullPath := filepath.Join(filepath.Dir(lsdb.Path()), litestreamDir)
	generationsDir := filepath.Join(litestreamFullPath, "generations")
	
	// Verificar se o diretório local existe
	if _, err := os.Stat(generationsDir); err == nil {
		// Ler diretórios de generations locais
		entries, err := os.ReadDir(generationsDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				
				generationID := entry.Name()
				generationPath := filepath.Join(generationsDir, generationID)
				walDir := filepath.Join(generationPath, "wal")
				
				// Obter informações da generation
				info, err := entry.Info()
				if err != nil {
					continue
				}
				
				// Adicionar opção de restore para a generation local
				genTimestamp := info.ModTime()
				if genTimestamp.After(latestTimestamp) {
					latestTimestamp = genTimestamp
				}
				
				sourceLabel := "local"
				if s3Available {
					sourceLabel = "local+s3"
				}
				
				restoreOptions = append(restoreOptions, RestoreOption{
					ID:          generationID + "-local",
					Type:        "generation",
					Timestamp:   genTimestamp.Format("2006-01-02 15:04:05"),
					Size:        "-",
					Description: fmt.Sprintf("Local generation %s (%s)", generationID[:8], sourceLabel),
					Command:     fmt.Sprintf("litestream restore -generation %s -o restored.db s3://%s/databases/%s", generationID, dm.bucket, clientID),
				})
				
				// Listar WAL files individuais para restore point-in-time
				if walEntries, err := os.ReadDir(walDir); err == nil {
					for _, walEntry := range walEntries {
						if !walEntry.IsDir() && strings.HasSuffix(walEntry.Name(), ".wal") {
							walInfo, err := walEntry.Info()
							if err != nil {
								continue
							}
							
							walTimestamp := walInfo.ModTime()
							if walTimestamp.After(latestTimestamp) {
								latestTimestamp = walTimestamp
							}
							
							// Converter bytes para formato amigável
							size := walInfo.Size()
							var sizeStr string
							if size < 1024 {
								sizeStr = fmt.Sprintf("%dB", size)
							} else if size < 1024*1024 {
								sizeStr = fmt.Sprintf("%.1fKB", float64(size)/1024)
							} else {
								sizeStr = fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
							}
							
							walID := strings.TrimSuffix(walEntry.Name(), ".wal")
							restoreOptions = append(restoreOptions, RestoreOption{
								ID:          walID + "-local",
								Type:        "wal",
								Timestamp:   walTimestamp.Format("2006-01-02 15:04:05"),
								Size:        sizeStr,
								Description: fmt.Sprintf("Point-in-time WAL %s (%s)", walID, sourceLabel),
								Command:     fmt.Sprintf("litestream restore -timestamp \"%s\" -o restored.db s3://%s/databases/%s", walTimestamp.Format("2006-01-02T15:04:05Z"), dm.bucket, clientID),
							})
						}
					}
				}
			}
		}
	}
	
	// Se não há dados nem no S3 nem local
	if len(restoreOptions) == 0 {
		return &RestoreOptionsData{
			ClientID:       clientID,
			TotalOptions:   0,
			LatestBackup:   "No backups available",
			RestoreOptions: []RestoreOption{},
		}, nil
	}
	
	// Ordenar por timestamp (mais recente primeiro)
	sort.Slice(restoreOptions, func(i, j int) bool {
		return restoreOptions[i].Timestamp > restoreOptions[j].Timestamp
	})
	
	latestBackupStr := "No backups available"
	if !latestTimestamp.IsZero() {
		latestBackupStr = latestTimestamp.Format("2006-01-02 15:04:05")
		if s3Available {
			latestBackupStr += " (S3+Local)"
		} else {
			latestBackupStr += " (Local only)"
		}
	}
	
	return &RestoreOptionsData{
		ClientID:       clientID,
		TotalOptions:   len(restoreOptions),
		LatestBackup:   latestBackupStr,
		RestoreOptions: restoreOptions,
	}, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	// Configura logger para filtrar mensagens técnicas do Litestream
	log.SetOutput(&filteredWriter{writer: os.Stdout})

	// Inicializa tempo de start do servidor
	startTime = time.Now()

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

	fmt.Println("🏢 Litestream Multi-Client Manager")
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

// NewDatabaseManager cria novo gerenciador otimizado (1:1 cliente:banco)
func NewDatabaseManager(bucket string, watchDirs []string) *DatabaseManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Failed to create file watcher:", err)
	}

	return &DatabaseManager{
		databases: make(map[string]*litestream.DB),   // clientID -> DB
		clients:   make(map[string]*ClientConfig),    // clientID -> config
		pathIndex: make(map[string]string),           // path -> clientID
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

// Stop para o gerenciador (1:1 otimizado)
func (dm *DatabaseManager) Stop() {
	dm.cancel()
	dm.watcher.Close()
	
	dm.mutex.Lock()
	defer dm.mutex.Unlock()
	
	// Iteração otimizada usando clientID como chave
	for clientID, db := range dm.databases {
		db.SoftClose()
		log.Printf("❌ Stopped replication: %s", clientID)
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
		if dm.isDatabaseFile(event.Name) {
			log.Printf("🗑️  Database removed: %s", event.Name) 
			dm.unregisterDatabase(event.Name)
		}
	case event.Op&fsnotify.Write == fsnotify.Write:
		// Arquivo modificado - já está sendo replicado
	}
}

// isDatabaseFile verifica se é arquivo de banco
func (dm *DatabaseManager) isDatabaseFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".db" || ext == ".sqlite" || ext == ".sqlite3"
}

// isClientRegistered verifica se cliente já está registrado
func (dm *DatabaseManager) isClientRegistered(clientID string) bool {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()
	_, exists := dm.databases[clientID]
	return exists
}

// registerDatabase registra novo cliente (1:1 otimizado)
func (dm *DatabaseManager) registerDatabase(dbPath string) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	// Extrai GUID do filename
	clientID := extractClientID(dbPath)
	if clientID == "" {
		return fmt.Errorf("invalid GUID format in filename: %s", filepath.Base(dbPath))
	}

	// Verifica se cliente já existe (usar clientID como chave primária)
	if _, exists := dm.databases[clientID]; exists {
		return fmt.Errorf("client already registered: %s", clientID)
	}

	// Verifica se path já está mapeado
	if existingClientID, exists := dm.pathIndex[dbPath]; exists {
		return fmt.Errorf("path already mapped to client: %s -> %s", dbPath, existingClientID)
	}
	
	// Cria configuração otimizada
	config := &ClientConfig{
		ClientID:     clientID,
		DatabasePath: dbPath,
		CreatedAt:    time.Now(),
	}

	// Cria instância Litestream
	lsdb := litestream.NewDB(dbPath)
	
	// Configura S3 (path inline para performance)
	client := lss3.NewReplicaClient()
	client.Bucket = dm.bucket
	client.Path = fmt.Sprintf("databases/%s", clientID)

	replica := litestream.NewReplica(lsdb, "s3")
	replica.Client = client
	lsdb.Replicas = append(lsdb.Replicas, replica)

	// Inicializa
	if err := lsdb.Open(); err != nil {
		return fmt.Errorf("failed to open database %s: %v", dbPath, err)
	}

	// Registra usando clientID como chave primária
	dm.databases[clientID] = lsdb
	dm.clients[clientID] = config
	dm.pathIndex[dbPath] = clientID

	log.Printf("✅ Client registered: %s -> s3://%s/databases/%s/", 
		clientID, dm.bucket, clientID)

	return nil
}

// unregisterDatabase remove cliente (1:1 otimizado) 
func (dm *DatabaseManager) unregisterDatabase(dbPath string) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	// Lookup otimizado via pathIndex
	clientID, exists := dm.pathIndex[dbPath]
	if !exists {
		return nil // Silencioso se não existe
	}

	lsdb, dbExists := dm.databases[clientID] // O(1) lookup
	if dbExists {
		// Para replicação imediatamente 
		lsdb.Close()
	}
	
	// Remove de todos os mapas
	delete(dm.databases, clientID)
	delete(dm.clients, clientID)
	delete(dm.pathIndex, dbPath)

	log.Printf("❌ Client unregistered: %s", clientID)

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
				clientID := extractClientID(path)
				if clientID != "" && !dm.isClientRegistered(clientID) {
					if err := dm.registerDatabase(path); err != nil {
						log.Printf("⚠️  Failed to register existing database %s: %v", path, err)
					}
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



// startStatusServer inicia servidor de status usando template HTML
func startStatusServer(dm *DatabaseManager, addr string) {
	// Parse embedded template
	tmpl, err := template.New("dashboard").Parse(templateContent)
	if err != nil {
		log.Fatal("Failed to parse embedded template:", err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		dm.mutex.RLock()
		defer dm.mutex.RUnlock()
		
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		
		// Preparar dados para o template (ordenado por clientID)
		clientIDs := make([]string, 0, len(dm.clients))
		for clientID := range dm.clients {
			clientIDs = append(clientIDs, clientID)
		}
		sort.Strings(clientIDs) // Ordena alfabeticamente
		
		var clients []ClientData
		for _, clientID := range clientIDs {
			config := dm.clients[clientID]
			statusClass := "status-active"
			statusText := "ACTIVE"
			if _, exists := dm.databases[clientID]; !exists {
				statusClass = "status-inactive"
				statusText = "INACTIVE"
			}
			
			clients = append(clients, ClientData{
				ClientID:     clientID,
				DatabasePath: config.DatabasePath,
				StatusClass:  statusClass,
				StatusText:   statusText,
				CreatedAt:    config.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
		
		data := DashboardData{
			Bucket:        dm.bucket,
			WatchDirCount: len(dm.watchDirs),
			ClientCount:   len(dm.clients),
			Uptime:        formatUptime(),
			Clients:       clients,
		}
		
		// Renderizar template
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		dm.mutex.RLock()
		defer dm.mutex.RUnlock()
		
		w.Header().Set("Content-Type", "application/json")
		
		// Pre-allocate para melhor performance (ordenado)
		clientIDs := make([]string, 0, len(dm.clients))
		for clientID := range dm.clients {
			clientIDs = append(clientIDs, clientID)
		}
		sort.Strings(clientIDs) // Ordena alfabeticamente
		
		clients := make([]map[string]interface{}, 0, len(dm.clients))
		
		// Iteração otimizada usando clientID ordenado
		for _, clientID := range clientIDs {
			config := dm.clients[clientID]
			status := "active"
			if _, exists := dm.databases[clientID]; !exists {
				status = "inactive"
			}
			
			clients = append(clients, map[string]interface{}{
				"clientId":     clientID,
				"databasePath": config.DatabasePath,
				"s3Path":       fmt.Sprintf("databases/%s", clientID), // inline para performance
				"status":       status,
				"createdAt":    config.CreatedAt,
			})
		}
		
		response := map[string]interface{}{
			"bucket":          dm.bucket,
			"watchDirs":       dm.watchDirs,
			"totalClients":    len(dm.clients),    // otimizado
			"activeClients":   len(dm.databases),  // já usa clientID
			"uptime":          formatUptime(),
			"clients":         clients,
		}
		
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	
	// Endpoint para obter gerações e snapshots de um cliente específico
	http.HandleFunc("/api/client/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		// Extrair clientID da URL: /api/client/{clientID}/generations
		path := strings.TrimPrefix(r.URL.Path, "/api/client/")
		parts := strings.Split(path, "/")
		
		if len(parts) < 2 || (parts[1] != "generations" && parts[1] != "restore-options") {
			http.Error(w, "Invalid path. Use /api/client/{clientID}/generations or /api/client/{clientID}/restore-options", http.StatusBadRequest)
			return
		}
		
		clientID := parts[0]
		endpoint := parts[1]
		
		dm.mutex.RLock()
		_, exists := dm.clients[clientID]
		dm.mutex.RUnlock()
		
		if !exists {
			http.Error(w, "Client not found", http.StatusNotFound)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		
		if endpoint == "restore-options" {
			// Endpoint para listar todas as opções de restore
			restoreData, err := dm.getClientRestoreOptions(clientID)
			if err != nil {
				log.Printf("⚠️  Failed to get restore options for client %s: %v", clientID, err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			
			if err := json.NewEncoder(w).Encode(restoreData); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		
		// Endpoint original para generations
		// Obter gerações
		generations, err := dm.getClientGenerations(clientID)
		if err != nil {
			log.Printf("⚠️  Failed to get generations for client %s: %v", clientID, err)
			// Retorna array vazio em caso de erro para não quebrar a UI
			generations = []GenerationData{}
		}
		
		// Obter snapshots para cada geração
		for i := range generations {
			snapshots, err := dm.getClientSnapshots(clientID, generations[i].ID)
			if err != nil {
				log.Printf("⚠️  Failed to get snapshots for client %s generation %s: %v", 
					clientID, generations[i].ID, err)
				snapshots = []SnapshotData{}
			}
			generations[i].Snapshots = snapshots
		}
		
		response := map[string]interface{}{
			"clientId":    clientID,
			"generations": generations,
		}
		
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	
	log.Fatal(http.ListenAndServe(addr, nil))
}
