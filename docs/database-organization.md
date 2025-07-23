# Organização de Bancos de Dados no S3

## 📁 Visão Geral

Esta implementação organiza **bancos SQLite de clientes** em pastas separadas no S3, usando **GUIDs como identificadores únicos** de cada cliente/tenant.

## 🎯 Estrutura Resultante no S3

```
seu-bucket/
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

## 🚀 Formas de Uso

### 1. **Extração Automática do GUID**
O sistema extrai automaticamente o GUID do nome do arquivo para organização no S3:

```bash
# Cliente 1: GUID extraído automaticamente
./litestream-example \
  -dsn "/data/12345678-1234-5678-9abc-123456789012.db" \
  -bucket "meu-bucket"
# Resultado: databases/12345678-1234-5678-9abc-123456789012/

# Cliente 2: Outro GUID
./litestream-example \
  -dsn "/data/98765432-4321-8765-cba9-876543210987.db" \
  -bucket "meu-bucket"
# Resultado: databases/98765432-4321-8765-cba9-876543210987/

# Cliente 3: Caminho completo
./litestream-example \
  -dsn "/var/data/clients/abcdef01-2345-6789-abcd-ef0123456789.db" \
  -bucket "meu-bucket"
# Resultado: databases/abcdef01-2345-6789-abcd-ef0123456789/
```

### 2. **Nome Personalizado (Para casos especiais)**
Você pode especificar um nome customizado usando a flag `-db-name`:

```bash
# Nome personalizado para banco não-GUID
./litestream-example \
  -dsn "/data/legacy-client.db" \
  -bucket "meu-bucket" \
  -db-name "legacy-client-001"
# Resultado: databases/legacy-client-001/

# Sistema interno
./litestream-example \
  -dsn "/data/system.db" \
  -bucket "meu-bucket" \
  -db-name "internal-system"
# Resultado: databases/internal-system/
```

## 📋 Exemplos Práticos

### Cenário: SaaS Multi-Cliente

```bash
# Cliente 1 - Empresa ACME
./litestream-example \
  -dsn "/data/12345678-1234-5678-9abc-123456789012.db" \
  -bucket "saas-backups"
# → s3://saas-backups/databases/12345678-1234-5678-9abc-123456789012/

# Cliente 2 - Empresa Beta Corp
./litestream-example \
  -dsn "/data/98765432-4321-8765-cba9-876543210987.db" \
  -bucket "saas-backups"
# → s3://saas-backups/databases/98765432-4321-8765-cba9-876543210987/

# Cliente 3 - Startup Gamma
./litestream-example \
  -dsn "/data/abcdef01-2345-6789-abcd-ef0123456789.db" \
  -bucket "saas-backups"
# → s3://saas-backups/databases/abcdef01-2345-6789-abcd-ef0123456789/
```

### Cenário: Ambientes Separados

```bash
# Cliente em Produção
./litestream-example \
  -dsn "/prod/12345678-1234-5678-9abc-123456789012.db" \
  -bucket "company-backups"
# → s3://company-backups/databases/12345678-1234-5678-9abc-123456789012/

# Cliente em Staging
./litestream-example \
  -dsn "/staging/12345678-1234-5678-9abc-123456789012.db" \
  -bucket "company-backups-staging"
# → s3://company-backups-staging/databases/12345678-1234-5678-9abc-123456789012/
```

### Cenário: Múltiplos Clientes por Servidor

```bash
# Servidor 1 - Clientes A, B, C
./litestream-example -dsn "/server1/aaaaaaaa-1111-2222-3333-444444444444.db" -bucket "backups"
./litestream-example -dsn "/server1/bbbbbbbb-2222-3333-4444-555555555555.db" -bucket "backups"
./litestream-example -dsn "/server1/cccccccc-3333-4444-5555-666666666666.db" -bucket "backups"
# → s3://backups/databases/{cada-guid}/
```

## 🔧 Regras de Nomenclatura

O sistema detecta e processa GUIDs automaticamente:

| Entrada | Saída | Transformação |
|---------|-------|---------------|
| `12345678-1234-5678-9abc-123456789012.db` | `12345678-1234-5678-9abc-123456789012` | GUID extraído (mantém formato) |
| `aaaaaaaa-1111-2222-3333-444444444444.db` | `aaaaaaaa-1111-2222-3333-444444444444` | GUID extraído (mantém formato) |
| `fedcba98-7654-3210-fedc-ba9876543210.db` | `fedcba98-7654-3210-fedc-ba9876543210` | GUID extraído (mantém formato) |
| `legacy-client.db` + `-db-name "legacy-001"` | `legacy-001` | Nome personalizado |
| `system.db` + `-db-name "internal-system"` | `internal-system` | Nome personalizado |
| *(nome inválido)* | `default` | Fallback para nome inválido |

## 🔄 Restauração Organizada

A restauração também funciona com a estrutura baseada em GUIDs:

```bash
# Restaurar cliente específico
litestream restore \
  -o "/restore/12345678-1234-5678-9abc-123456789012.db" \
  s3://meu-bucket/databases/12345678-1234-5678-9abc-123456789012

# Restaurar outro cliente com configuração
litestream restore \
  -config litestream.yml \
  -o "/restore/98765432-4321-8765-cba9-876543210987.db" \
  s3://meu-bucket/databases/98765432-4321-8765-cba9-876543210987
```

## ⚙️ Configuração via Arquivo

Você também pode usar arquivo de configuração YAML:

```yaml
# litestream.yml
access-key-id: AKIAxxxxxxxxxxxxxxxx
secret-access-key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx/xxxxxxxxx

dbs:
  # Cliente 1 - GUID extraído automaticamente
  - path: /data/12345678-1234-5678-9abc-123456789012.db
    replicas:
      - type: s3
        bucket: saas-backups
        path: databases/12345678-1234-5678-9abc-123456789012
        region: us-east-1

  # Cliente 2 - Outro GUID
  - path: /data/98765432-4321-8765-cba9-876543210987.db  
    replicas:
      - type: s3
        bucket: saas-backups
        path: databases/98765432-4321-8765-cba9-876543210987
        region: us-east-1

  # Cliente 3 - Com retenção customizada
  - path: /data/abcdef01-2345-6789-abcd-ef0123456789.db
    replicas:
      - type: s3
        bucket: saas-backups
        path: databases/abcdef01-2345-6789-abcd-ef0123456789
        region: us-east-1
        retention: 168h  # 7 dias
```

## 🎯 Vantagens da Organização por GUID

1. **🆔 Identificação Única**: Cada cliente tem identificador globalmente único
2. **🔍 Localização Direta**: GUIDs facilitam busca exata de clientes
3. **🛡️ Isolamento Total**: Cada cliente tem pasta S3 separada
4. **📊 Monitoramento Granular**: Métricas específicas por cliente/GUID
5. **🔐 Permissões Específicas**: Acesso S3 por GUID individual
6. **🗂️ Governança Flexível**: Políticas diferentes por cliente
7. **🔄 Migração Simples**: GUIDs mantêm consistência entre ambientes

## 📈 Monitoramento

Cada cliente agora inclui o GUID na saída de logs:

```
2025/01/15 10:30:45 new transaction: db=12345678-1234-5678-9abc-123456789012 pre=abc123 post=def456 elapsed=150ms
2025/01/15 10:30:46 new transaction: db=98765432-4321-8765-cba9-876543210987 pre=def456 post=ghi789 elapsed=200ms
```

## 🚨 Considerações Importantes

1. **GUID Válido**: Sistema valida formato GUID (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)
2. **Uma Instância por Cliente**: Litestream suporta uma instância ativa por banco/cliente
3. **GUIDs Únicos**: Certifique-se que GUIDs são únicos globalmente
4. **Compatibilidade**: Estrutura baseada em GUIDs é estável e compatível
5. **Fallback**: Bancos não-GUID usam flag `-db-name` ou nome "default"
6. **Performance**: GUIDs como chaves oferecem performance consistente
7. **Migração**: Clientes existentes podem migrar mantendo mesmo GUID 