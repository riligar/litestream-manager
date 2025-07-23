# Organização de Bancos de Dados no S3

## 📁 Visão Geral

Esta implementação permite organizar diferentes bancos de dados SQLite em **pastas separadas** dentro do bucket S3, facilitando a gestão e organização de múltiplos bancos.

## 🎯 Estrutura Resultante no S3

```
seu-bucket/
├── databases/
│   ├── users/
│   │   ├── snapshots/
│   │   │   └── generation-abc123/
│   │   └── wal/
│   │       ├── generation-abc123/
│   │       └── 00000001.wal
│   ├── products/
│   │   ├── snapshots/
│   │   └── wal/
│   ├── analytics_dashboard/
│   │   ├── snapshots/
│   │   └── wal/
│   └── orders_2025_01_15/
│       ├── snapshots/
│       └── wal/
```

## 🚀 Formas de Uso

### 1. **Extração Automática do Nome**
O sistema extrai automaticamente o nome do banco de dados a partir do caminho do arquivo:

```bash
# Banco: users.db -> Pasta: databases/users/
./litestream-example -dsn "/data/users.db" -bucket "meu-bucket"

# Banco: products.sqlite -> Pasta: databases/products/
./litestream-example -dsn "/var/lib/products.sqlite" -bucket "meu-bucket"

# Banco: 2025-01-15.db -> Pasta: databases/2025-01-15/
./litestream-example -dsn "data/2025-01-15.db" -bucket "meu-bucket"
```

### 2. **Nome Personalizado**
Você pode especificar um nome customizado usando a flag `-db-name`:

```bash
# Nome personalizado
./litestream-example \
  -dsn "/data/analytics.db" \
  -bucket "meu-bucket" \
  -db-name "Analytics Dashboard"
# Resultado: databases/analytics_dashboard/

# Múltiplas instâncias do mesmo app
./litestream-example \
  -dsn "/data/app.db" \
  -bucket "meu-bucket" \
  -db-name "Production Instance"
# Resultado: databases/production_instance/
```

## 📋 Exemplos Práticos

### Cenário: Múltiplos Microserviços

```bash
# Serviço de usuários
./litestream-example -dsn "/app/users.db" -bucket "company-backups"
# → s3://company-backups/databases/users/

# Serviço de produtos  
./litestream-example -dsn "/app/products.db" -bucket "company-backups"
# → s3://company-backups/databases/products/

# Serviço de pedidos
./litestream-example -dsn "/app/orders.db" -bucket "company-backups"
# → s3://company-backups/databases/orders/
```

### Cenário: Ambientes Separados

```bash
# Produção
./litestream-example \
  -dsn "/data/app.db" \
  -bucket "company-backups" \
  -db-name "Production App"

# Desenvolvimento  
./litestream-example \
  -dsn "/data/app.db" \
  -bucket "company-backups" \
  -db-name "Development App"

# Teste
./litestream-example \
  -dsn "/data/app.db" \
  -bucket "company-backups" \
  -db-name "Test Environment"
```

### Cenário: Bancos por Data

```bash
# Logs diários
./litestream-example -dsn "logs/2025-01-15.db" -bucket "logs-backup"
# → s3://logs-backup/databases/2025-01-15/

./litestream-example -dsn "logs/2025-01-16.db" -bucket "logs-backup"  
# → s3://logs-backup/databases/2025-01-16/
```

## 🔧 Regras de Nomenclatura

O sistema automaticamente sanitiza os nomes para garantir compatibilidade com S3:

| Entrada | Saída | Transformação |
|---------|-------|---------------|
| `users.db` | `users` | Remove extensão |
| `User Database` | `user_database` | Espaços → underscores, lowercase |
| `app/data/products` | `app_data_products` | Barras → underscores |
| `My-App_DB.backup` | `my-app_db.backup` | Mantém hífens e pontos |
| `2025-01-15` | `2025-01-15` | Mantém formato de data |
| *(vazio)* | `default` | Fallback para nome vazio |

## 🔄 Restauração Organizada

A restauração também funciona com a estrutura organizada:

```bash
# Restaurar banco específico
litestream restore \
  -o "/nova/localizacao/users.db" \
  s3://meu-bucket/databases/users

# Restaurar com configuração
litestream restore \
  -config litestream.yml \
  -o "/restore/products.db" \
  s3://meu-bucket/databases/products
```

## ⚙️ Configuração via Arquivo

Você também pode usar arquivo de configuração YAML:

```yaml
# litestream.yml
access-key-id: AKIAxxxxxxxxxxxxxxxx
secret-access-key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx/xxxxxxxxx

dbs:
  # Banco de usuários
  - path: /data/users.db
    replicas:
      - type: s3
        bucket: meu-bucket
        path: databases/users
        region: us-east-1

  # Banco de produtos com nome personalizado
  - path: /data/catalog.db  
    replicas:
      - type: s3
        bucket: meu-bucket
        path: databases/product_catalog
        region: us-east-1

  # Analytics com retenção customizada
  - path: /data/analytics.db
    replicas:
      - type: s3
        bucket: meu-bucket
        path: databases/analytics_dashboard
        region: us-east-1
        retention: 168h  # 7 dias
```

## 🎯 Vantagens da Organização

1. **📁 Separação Clara**: Cada banco tem sua própria pasta
2. **🔍 Fácil Localização**: Encontre backups rapidamente
3. **🛡️ Isolamento**: Problemas em um banco não afetam outros
4. **📊 Monitoramento**: Veja uso de storage por banco
5. **🔐 Permissões**: Configure acesso granular por pasta
6. **🗂️ Governança**: Aplique políticas de retenção específicas

## 📈 Monitoramento

Cada banco agora inclui o nome na saída de logs:

```
2025/01/15 10:30:45 new transaction: db=users pre=abc123 post=def456 elapsed=150ms
2025/01/15 10:30:46 new transaction: db=products pre=def456 post=ghi789 elapsed=200ms
```

## 🚨 Considerações Importantes

1. **Uma Instância por Banco**: Litestream só suporta uma instância ativa por banco
2. **Nomes Únicos**: Certifique-se que nomes de banco são únicos no bucket  
3. **Compatibilidade**: Estrutura é compatível com versões futuras do Litestream
4. **Custos**: Cada pasta adiciona overhead mínimo no S3
5. **Migração**: Bancos existentes podem ser migrados para nova estrutura 