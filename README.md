# Litestream Manager

Sistema de backup automático para SQLite usando Litestream como biblioteca, com suporte a múltiplos clientes baseados em GUID.

## 🚀 Instalação

```bash
go install .
```

## ⚙️ Uso Básico

### Configuração AWS
```bash
export AWS_ACCESS_KEY_ID=xxxxxxxxxxxxxxxxxxxx
export AWS_SECRET_ACCESS_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Modo Recomendado: Monitoramento de Diretório
```bash
# Criar estrutura local
mkdir -p data/clients

# Iniciar monitoramento automático
go run main.go -watch-dir "data/clients" -bucket "seu-bucket-s3"

# Acessar dashboard: http://localhost:8080
```

### Criação Dinâmica de Clientes
```bash
# Criar novo cliente (GUID obrigatório)
touch data/clients/12345678-1234-5678-9abc-123456789012.db

# Sistema detecta automaticamente e cria backup em:
# s3://seu-bucket/databases/12345678-1234-5678-9abc-123456789012/
```

## 📋 Parâmetros Disponíveis

| Parâmetro | Descrição | Exemplo |
|-----------|-----------|---------|
| `-watch-dir` | Diretórios para monitorar (separados por vírgula) | `"data/clients,data/prod"` |
| `-bucket` | Bucket S3 para backup | `"company-backups"` |
| `-port` | Porta do servidor web | `8080` (padrão) |
| `-dsn` | Modo legado: arquivo único | `"/data/single.db"` |
| `-db-name` | Nome personalizado no S3 | `"legacy-system"` |

## 🎯 Casos de Uso

### SaaS Multicliente
```bash
# Uma instância monitora todos os clientes
mkdir -p data/clients
go run main.go -watch-dir "data/clients" -bucket "saas-backups"

# Estrutura local:
# data/clients/
# ├── 12345678-1234-5678-9abc-123456789012.db  # Cliente A
# ├── 98765432-4321-8765-cba9-876543210987.db  # Cliente B
# └── abcdef01-2345-6789-abcd-ef0123456789.db  # Cliente C

# Estrutura S3:
# s3://saas-backups/databases/
# ├── 12345678-1234-5678-9abc-123456789012/
# ├── 98765432-4321-8765-cba9-876543210987/
# └── abcdef01-2345-6789-abcd-ef0123456789/
```

### Múltiplos Ambientes
```bash
# Produção
go run main.go -watch-dir "data/prod" -bucket "prod-backups" -port 8080

# Staging  
go run main.go -watch-dir "data/staging" -bucket "staging-backups" -port 8081

# Desenvolvimento
go run main.go -watch-dir "data/dev" -bucket "dev-backups" -port 8082
```

### Sistema Legado
```bash
# Banco único com nome personalizado
go run main.go -dsn "data/legacy.db" -bucket "backups" -db-name "sistema-antigo"
```

## 🔍 Regras de Nomenclatura

### GUID Válido (Automático)
```
✅ 12345678-1234-5678-9abc-123456789012.db
✅ aaaaaaaa-1111-2222-3333-444444444444.db
❌ cliente-123.db (ignorado)
❌ dados.db (ignorado)
```

### Extensões Suportadas
- `.db` (recomendado)
- `.sqlite`
- `.sqlite3`

## 📊 Monitoramento

### Dashboard Web
- **URL**: `http://localhost:8080`
- **API Status**: `http://localhost:8080/api/status`
- **Estatísticas**: Clientes ativos, status S3, tempo de atividade

### Logs Estruturados
```
2025/01/15 10:30:45 ✅ Cliente registrado: 12345678-1234-5678-9abc-123456789012
2025/01/15 10:45:23 📁 Database criado: /data/clients/novo-cliente.db
2025/01/15 11:15:10 🗑️ Cliente removido: cliente-antigo
```

## 🔄 Operações Dinâmicas

### Adicionar Cliente
```bash
# Durante execução, criar novo arquivo
touch data/clients/fedcba98-7654-3210-fedc-ba9876543210.db

# Sistema automaticamente:
# 1. Detecta o arquivo
# 2. Valida formato GUID
# 3. Configura backup S3
# 4. Inicia replicação
# 5. Atualiza dashboard
```

### Remover Cliente
```bash
# Deletar arquivo local
rm data/clients/cliente-antigo.db

# Sistema automaticamente:
# 1. Para replicação
# 2. Remove da lista ativa  
# 3. Mantém dados S3 (conforme retenção)
```

## 🛡️ Características de Segurança

- **Isolamento S3**: Cada cliente tem prefix próprio
- **Validação GUID**: Formato rigoroso obrigatório
- **Path Validation**: Previne directory traversal
- **Thread Safety**: Operações concurrent-safe

## ⚡ Performance

### Capacidades
- **Clientes suportados**: ~1000 por instância
- **Bancos por cliente**: ~50
- **Threads concurrent**: Até 50 sync S3
- **File Watcher**: `fsnotify` nativo (alta performance)

### Recursos Típicos
- **CPU**: Baixo a médio
- **Memória**: 50-200MB (dependendo do número de clientes)
- **Network**: Conforme atividade S3

## 🔧 Restauração

```bash
# Restaurar cliente específico
litestream restore \
  -o "restore/12345678-1234-5678-9abc-123456789012.db" \
  s3://bucket/databases/12345678-1234-5678-9abc-123456789012

# Restaurar com timestamp específico
litestream restore \
  -timestamp "2025-01-15T10:30:00Z" \
  -o "restore/cliente.db" \
  s3://bucket/databases/12345678-1234-5678-9abc-123456789012
```

## 🚨 Solução de Problemas

### Diretório não encontrado
```bash
❌ directory does not exist: /data/ (please create it first)
✅ mkdir -p data/clients
```

### Porta em uso
```bash
❌ listen tcp :8080: bind: address already in use
✅ go run main.go -watch-dir "data" -bucket "backups" -port 9090
```

### GUID inválido
```bash
❌ cliente-123.db (ignorado)  
✅ 12345678-1234-5678-9abc-123456789012.db
```

## 🎯 Exemplo Completo

```bash
# 1. Configurar AWS
export AWS_ACCESS_KEY_ID=your-key
export AWS_SECRET_ACCESS_KEY=your-secret

# 2. Criar estrutura
mkdir -p data/clients

# 3. Iniciar sistema
go run main.go -watch-dir "data/clients" -bucket "company-backups"

# 4. Adicionar clientes  
touch data/clients/12345678-1234-5678-9abc-123456789012.db
touch data/clients/98765432-4321-8765-cba9-876543210987.db

# 5. Verificar dashboard
# Abrir: http://localhost:8080

# 6. Testar remoção
rm data/clients/98765432-4321-8765-cba9-876543210987.db
```

**Sistema pronto para produção com backup automático e monitoramento em tempo real!** 🚀

