# Guia de Implementação: Sistema Multitenente

## 🎯 Visão Geral

Este guia fornece instruções práticas para implementar um sistema **multitenente** baseado no Litestream, expandindo o exemplo atual para suportar **múltiplos bancos SQLite dinâmicos**.

## 🏗️ Componentes Necessários

### 1. **Dependências Adicionais**
```go
// go.mod additions
require (
    github.com/fsnotify/fsnotify v1.7.0  // File watching
    github.com/gorilla/mux v1.8.1        // HTTP routing
)
```

### 2. **Estruturas de Dados Principais**
```go
type DatabaseManager struct {
    databases  map[string]*litestream.DB
    configs    map[string]*TenantConfig
    watcher    *fsnotify.Watcher
    mutex      sync.RWMutex
    bucket     string
    watchPaths []string
}

type TenantConfig struct {
    TenantID     string
    DatabasePath string
    DatabaseName string
    S3Prefix     string
    Enabled      bool
    CreatedAt    time.Time
}
```

### 3. **File Watching System**
```go
func (dm *DatabaseManager) watchFiles() {
    for {
        select {
        case event := <-dm.watcher.Events:
            dm.handleFileEvent(event)
        case err := <-dm.watcher.Errors:
            log.Printf("Watcher error: %v", err)
        }
    }
}

func (dm *DatabaseManager) handleFileEvent(event fsnotify.Event) {
    if dm.isDatabaseFile(event.Name) {
        switch event.Op {
        case fsnotify.Create:
            dm.registerDatabase(event.Name)
        case fsnotify.Remove:
            dm.unregisterDatabase(event.Name)
        }
    }
}
```

## 🎛️ APIs de Controle

### Endpoints Essenciais
```go
// GET /api/health - Status geral
type HealthResponse struct {
    TotalTenants   int            `json:"totalTenants"`
    TotalDatabases int            `json:"totalDatabases"`
    ActiveTenants  []string       `json:"activeTenants"`
    Databases      []DatabaseInfo `json:"databases"`
}

// GET /api/databases - Lista bancos
// POST /api/databases - Registra banco manualmente
// DELETE /api/databases/{tenant}/{db} - Remove banco
```

## 📁 Padrões de Organização

### Estrutura de Diretórios Local
```
/data/
├── tenant-001/
│   ├── users.db
│   ├── orders.db
│   └── analytics.db
├── tenant-002/
│   ├── users.db
│   └── products.db
├── logs/
│   ├── tenant-001/
│   │   ├── 2025-01-15.db
│   │   └── 2025-01-16.db
│   └── tenant-002/
│       └── 2025-01-15.db
└── shared/
    └── system.db
```

### Mapeamento S3 Automático
```go
func (dm *DatabaseManager) extractTenantInfo(dbPath string) (tenantID, dbName string) {
    // Padrão: /data/tenant-001/users.db
    re := regexp.MustCompile(`/([^/]+)/([^/]+)\.db$`)
    matches := re.FindStringSubmatch(dbPath)
    
    if len(matches) >= 3 {
        tenantID = matches[1]  // tenant-001
        dbName = matches[2]    // users
    }
    
    return sanitizeName(tenantID), sanitizeName(dbName)
}

// Resultado S3: tenants/tenant-001/users/
```

## ⚙️ Configuração YAML

```yaml
# litestream-multitenant.yml
server:
  port: 8080
  bucket: company-multitenant
  
# Auto-discovery de bancos
autoDiscovery:
  enabled: true
  watchPaths:
    - /data/tenants/
    - /data/logs/
  filePatterns:
    - "*.db"
    - "*.sqlite"
  tenantRegex: "(?P<tenant>[^/]+)"

# Configurações por tenant
tenantDefaults:
  retention: 72h
  syncInterval: 1s
  snapshotInterval: 1h
  
# Limites de recursos
limits:
  maxTenantsPerInstance: 500
  maxDatabasesPerTenant: 20
  maxConcurrentSyncs: 50

# Tenants específicos
tenants:
  - tenantId: tenant-001
    watchPath: /data/tenant-001/
    s3Prefix: tenants/tenant-001
    retention: 168h  # 7 dias
    
  - tenantId: tenant-002
    watchPath: /data/tenant-002/
    s3Prefix: tenants/tenant-002
    retention: 72h   # 3 dias
```

## 🔧 Funções de Registro Dinâmico

### Registro Automático
```go
func (dm *DatabaseManager) registerDatabase(dbPath string) error {
    dm.mutex.Lock()
    defer dm.mutex.Unlock()

    // Verifica duplicação
    if _, exists := dm.databases[dbPath]; exists {
        return fmt.Errorf("database already registered: %s", dbPath)
    }

    // Extrai informações
    tenantID, dbName := dm.extractTenantInfo(dbPath)
    
    // Configura Litestream
    lsdb := litestream.NewDB(dbPath)
    
    client := lss3.NewReplicaClient()
    client.Bucket = dm.bucket
    client.Path = fmt.Sprintf("tenants/%s/%s", tenantID, dbName)

    replica := litestream.NewReplica(lsdb, "s3")
    replica.Client = client
    lsdb.Replicas = append(lsdb.Replicas, replica)

    // Inicializa
    if err := lsdb.Open(); err != nil {
        return err
    }

    // Registra
    dm.databases[dbPath] = lsdb
    dm.configs[dbPath] = &TenantConfig{
        TenantID:     tenantID,
        DatabasePath: dbPath,
        DatabaseName: dbName,
        S3Prefix:     fmt.Sprintf("tenants/%s/%s", tenantID, dbName),
        Enabled:      true,
        CreatedAt:    time.Now(),
    }

    log.Printf("✅ Registered: tenant=%s db=%s", tenantID, dbName)
    return nil
}
```

### Remoção Segura
```go
func (dm *DatabaseManager) unregisterDatabase(dbPath string) error {
    dm.mutex.Lock()
    defer dm.mutex.Unlock()

    lsdb, exists := dm.databases[dbPath]
    if !exists {
        return fmt.Errorf("database not found: %s", dbPath)
    }

    config := dm.configs[dbPath]
    
    // Para replicação gracefully
    lsdb.SoftClose()
    
    // Remove dos registros
    delete(dm.databases, dbPath)
    delete(dm.configs, dbPath)

    log.Printf("❌ Unregistered: tenant=%s db=%s", 
        config.TenantID, config.DatabaseName)
    
    return nil
}
```

## 🚀 Inicialização do Sistema

```go
func main() {
    // Configuração
    bucket := "company-multitenant"
    watchPaths := []string{"/data/tenants", "/data/logs"}

    // Cria manager
    dm := NewDatabaseManager(bucket, watchPaths)
    defer dm.Stop()

    // Inicia monitoramento
    if err := dm.Start(); err != nil {
        log.Fatal("Failed to start:", err)
    }

    // Configura API
    router := dm.setupRoutes()
    server := &http.Server{
        Addr:    ":8080",
        Handler: router,
    }

    log.Println("🚀 Multitenant Litestream started on :8080")
    log.Fatal(server.ListenAndServe())
}
```

## 📊 Monitoramento e Métricas

### Logs Estruturados
```go
type LogEntry struct {
    Timestamp time.Time `json:"timestamp"`
    Level     string    `json:"level"`
    Tenant    string    `json:"tenant"`
    Database  string    `json:"database"`
    Operation string    `json:"operation"`
    Duration  string    `json:"duration"`
    S3Path    string    `json:"s3_path"`
}
```

### Health Checks
```bash
# Status geral
curl http://localhost:8080/api/health

# Resposta:
{
  "totalTenants": 3,
  "totalDatabases": 8,
  "activeTenants": ["tenant-001", "tenant-002", "tenant-003"],
  "databases": [...]
}

# Lista de bancos
curl http://localhost:8080/api/databases
```

## 🔐 Segurança e Isolamento

### Permissões S3 Granulares
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": "arn:aws:iam::ACCOUNT:user/tenant-001"},
      "Action": ["s3:GetObject", "s3:PutObject"],
      "Resource": "arn:aws:s3:::bucket/tenants/tenant-001/*"
    }
  ]
}
```

### Validação de Paths
```go
func (dm *DatabaseManager) validatePath(dbPath string) error {
    // Previne directory traversal
    if strings.Contains(dbPath, "..") {
        return fmt.Errorf("invalid path: %s", dbPath)
    }
    
    // Valida extensão
    if !dm.isDatabaseFile(dbPath) {
        return fmt.Errorf("not a database file: %s", dbPath)
    }
    
    return nil
}
```

## 🎯 Cenários de Uso Práticos

### 1. **SaaS Platform**
```bash
# Cliente cria novo banco
mkdir -p /data/client-acme/
echo | sqlite3 /data/client-acme/app.db "CREATE TABLE users(id INTEGER)"

# Sistema detecta automaticamente
# S3: s3://bucket/tenants/client-acme/app/
```

### 2. **Logs Temporais**
```bash
# Script diário cria novo log
DATE=$(date +%Y-%m-%d)
sqlite3 /data/logs/tenant-001/${DATE}.db "CREATE TABLE events(...)"

# Backup automático para S3
# S3: s3://bucket/tenants/tenant-001/logs_${DATE}/
```

### 3. **Ambiente de Desenvolvimento**
```bash
# Desenvolvedores criam bancos temporários
sqlite3 /data/dev/feature-123/test.db "..."

# Backup automático sem configuração manual
# S3: s3://bucket/tenants/feature-123/test/
```

## ⚡ Performance e Otimizações

### Pool de Resources
```go
type ResourcePool struct {
    watchers     sync.Pool
    connections  sync.Pool
    s3Clients    sync.Pool
}
```

### Batch Operations
```go
func (dm *DatabaseManager) batchSync() {
    // Agrupa operações S3 para eficiência
    for tenant, databases := range dm.groupByTenant() {
        go dm.syncTenantDatabases(tenant, databases)
    }
}
```

## 🎯 Próximos Passos

1. **Implementar DatabaseManager** básico
2. **Adicionar File Watching** com fsnotify
3. **Criar APIs** de controle REST
4. **Configurar auto-discovery** de bancos
5. **Adicionar monitoramento** e métricas
6. **Implementar segurança** granular
7. **Otimizar performance** para scale

## 📚 Recursos Adicionais

- `docs/multitenant-architecture.md` - Arquitetura completa
- `docs/system-comparison.md` - Comparação com sistema atual
- `docs/database-organization.md` - Organização atual de bancos

Este guia fornece a base para implementar um **sistema multitenente robusto** que suporta **SQLite dinâmico** com **detecção automática**, **APIs de controle** e **organização hierárquica** no S3. 