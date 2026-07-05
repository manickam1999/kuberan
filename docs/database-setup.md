# Database Setup

Kuberan supports two database backends in production. Choose the one that fits your setup.

Kuberan uses **two databases** on the same PostgreSQL instance:

- The **app database** (`kuberan`) -- all application data, migrated by golang-migrate.
- The **Hydra database** (`hydra`) -- Ory Hydra's OAuth state (clients, grants, consent sessions) for the MCP OAuth flow, migrated by the `hydra-migrate` compose service. Its connection string is configured separately via `HYDRA_DSN` in `.env.prod` (see `.env.prod.example` for examples per backend).

In development, `apps/api/dev/init.sql` creates the `hydra` database automatically. In production you must set it up yourself (covered per option below).

| | Self-Hosted PostgreSQL | Supabase |
|---|---|---|
| **Data location** | Your VPS (Docker volume) | Supabase's cloud |
| **Setup complexity** | Low (runs in Docker) | Low (managed service) |
| **Free tier** | Unlimited | Limited (paused after inactivity) |
| **Backups** | Manual (pg_dump) or cron | Automatic (Supabase dashboard) |
| **Best for** | Full self-hosting, privacy-first | Managed convenience |

---

## Option A: Self-Hosted PostgreSQL

PostgreSQL runs as a Docker Compose service alongside the API and web app. Data is persisted in a named Docker volume on your VPS.

### 1. Configure `.env.prod`

Uncomment the Option A block and comment out (or remove) Option B:

```bash
COMPOSE_PROFILES=postgres,backup
DB_HOST=postgres
DB_PORT=5432
DB_USER=kuberan
DB_PASSWORD=your_strong_password_here
DB_NAME=kuberan
DB_SSLMODE=disable

# Hydra's OAuth state lives in a dedicated database on the same instance
HYDRA_DSN=postgres://kuberan:your_strong_password_here@postgres:5432/hydra?sslmode=disable
```

Generate a strong password:
```bash
openssl rand -hex 32
```

### 2. Create the Hydra database

The production `postgres` service does not run an init script, so the `hydra` database must be created once before the first deploy (otherwise `hydra-migrate` fails, and `hydra` and `mcp` won't start):

```bash
docker compose -f docker-compose.prod.yml up -d postgres
docker compose -f docker-compose.prod.yml exec postgres \
  psql -U kuberan -c "CREATE DATABASE hydra OWNER kuberan;"
```

### 3. Deploy

The `postgres` service starts automatically because `COMPOSE_PROFILES` includes `postgres`:

```bash
cd /opt/kuberan
./deploy/deploy.sh
```

The deploy script:
1. Builds the images
2. Runs app-database migrations directly against Postgres (port 5432)
3. Starts all services: API, web, MCP server, Hydra (after `hydra-migrate` completes), bot, and any profile-gated services (postgres, backup, oracle)

### 4. Verify

```bash
# Check postgres container is running
docker compose -f docker-compose.prod.yml ps postgres

# Check API health (includes DB ping)
curl http://localhost:8080/api/health
```

### Data Location

Data is stored in a Docker named volume:
```bash
# Inspect volume location
docker volume inspect kuberan_postgres_data

# List volume contents (mounts at /var/lib/postgresql/data inside container)
docker compose -f docker-compose.prod.yml exec postgres ls /var/lib/postgresql/data
```

### Backups

See the [Automated Backups](#automated-backups) section below for the recommended approach.

**Manual backup:**
```bash
docker compose -f docker-compose.prod.yml exec postgres \
  pg_dump -U kuberan kuberan --no-owner --no-acl -Fc \
  > kuberan_backup_$(date +%Y%m%d_%H%M%S).dump
```

**Restore from backup:**
```bash
docker compose -f docker-compose.prod.yml exec -T postgres \
  pg_restore -U kuberan -d kuberan --no-owner --clean \
  < kuberan_backup_YYYYMMDD_HHMMSS.dump
```

---

## Option B: Supabase

Supabase provides a managed PostgreSQL instance with a web dashboard, automatic backups, and connection pooling via Supavisor.

### 1. Create a Supabase Project

1. Go to [supabase.com](https://supabase.com) and create a new project
2. Wait for the project to be provisioned

### 2. Get Your Credentials

In the Supabase dashboard:
1. Go to **Settings > Database**
2. Find **Connection string** section
3. Select **Transaction** mode (port 6543) for the app connection
4. Copy the credentials

### 3. Configure `.env.prod`

Keep the Option B block active (it is the default in `.env.prod.example`):

```bash
COMPOSE_PROFILES=
DB_HOST=db.xxxxxxxxxxxx.supabase.co
DB_PORT=6543
DB_USER=postgres.xxxxxxxxxxxx
DB_PASSWORD=your_supabase_db_password
DB_NAME=postgres
DB_SSLMODE=require
```

> **Why port 6543?** The app connects via Supavisor (connection pooler, transaction mode) for efficiency. The deploy script automatically uses port 5432 (direct connection) for migrations, since `golang-migrate` requires advisory locks that Supavisor's transaction mode does not support.

### 4. Set up Hydra's schema

On Supabase, Hydra gets a dedicated schema and role inside the main `postgres` database, targeted via `search_path`. In the SQL editor:

```sql
CREATE SCHEMA hydra;
CREATE ROLE hydra_svc WITH LOGIN PASSWORD 'your_strong_password_here';
GRANT ALL ON SCHEMA hydra TO hydra_svc;
ALTER ROLE hydra_svc SET search_path = hydra;
```

Then set `HYDRA_DSN` in `.env.prod` using the **session** pooler (port 5432 -- Hydra's long-lived connections break on the 6543 transaction pooler):

```bash
HYDRA_DSN=postgres://hydra_svc.<project-ref>:<pw>@<region>.pooler.supabase.com:5432/postgres?sslmode=require&search_path=hydra
```

### 5. Deploy

```bash
cd /opt/kuberan
./deploy/deploy.sh
```

No local postgres container is started. Migrations run via the direct Supabase connection (port 5432), then the API connects via the pooler (port 6543).

### Backups

Supabase provides automatic daily backups on paid plans. For an independent backup you control, see the [Automated Backups](#automated-backups) section below.

---

## Switching Between Options

If you want to move data from one backend to the other:

> **Note:** the commands below move only the app database. Hydra's OAuth state (registered MCP clients, grants, consent sessions) lives in its own database/schema and is not included -- after switching, either dump/restore the `hydra` database the same way or simply let MCP clients re-register and re-authorize.

### Supabase → Self-Hosted

```bash
# 1. Dump from Supabase (use direct port 5432)
pg_dump "postgres://postgres.xxxx:PASSWORD@db.xxxx.supabase.co:5432/postgres" \
  --no-owner --no-acl -Fc -f kuberan_backup.dump

# 2. Update .env.prod to Option A (self-hosted) and redeploy
./deploy/deploy.sh

# 3. Restore into the local container
docker compose -f docker-compose.prod.yml exec -T postgres \
  pg_restore -U kuberan -d kuberan --no-owner --clean \
  < kuberan_backup.dump
```

### Self-Hosted → Supabase

```bash
# 1. Dump from local container
docker compose -f docker-compose.prod.yml exec -T postgres \
  pg_dump -U kuberan kuberan --no-owner --no-acl -Fc \
  > kuberan_backup.dump

# 2. Restore into Supabase (use direct port 5432)
pg_restore "postgres://postgres.xxxx:PASSWORD@db.xxxx.supabase.co:5432/postgres" \
  --no-owner --no-acl --clean -1 < kuberan_backup.dump

# 3. Update .env.prod to Option B (Supabase) and redeploy
./deploy/deploy.sh
```

---

## Automated Backups

Kuberan includes a `backup` service that runs `pg_dump` on a schedule and stores `.dump` files locally. It works with both self-hosted PostgreSQL and Supabase.

### How It Works

- A lightweight Alpine container runs [supercronic](https://github.com/aptible/supercronic) (a container-friendly cron)
- Executes `pg_dump` daily at **2:00 AM UTC**
- Stores compressed custom-format (`.dump`) backups in `./backups/` on the host
- Automatically prunes backups older than **30 days**
- Runs an initial backup immediately on container start
- All output is logged to stdout (visible via `docker logs`)

> **Note:** the backup service dumps only the app database (`DB_NAME`). Hydra's OAuth database is not backed up; losing it means MCP clients must re-register and re-authorize, but no financial data is affected.

### Enable the Backup Service

Add `backup` to `COMPOSE_PROFILES` in your `.env.prod`:

```bash
# Supabase + backups
COMPOSE_PROFILES=backup

# Self-hosted PostgreSQL + backups
COMPOSE_PROFILES=postgres,backup
```

Then deploy as usual:

```bash
./deploy/deploy.sh
```

The deploy script automatically creates the `backups/` directory if it doesn't exist.

### Configuration

The backup service uses the same `DB_*` variables from `.env.prod`. It overrides `DB_PORT=5432` internally because `pg_dump` requires a direct connection (not a connection pooler).

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKUP_RETENTION_DAYS` | `30` | Days to keep backups before pruning |

To customize retention, edit the `environment` section of the `backup` service in `docker-compose.prod.yml`.

### Managing Backups

```bash
# Check backup logs
docker compose -f docker-compose.prod.yml logs backup

# Run a manual backup (outside the daily schedule)
docker compose -f docker-compose.prod.yml exec backup /backup.sh

# List all backups
ls -lah backups/

# Stop the backup service
docker compose -f docker-compose.prod.yml --profile backup stop backup
```

### Restoring from a Backup

**Into self-hosted PostgreSQL:**
```bash
docker compose -f docker-compose.prod.yml exec -T postgres \
  pg_restore -U kuberan -d kuberan --no-owner --clean \
  < backups/kuberan_YYYYMMDD_HHMMSS.dump
```

**Into Supabase (use direct port 5432):**
```bash
pg_restore "postgres://postgres.xxxx:PASSWORD@db.xxxx.supabase.co:5432/postgres" \
  --no-owner --no-acl --clean -1 < backups/kuberan_YYYYMMDD_HHMMSS.dump
```

**Into any PostgreSQL instance:**
```bash
pg_restore -h <host> -p <port> -U <user> -d <dbname> \
  --no-owner --clean backups/kuberan_YYYYMMDD_HHMMSS.dump
```
