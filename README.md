# Litestream Manager

Sistema de backup automático para SQLite com suporte a múltiplos clientes baseados em GUID.

## 🚀 Instalação

```bash
# Compilar
go build -o litestream-manager main.go

# Para outros sistemas:
# Windows: GOOS=windows GOARCH=amd64 go build -o litestream-manager.exe main.go
# Linux:   GOOS=linux GOARCH=amd64 go build -o litestream-manager-linux main.go
```

## ⚙️ Uso

### Configuração AWS
```bash
export AWS_ACCESS_KEY_ID=xxxxxxxxxxxxxxxxxxxx
export AWS_SECRET_ACCESS_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Comando Básico
```bash
# Criar diretório
mkdir -p data/clients

# Iniciar monitoramento
./litestream-manager -watch-dir "data/clients" -bucket "seu-bucket-s3"

# Dashboard: http://localhost:8080
```

## 📋 Parâmetros

| Flag | Descrição | Padrão |
|------|-----------|--------|
| `-watch-dir` | Diretórios para monitorar (separados por vírgula) | **obrigatório** |
| `-bucket` | Bucket S3 para backup | **obrigatório** |
| `-port` | Porta do servidor web | `8080` |

## 🎯 Como Funciona

### 1. Criar Cliente
```bash
# GUID obrigatório: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
touch data/clients/12345678-1234-5678-9abc-123456789012.db

# Sistema detecta automaticamente e cria:
# s3://bucket/databases/12345678-1234-5678-9abc-123456789012/
```

### 2. Remover Cliente
```bash
rm data/clients/12345678-1234-5678-9abc-123456789012.db
# Sistema remove automaticamente do monitoramento
```

### 3. Múltiplos Ambientes
```bash
# Produção
./litestream-manager -watch-dir "data/prod" -bucket "prod-backups"

# Staging  
./litestream-manager -watch-dir "data/staging" -bucket "staging-backups" -port 8081
```

## 📊 Estrutura

### Local
```
data/clients/
├── 12345678-1234-5678-9abc-123456789012.db  # Cliente A
├── 98765432-4321-8765-cba9-876543210987.db  # Cliente B
└── abcdef01-2345-6789-abcd-ef0123456789.db  # Cliente C
```

### S3
```
s3://bucket/databases/
├── 12345678-1234-5678-9abc-123456789012/
├── 98765432-4321-8765-cba9-876543210987/
└── abcdef01-2345-6789-abcd-ef0123456789/
```

## 🔧 Restauração

```bash
litestream restore \
  -o "restore/cliente.db" \
  s3://bucket/databases/12345678-1234-5678-9abc-123456789012
```

## ⚡ Performance

- **Clientes**: ~1000 por instância (1:1 cliente:banco)
- **Lookup**: O(1) para todas as operações  
- **Memória**: 30-150MB otimizada
- **File Watcher**: fsnotify nativo (sub-milissegundo)

## 🔄 Fluxo de Trabalho

**Step by Step do Servidor:**

1. **Inicialização**: Valida diretórios e inicia file watcher
2. **Descoberta**: Escaneia arquivos `.db` existentes com GUID válido
3. **Configuração**: Para cada banco detectado:
   - Cria configuração Litestream única
   - Inicia processo de backup contínuo
   - Registra cliente no sistema (O(1) lookup)
4. **Monitoramento**: File watcher detecta mudanças em tempo real:
   - **CREATE**: Novo `.db` → adiciona cliente automaticamente
   - **DELETE**: Remove `.db` → para backup e limpa registros
   - **MODIFY**: Atualiza estatísticas de tamanho
5. **Dashboard**: Interface web atualiza dados em tempo real
6. **Backup S3**: Litestream replica continuamente para `s3://bucket/databases/{clientID}/`

**Fluxo Otimizado**: Detecção sub-milissegundo → Backup automático → Dashboard em tempo real

## 🎯 Exemplo Completo

```bash
# 1. Configurar AWS
export AWS_ACCESS_KEY_ID=your-key
export AWS_SECRET_ACCESS_KEY=your-secret

# 2. Criar estrutura
mkdir -p data/clients

# 3. Iniciar sistema  
./litestream-manager -watch-dir "data/clients" -bucket "company-backups"

# 4. Adicionar clientes
touch data/clients/12345678-1234-5678-9abc-123456789012.db
touch data/clients/98765432-4321-8765-cba9-876543210987.db

# 5. Monitorar: http://localhost:8080
```

**Sistema otimizado para produção SaaS com backup automático!** 🚀

