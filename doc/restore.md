## 🔍 Natureza do Litestream

O **Litestream** é uma ferramenta de replicação contínua para bancos SQLite que trabalha com o conceito de **streaming de WAL (Write-Ahead Log)**. Aqui está como funciona:

### 📚 Conceitos Fundamentais

**1. WAL Streaming:**
- O SQLite usa WAL (Write-Ahead Log) para transações
- O Litestream monitora o arquivo WAL em tempo real
- Cada mudança é imediatamente enviada para o destino (S3)
- **Resultado:** Backup contínuo, não apenas snapshots pontuais

**2. Gerações e Snapshots:**
- **Geração:** Um ciclo completo de backup (snapshot inicial + WAL incremental)
- **Snapshot:** Cópia completa do banco em um momento específico
- **WAL Segments:** Mudanças incrementais após o snapshot

**3. Estrutura no S3:**
```
s3://bucket/databases/{clientID}/
├── generations/
│   ├── {generation-id}/
│   │   ├── snapshot/           # Snapshot completo
│   │   └── wal/               # Segmentos WAL incrementais
│   │       ├── 000000.wal
│   │       ├── 000001.wal
│   │       └── ...
│   └── ...
```

## ⏰ Restauração Temporal (Point-in-Time Recovery)

### 🎯 Como Funciona

O Litestream permite restaurar o banco em **qualquer momento específico** através de:

**1. Identificação do Ponto:**
```bash
# Listar gerações disponíveis
litestream generations s3://bucket/databases/12345678-1234-5678-9abc-123456789012

# Listar snapshots de uma geração
litestream snapshots s3://bucket/databases/12345678-1234-5678-9abc-123456789012 -generation {gen-id}
```

**2. Restauração com Timestamp:**
```bash
litestream restore \
  -o "restored-database.db" \
  -timestamp "2024-01-15T14:30:00Z" \
  s3://bucket/databases/12345678-1234-5678-9abc-123456789012
```

**3. Restauração com Geração Específica:**
```bash
litestream restore \
  -o "restored-database.db" \
  -generation "abc123def456" \
  s3://bucket/databases/12345678-1234-5678-9abc-123456789012
```

### 🔧 Opções de Restauração

**Por Timestamp:**
```bash
# Restaurar banco às 14:30 de hoje
litestream restore -timestamp "2024-01-15T14:30:00Z" -o restore.db s3://...

# Restaurar banco há 2 horas
litestream restore -timestamp "$(date -u -d '2 hours ago' +%Y-%m-%dT%H:%M:%SZ)" -o restore.db s3://...
```

**Por Índice WAL:**
```bash
# Restaurar até um índice específico do WAL
litestream restore -index 1234 -o restore.db s3://...
```

**Última Versão Disponível:**
```bash
# Restaurar a versão mais recente (padrão)
litestream restore -o restore.db s3://...
```

### 🎪 Exemplo Prático

**Cenário:** Você quer restaurar o banco do cliente `12345678-1234-5678-9abc-123456789012` como estava às 15:45 de ontem:

```bash
# 1. Verificar gerações disponíveis
litestream generations s3://your-bucket/databases/12345678-1234-5678-9abc-123456789012

# 2. Restaurar para o timestamp específico
litestream restore \
  -o "client-backup-15h45.db" \
  -timestamp "2024-01-14T15:45:00Z" \
  s3://your-bucket/databases/12345678-1234-5678-9abc-123456789012

# 3. Verificar o banco restaurado
sqlite3 client-backup-15h45.db ".tables"
```

### ⚡ Características Importantes

**1. Precisão Temporal:**
- Restauração com precisão de **microssegundos**
- Baseado nos timestamps dos WAL segments
- Garantia de consistência transacional

**2. Eficiência:**
- Só baixa o **snapshot base** + **WAL segments necessários**
- Não precisa baixar todo o histórico
- Reconstrução incremental local

**3. Flexibilidade:**
- Múltiplos pontos de restauração por geração
- Pode escolher geração específica ou timestamp global
- Restauração parcial ou completa

O Litestream transforma o SQLite em um banco com **backup contínuo** e **recuperação temporal**, mantendo a simplicidade do SQLite mas com recursos enterprise de backup e restore! 🚀