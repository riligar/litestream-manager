# Directory Watching - Multi-Client Mode

## 🎯 **Visão Geral**

O **Directory Watching Mode** é a nova funcionalidade principal do sistema, projetada para **cenários SaaS multitenente** onde novos clientes são criados dinamicamente.

## 🆕 **Nova Abordagem**

### **❌ Antes: Manual por Cliente**
```bash
# Era necessário um comando para cada cliente
./litestream-example -dsn "/data/client1.db" -bucket "backups"
./litestream-example -dsn "/data/client2.db" -bucket "backups"
./litestream-example -dsn "/data/client3.db" -bucket "backups"
```

### **✅ Agora: Uma Instância para Todos**
```bash
# Uma única instância monitora todos os clientes
mkdir -p data/clients
./litestream-example -watch-dir "data/clients" -bucket "backups"
```

## 🏗️ **Como Funciona**

### **1. Inicialização**
```
1. Escaneia diretórios especificados
2. Detecta arquivos *.db existentes
3. Valida formato GUID
4. Registra automaticamente no Litestream
5. Inicia replicação S3 para cada cliente
```

### **2. Monitoramento Contínuo**
```
1. File Watcher monitora criação/remoção
2. Novos arquivos GUID.db → Registro automático
3. Arquivos removidos → Cleanup automático
4. Modificações → Replicação em tempo real
```

### **3. Interface Web**
```
1. Dashboard em http://localhost:8080
2. Lista todos os clientes ativos
3. Mostra paths S3 e status
4. API JSON em /api/status
```

## 🚀 **Casos de Uso Práticos**

### **1. SaaS Platform**
```bash
# Estrutura típica SaaS
/data/clients/
├── 12345678-1234-5678-9abc-123456789012.db  # Cliente ACME
├── 98765432-4321-8765-cba9-876543210987.db  # Cliente Beta
└── abcdef01-2345-6789-abcd-ef0123456789.db  # Cliente Gamma

# Comando único
mkdir -p data/clients
./litestream-example -watch-dir "data/clients" -bucket "saas-backups"

# Resultado automático:
# ✅ s3://saas-backups/databases/12345678-1234-5678-9abc-123456789012/
# ✅ s3://saas-backups/databases/98765432-4321-8765-cba9-876543210987/
# ✅ s3://saas-backups/databases/abcdef01-2345-6789-abcd-ef0123456789/
```

### **2. Múltiplos Ambientes**
```bash
# Monitorar produção e staging simultaneamente
./litestream-example \
  -watch-dir "/data/prod/,/data/staging/" \
  -bucket "company-backups"

# Estrutura:
/data/
├── prod/
│   ├── 12345678-1234-5678-9abc-123456789012.db
│   └── 98765432-4321-8765-cba9-876543210987.db
└── staging/
    ├── 12345678-1234-5678-9abc-123456789012.db
    └── 98765432-4321-8765-cba9-876543210987.db
```

### **3. Novo Cliente Dinâmico**
```bash
# Durante operação, novo cliente é criado
touch /data/clients/fedcba98-7654-3210-fedc-ba9876543210.db

# Logs automáticos:
# 2025/01/15 10:45:23 📁 Database created: /data/clients/fedcba98-7654-3210-fedc-ba9876543210.db
# 2025/01/15 10:45:23 ✅ Client registered: fedcba98-7654-3210-fedc-ba9876543210 -> s3://saas-backups/databases/fedcba98-7654-3210-fedc-ba9876543210/

# Dashboard atualizado automaticamente!
```

## 📊 **Interface Web Interativa**

### **Dashboard Principal (`http://localhost:8080`)**
```html
🏢 Litestream Multi-Client Manager
===============================================
📦 S3 Bucket: saas-backups
👀 Watching: [/data/clients/]

📊 Statistics:
- 15 Active Clients
- 1 Watch Directory  
- 2h 34m Uptime

📋 Active Clients:
┌─ 12345678-1234-5678-9abc-123456789012 ─── 🟢 Active
│  📁 /data/clients/12345678-1234-5678-9abc-123456789012.db
│  ☁️  s3://saas-backups/databases/12345678-1234-5678-9abc-123456789012/
│  ⏰ Created: 2025-01-15 08:30:45
└─────────────────────────────────────────────────────────

💡 Usage Tips:
- Create client: touch /path/to/12345678-1234-5678-9abc-123456789012.db
- Remove client: Delete the .db file
- GUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

### **API JSON (`/api/status`)**
```json
{
  "bucket": "saas-backups",
  "watchDirs": ["/data/clients/"],
  "totalClients": 15,
  "activeClients": 15,
  "clients": [
    {
      "clientId": "12345678-1234-5678-9abc-123456789012",
      "databasePath": "/data/clients/12345678-1234-5678-9abc-123456789012.db",
      "s3Path": "databases/12345678-1234-5678-9abc-123456789012",
      "status": "active",
      "createdAt": "2025-01-15T08:30:45Z"
    }
  ]
}
```

## ⚙️ **Configuração e Opções**

### **Flags Disponíveis**
```bash
./litestream-example --help

Usage:
  -watch-dir string
        directory to watch for GUID.db files (comma-separated for multiple)
  -bucket string
        s3 replica bucket
  -dsn string
        datasource name (legacy mode)
  -db-name string
        database name for organizing in S3 (optional)
```

### **Modos de Operação**

#### **🆕 Directory Mode (Recomendado)**
```bash
# Diretório único
mkdir -p data/clients
./litestream-example -watch-dir "data/clients" -bucket "backups"

# Múltiplos diretórios
mkdir -p data/prod data/staging data/dev
./litestream-example -watch-dir "data/prod,data/staging,data/dev" -bucket "backups"
```

#### **🔄 Legacy Mode (Compatibilidade)**
```bash
# Banco único
./litestream-example -dsn "/data/single.db" -bucket "backups"

# Com nome personalizado
./litestream-example -dsn "/data/legacy.db" -bucket "backups" -db-name "legacy-system"
```

## 🔍 **Detecção de Arquivos**

### **Extensões Suportadas**
- `.db` (recomendado)
- `.sqlite`
- `.sqlite3`

### **Validação GUID**
```go
// Formato válido: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
"12345678-1234-5678-9abc-123456789012" ✅
"abcdef01-2345-6789-abcd-ef0123456789" ✅
"invalid-guid"                          ❌
"regular-name"                          ❌ (usa fallback)
```

### **Organização S3 Automática**
```
Local: /data/clients/12345678-1234-5678-9abc-123456789012.db
S3:    s3://bucket/databases/12345678-1234-5678-9abc-123456789012/

Local: /data/prod/98765432-4321-8765-cba9-876543210987.db
S3:    s3://bucket/databases/98765432-4321-8765-cba9-876543210987/
```

## 📈 **Monitoramento e Logs**

### **Logs Estruturados**
```
2025/01/15 10:30:45 🏢 Litestream Multi-Client Manager (Directory Mode)
2025/01/15 10:30:45 📦 S3 Bucket: saas-backups
2025/01/15 10:30:45 👀 Watching Directories: [/data/clients/]
2025/01/15 10:30:45 👀 Watching directory: /data/clients/
2025/01/15 10:30:45 ✅ Client registered: 12345678-1234-5678-9abc-123456789012 -> s3://saas-backups/databases/12345678-1234-5678-9abc-123456789012/
2025/01/15 10:30:45 ✅ Client registered: 98765432-4321-8765-cba9-876543210987 -> s3://saas-backups/databases/98765432-4321-8765-cba9-876543210987/
2025/01/15 10:30:45 🎯 Monitoring 15 clients across 1 directories
```

### **Eventos em Tempo Real**
```
# Novo cliente criado
2025/01/15 10:45:23 📁 Database created: /data/clients/fedcba98-7654-3210-fedc-ba9876543210.db
2025/01/15 10:45:23 ✅ Client registered: fedcba98-7654-3210-fedc-ba9876543210

# Cliente removido
2025/01/15 11:15:10 🗑️  Database removed: /data/clients/old-client.db
2025/01/15 11:15:10 ❌ Client unregistered: old-client
```

## 🎯 **Vantagens do Directory Mode**

### **1. Escalabilidade**
- **Suporta milhares** de clientes por instância
- **Uma instância** gerencia todos os bancos
- **Recursos otimizados** com pooling

### **2. Simplicidade Operacional**
- **Zero configuração** para novos clientes
- **Hot-reload** automático
- **Sem restart** necessário

### **3. Monitoramento Centralizado**
- **Dashboard único** para todos os clientes
- **APIs programáticas** para integração
- **Logs centralizados** com contexto

### **4. Compatibilidade**
- **Modo legado** mantido para compatibilidade
- **Migração gradual** possível
- **Flags existentes** preservadas

## 🚨 **Considerações Importantes**

### **Performance**
- **File Watcher**: Usa `fsnotify` nativo (alta performance)
- **Thread Safety**: Operações concurrent-safe
- **Resource Limits**: ~1000 clientes por instância recomendado

### **Segurança**
- **Path Validation**: Previne directory traversal
- **GUID Validation**: Formato rigoroso obrigatório
- **S3 Isolation**: Cada cliente tem prefix próprio

### **Disponibilidade**
- **Graceful Shutdown**: Para replicação antes de fechar
- **Error Recovery**: Continua operando mesmo com falhas individuais
- **Health Monitoring**: Status individual por cliente

## 📚 **Recursos Relacionados**

- `docs/guid-implementation-summary.md` - Implementação GUID completa
- `docs/multitenant-architecture.md` - Arquitetura multitenente
- `docs/database-organization.md` - Organização S3
- `README.md` - Overview e exemplos básicos

## 🎉 **Conclusão**

O **Directory Watching Mode** representa a evolução natural do sistema para **cenários SaaS reais**, oferecendo:

- **🔍 Detecção automática** de novos clientes
- **⚡ Zero configuração** para casos comuns  
- **📊 Interface web** para monitoramento
- **🏗️ Arquitetura escalável** para produção
- **🔄 Compatibilidade total** com modo legado

**Ideal para plataformas SaaS que precisam de backup automático e escalável para múltiplos clientes.** 