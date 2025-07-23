# Arquitetura Multitenente para SQLite Dinâmico

## 🏢 Visão Geral

Esta arquitetura permite suportar **múltiplos tenants** com bancos SQLite gerados dinamicamente, oferecendo detecção automática, gestão em tempo real e organização hierárquica no S3.

## 🎯 Estrutura Proposta no S3

```
bucket-multitenente/
├── databases/
│   ├── 12345678-1234-5678-9abc-123456789012/
│   │   ├── snapshots/
│   │   │   └── generation-abc123/
│   │   └── wal/
│   │       ├── generation-abc123/
│   │       └── 00000001.wal
│   ├── 98765432-4321-8765-cba9-876543210987/
│   │   ├── snapshots/
│   │   └── wal/
│   ├── abcdef01-2345-6789-abcd-ef0123456789/
│   │   ├── snapshots/
│   │   └── wal/
│   └── fedcba98-7654-3210-fedc-ba9876543210/
│       ├── snapshots/
│       └── wal/
```

## 🔧 Componentes da Arquitetura

### 1. **Database Manager (Gerenciador de Bancos)**
```go
type DatabaseManager struct {
    databases     map[string]*litestream.DB
    watchers      map[string]*fsnotify.Watcher
    mutex         sync.RWMutex
    bucket        string
    watchPaths    []string
}
```

### 2. **Tenant Isolamento**
```go
type TenantConfig struct {
    TenantID      string
    DatabasePath  string
    S3Prefix      string
    RetentionDays int
    Enabled       bool
}
```

### 3. **File Watcher (Monitor de Arquivos)**
- Detecta novos arquivos `.db` em diretórios monitorados
- Registra automaticamente novos bancos no Litestream
- Remove bancos quando arquivos são deletados

### 4. **API de Controle Dinâmico**
```bash
# Registrar novo banco
POST /api/databases
{
  "tenantId": "tenant-001",
  "dbPath": "/data/tenant-001/orders.db",
  "dbName": "orders"
}

# Listar bancos ativos
GET /api/databases

# Pausar/retomar replicação
PUT /api/databases/{tenantId}/{dbName}/status
```

## 🚀 Padrões de Nomenclatura

### Convenções de Paths Locais:
```
/data/
├── 12345678-1234-5678-9abc-123456789012.db  # Cliente 1
├── 98765432-4321-8765-cba9-876543210987.db  # Cliente 2
├── abcdef01-2345-6789-abcd-ef0123456789.db  # Cliente 3
├── fedcba98-7654-3210-fedc-ba9876543210.db  # Cliente 4
└── shared/
    └── system.db                            # Sistema interno
```

### Mapeamento S3:
```
Local: /data/12345678-1234-5678-9abc-123456789012.db
S3: s3://bucket/databases/12345678-1234-5678-9abc-123456789012/

Local: /data/98765432-4321-8765-cba9-876543210987.db  
S3: s3://bucket/databases/98765432-4321-8765-cba9-876543210987/
```

## ⚙️ Implementações Específicas

### A. **Detecção Automática por Padrão**
```bash
# Monitora diretório principal por arquivos GUID.db
./litestream-multitenant \
  -bucket "company-data" \
  -watch-path "/data/" \
  -guid-pattern "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.db"
```

### B. **Configuração via API**
```bash
# Registra cliente dinamicamente
curl -X POST http://localhost:8080/api/databases \
  -d '{
    "clientId": "12345678-1234-5678-9abc-123456789012",
    "dbPath": "/data/12345678-1234-5678-9abc-123456789012.db",
    "s3Prefix": "databases/12345678-1234-5678-9abc-123456789012"
  }'
```

### C. **Configuração YAML Dinâmica**
```yaml
# litestream-multitenant.yml
bucket: company-data
clients:
  - clientId: 12345678-1234-5678-9abc-123456789012
    dbPath: /data/12345678-1234-5678-9abc-123456789012.db
    s3Prefix: databases/12345678-1234-5678-9abc-123456789012
    retention: 168h
    
  - clientId: 98765432-4321-8765-cba9-876543210987
    dbPath: /data/98765432-4321-8765-cba9-876543210987.db
    s3Prefix: databases/98765432-4321-8765-cba9-876543210987
    retention: 72h

# Auto-discovery
autoDiscovery:
  enabled: true
  watchPaths:
    - /data/
  patterns:
    - "*.db"
  guidValidation: true
  guidPattern: "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"
```

## 🔄 Fluxos de Operação

### 1. **Novo Cliente/Banco Criado**
```
1. File Watcher detecta /data/12345678-1234-5678-9abc-123456789012.db
2. Extrai clientId = "12345678-1234-5678-9abc-123456789012"
3. Valida formato GUID
4. Cria nova instância Litestream
5. Configura S3 path: databases/12345678-1234-5678-9abc-123456789012/
6. Inicia replicação automática
7. Registra no DatabaseManager
8. Logs: "New client registered: 12345678-1234-5678-9abc-123456789012"
```

### 2. **Banco Removido**
```
1. File Watcher detecta remoção de arquivo
2. Para replicação do cliente específico
3. Remove instância do DatabaseManager  
4. Mantém dados S3 (conforme política retenção)
5. Logs: "Client unregistered: 12345678-1234-5678-9abc-123456789012"
```

### 3. **Health Check por Cliente**
```bash
# Status de todos os clientes
GET /api/health/clients

# Status específico por GUID
GET /api/health/clients/12345678-1234-5678-9abc-123456789012
```

## 📊 Monitoramento e Métricas

### Métricas por Cliente:
```
litestream_databases_active{client="12345678-1234-5678-9abc-123456789012"} 1
litestream_replication_lag{client="12345678-1234-5678-9abc-123456789012"} 45ms
litestream_s3_sync_errors{client="98765432-4321-8765-cba9-876543210987"} 0
```

### Logs Estruturados:
```json
{
  "timestamp": "2025-01-15T10:30:45Z",
  "level": "info", 
  "client": "12345678-1234-5678-9abc-123456789012",
  "operation": "sync",
  "duration": "150ms",
  "s3_path": "databases/12345678-1234-5678-9abc-123456789012/"
}
```

## 🛡️ Considerações de Segurança

### 1. **Isolamento por Tenant**
- Permissões S3 granulares por prefix
- Validação de paths para evitar directory traversal
- Rate limiting por tenant

### 2. **Configuração de Acesso**
```yaml
tenants:
  - tenantId: tenant-001
    s3Config:
      bucket: company-data
      prefix: tenants/tenant-001
      accessKey: ${TENANT_001_ACCESS_KEY}
      secretKey: ${TENANT_001_SECRET_KEY}
```

## 🎯 Casos de Uso Suportados

### A. **SaaS com Banco por Cliente (Cenário Principal)**
```
/data/
├── 12345678-1234-5678-9abc-123456789012.db  # Cliente ACME Corp
├── 98765432-4321-8765-cba9-876543210987.db  # Cliente Beta LLC
├── abcdef01-2345-6789-abcd-ef0123456789.db  # Cliente Gamma Inc
└── fedcba98-7654-3210-fedc-ba9876543210.db  # Cliente Delta Co
```

### B. **Ambientes Separados por Cliente**
```
/production/
├── 12345678-1234-5678-9abc-123456789012.db  # Cliente A - Prod

/staging/
├── 12345678-1234-5678-9abc-123456789012.db  # Cliente A - Stage

/development/
├── 12345678-1234-5678-9abc-123456789012.db  # Cliente A - Dev
```

### C. **Distribuição Multi-Servidor**
```
/server1/data/
├── aaaaaaaa-1111-2222-3333-444444444444.db  # Clientes A-M

/server2/data/
├── bbbbbbbb-2222-3333-4444-555555555555.db  # Clientes N-Z
```

## ⚡ Performance e Escalabilidade

### Otimizações:
1. **Pool de Watchers** - Reutilização de file watchers
2. **Batch Operations** - Agrupamento de operações S3
3. **Async Processing** - Processamento assíncrono de eventos
4. **Caching** - Cache de metadados de bancos ativos
5. **Resource Limits** - Limites por tenant (CPU, Memória, S3 calls)

### Limites Recomendados:
- **Max Clientes**: 1000 por instância
- **Max Diretórios Monitorados**: 10
- **Concurrent S3 Uploads**: 50 simultâneos
- **GUID Validation**: Obrigatória

## 🔧 Configuração de Exemplo

```yaml
# litestream-multitenant.yml
server:
  port: 8080
  bucket: company-multitenant
  
autoDiscovery:
  enabled: true
  watchPaths:
    - /data/tenants/
  filePatterns:
    - "*.db"
    - "*.sqlite"
  tenantRegex: "tenants/(?P<tenant>[^/]+)"
  
tenantDefaults:
  retention: 72h
  syncInterval: 1s
  snapshotInterval: 1h
  
limits:
  maxTenantsPerInstance: 500
  maxDatabasesPerTenant: 20
  maxConcurrentSyncs: 50
```

## 🎯 Próximos Passos

1. **✅ Implementar DatabaseManager**
2. **✅ Adicionar File Watching**  
3. **✅ Criar API de controle**
4. **✅ Configurar auto-discovery**
5. **✅ Adicionar métricas por tenant**
6. **✅ Implementar health checks**
7. **✅ Documentar padrões de uso** 