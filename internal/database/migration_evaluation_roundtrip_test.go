package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	sqlite3migrate "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestSQLiteEvaluationMigrationsRoundTrip(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	chdirAndRestore(t, repoRoot)

	dbPath := filepath.Join(t.TempDir(), "evaluation-roundtrip.db")
	require.NoError(t, RunMigrationsWithOptions(
		"sqlite3://unused",
		MigrationOptions{SQLiteDBPath: dbPath},
	))

	sqlDB, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	driver, err := sqlite3migrate.WithInstance(sqlDB, &sqlite3migrate.Config{})
	require.NoError(t, err)
	migrator, err := migrate.NewWithDatabaseInstance("file://migrations/sqlite", "sqlite3", driver)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})

	version, dirty, err := migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(23), version)
	require.False(t, dirty)
	inspectionDB := openSQLiteDB(t, dbPath)
	assertSQLiteEvaluationTaskSchema(t, inspectionDB)
	assertSQLiteEvaluationQuestionResultsSchema(t, inspectionDB)
	assertSQLiteEvaluationTaskLabelsSchema(t, inspectionDB)
	assertSQLiteEvaluationRuntimeMetricsSchema(t, inspectionDB)
	assertSQLiteModelObservabilitySchema(t, inspectionDB)
	assertSQLiteEmbeddingCacheSchema(t, inspectionDB)

	require.NoError(t, migrator.Steps(-1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(22), version)
	require.False(t, dirty)
	require.False(t, sqliteTableExists(t, inspectionDB, "embedding_cache_entries"))

	require.NoError(t, migrator.Steps(-1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(21), version)
	require.False(t, dirty)
	require.False(t, sqliteTableExists(t, inspectionDB, "model_call_records"))
	require.False(t, sqliteTableExists(t, inspectionDB, "model_price_versions"))

	require.NoError(t, migrator.Steps(-1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(20), version)
	require.False(t, dirty)
	require.False(t, sqliteColumnExists(t, inspectionDB, "evaluation_tasks", "runtime_metrics"))
	require.False(t, sqliteColumnExists(t, inspectionDB, "evaluation_question_results", "usage_reported"))

	require.NoError(t, migrator.Steps(-1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(19), version)
	require.False(t, dirty)
	require.False(t, sqliteTableExists(t, inspectionDB, "evaluation_task_labels"))

	require.NoError(t, migrator.Steps(-3))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(16), version)
	require.False(t, dirty)
	assertSQLiteM2EvaluationTaskSchema(t, inspectionDB)
	require.False(t, sqliteTableExists(t, inspectionDB, "evaluation_datasets"))
	require.False(t, sqliteTableExists(t, inspectionDB, "evaluation_question_results"))

	require.NoError(t, migrator.Steps(-4))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(12), version)
	require.False(t, dirty)
	require.False(t, sqliteTableExists(t, inspectionDB, "evaluation_tasks"))

	require.NoError(t, migrator.Steps(4))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(16), version)
	require.False(t, dirty)
	assertSQLiteM2EvaluationTaskSchema(t, inspectionDB)

	require.NoError(t, migrator.Steps(3))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(19), version)
	require.False(t, dirty)
	assertSQLiteEvaluationTaskSchema(t, inspectionDB)
	assertSQLiteEvaluationQuestionResultsSchema(t, inspectionDB)

	require.NoError(t, migrator.Steps(1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(20), version)
	require.False(t, dirty)
	assertSQLiteEvaluationTaskLabelsSchema(t, inspectionDB)

	require.NoError(t, migrator.Steps(1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(21), version)
	require.False(t, dirty)
	assertSQLiteEvaluationRuntimeMetricsSchema(t, inspectionDB)

	require.NoError(t, migrator.Steps(1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(22), version)
	require.False(t, dirty)
	assertSQLiteModelObservabilitySchema(t, inspectionDB)

	require.NoError(t, migrator.Steps(1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(expectedSQLiteMigrationVersion), version)
	require.False(t, dirty)
	assertSQLiteEmbeddingCacheSchema(t, inspectionDB)
}

func TestPostgresMigrationsCreateAndRoundTripEvaluationSchema(t *testing.T) {
	baseDSN := os.Getenv("TEST_POSTGRES_MIGRATION_DSN")
	if baseDSN == "" {
		t.Skip("TEST_POSTGRES_MIGRATION_DSN is not configured")
	}
	repoRoot := sqliteRepoRoot(t)
	chdirAndRestore(t, repoRoot)

	adminDB, err := gorm.Open(postgres.Open(baseDSN), &gorm.Config{})
	require.NoError(t, err)
	adminSQLDB, err := adminDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminSQLDB.Close() })

	schema := fmt.Sprintf("m4_migration_roundtrip_%d", time.Now().UnixNano())
	require.NoError(t, adminDB.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		_ = adminDB.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error
	})

	migrationDSN := postgresMigrationDSN(t, baseDSN, schema)
	require.NoError(t, RunMigrations(migrationDSN))

	migrator, err := migrate.New("file://migrations/versioned", migrationDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})
	version, dirty, err := migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(100), version)
	require.False(t, dirty)
	assertPostgresEvaluationSchema(t, adminDB, schema)
	assertPostgresM3EvaluationSchema(t, adminDB, schema)
	assertPostgresM4EvaluationSchema(t, adminDB, schema)
	assertPostgresM5RuntimeSchema(t, adminDB, schema)
	assertPostgresM5LedgerSchema(t, adminDB, schema)
	assertPostgresM5EmbeddingCacheSchema(t, adminDB, schema)

	require.NoError(t, migrator.Steps(-1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(99), version)
	require.False(t, dirty)
	var cacheTableExists bool
	require.NoError(t, adminDB.Raw(
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables "+
			"WHERE table_schema = ? AND table_name = 'embedding_cache_entries')", schema,
	).Scan(&cacheTableExists).Error)
	require.False(t, cacheTableExists)

	require.NoError(t, migrator.Steps(-1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(98), version)
	require.False(t, dirty)
	var ledgerTableExists bool
	require.NoError(t, adminDB.Raw(
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables "+
			"WHERE table_schema = ? AND table_name = 'model_call_records')", schema,
	).Scan(&ledgerTableExists).Error)
	require.False(t, ledgerTableExists)

	require.NoError(t, migrator.Steps(-1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(97), version)
	require.False(t, dirty)
	var runtimeColumnExists bool
	require.NoError(t, adminDB.Raw(
		"SELECT EXISTS (SELECT 1 FROM information_schema.columns "+
			"WHERE table_schema = ? AND table_name = 'evaluation_tasks' AND column_name = 'runtime_metrics')",
		schema,
	).Scan(&runtimeColumnExists).Error)
	require.False(t, runtimeColumnExists)

	require.NoError(t, migrator.Steps(-1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(96), version)
	require.False(t, dirty)
	var labelsExist bool
	require.NoError(t, adminDB.Raw(
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables "+
			"WHERE table_schema = ? AND table_name = 'evaluation_task_labels')",
		schema,
	).Scan(&labelsExist).Error)
	require.False(t, labelsExist)

	require.NoError(t, migrator.Steps(-3))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(93), version)
	require.False(t, dirty)
	assertPostgresEvaluationSchema(t, adminDB, schema)
	for _, table := range []string{"evaluation_datasets", "evaluation_question_results"} {
		var exists bool
		require.NoError(t, adminDB.Raw(
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables "+
				"WHERE table_schema = ? AND table_name = ?)", schema, table,
		).Scan(&exists).Error)
		require.Falsef(t, exists, "PostgreSQL table %s must be absent at version 93", table)
	}

	require.NoError(t, migrator.Steps(-4))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(89), version)
	require.False(t, dirty)
	var evaluationTableExists bool
	require.NoError(t, adminDB.Raw(
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables "+
			"WHERE table_schema = ? AND table_name = 'evaluation_tasks')",
		schema,
	).Scan(&evaluationTableExists).Error)
	require.False(t, evaluationTableExists)

	require.NoError(t, migrator.Steps(4))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(93), version)
	require.False(t, dirty)
	assertPostgresEvaluationSchema(t, adminDB, schema)

	require.NoError(t, migrator.Steps(3))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(96), version)
	require.False(t, dirty)
	assertPostgresM3EvaluationSchema(t, adminDB, schema)

	require.NoError(t, migrator.Steps(1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(97), version)
	require.False(t, dirty)
	assertPostgresM4EvaluationSchema(t, adminDB, schema)

	require.NoError(t, migrator.Steps(1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(98), version)
	require.False(t, dirty)
	assertPostgresM5RuntimeSchema(t, adminDB, schema)

	require.NoError(t, migrator.Steps(1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(99), version)
	require.False(t, dirty)
	assertPostgresM5LedgerSchema(t, adminDB, schema)

	require.NoError(t, migrator.Steps(1))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(100), version)
	require.False(t, dirty)
	assertPostgresM5EmbeddingCacheSchema(t, adminDB, schema)
}

func assertSQLiteEvaluationRuntimeMetricsSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	require.True(t, sqliteColumnExists(t, db, "evaluation_tasks", "runtime_metrics"))
	require.True(t, sqliteColumnExists(t, db, "evaluation_question_results", "usage_reported"))
}

func assertPostgresM5RuntimeSchema(t *testing.T, db *gorm.DB, schema string) {
	t.Helper()
	for table, column := range map[string]string{
		"evaluation_tasks":            "runtime_metrics",
		"evaluation_question_results": "usage_reported",
	} {
		var exists bool
		require.NoError(t, db.Raw(
			"SELECT EXISTS (SELECT 1 FROM information_schema.columns "+
				"WHERE table_schema = ? AND table_name = ? AND column_name = ?)",
			schema, table, column,
		).Scan(&exists).Error)
		require.Truef(t, exists, "PostgreSQL %s must contain column %s", table, column)
	}
}

func assertPostgresM5LedgerSchema(t *testing.T, db *gorm.DB, schema string) {
	t.Helper()
	for _, table := range []string{"model_price_versions", "model_call_records"} {
		var exists bool
		require.NoError(t, db.Raw(
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = ? AND table_name = ?)",
			schema, table,
		).Scan(&exists).Error)
		require.Truef(t, exists, "PostgreSQL must contain table %s", table)
	}
}

func assertPostgresM5EmbeddingCacheSchema(t *testing.T, db *gorm.DB, schema string) {
	t.Helper()
	var dataType string
	require.NoError(t, db.Raw(
		"SELECT data_type FROM information_schema.columns "+
			"WHERE table_schema = ? AND table_name = 'embedding_cache_entries' AND column_name = 'embedding'",
		schema,
	).Scan(&dataType).Error)
	require.Equal(t, "bytea", dataType)
	var textColumnExists bool
	require.NoError(t, db.Raw(
		"SELECT EXISTS (SELECT 1 FROM information_schema.columns "+
			"WHERE table_schema = ? AND table_name = 'embedding_cache_entries' AND column_name = 'text')",
		schema,
	).Scan(&textColumnExists).Error)
	require.False(t, textColumnExists)
}

func postgresMigrationDSN(t *testing.T, baseDSN, schema string) string {
	t.Helper()
	parsed, err := url.Parse(baseDSN)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	query.Set("options", "-c app.skip_embedding=true")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func assertPostgresEvaluationSchema(t *testing.T, db *gorm.DB, schema string) {
	t.Helper()
	for _, column := range []string{
		"tenant_id", "dataset_id", "status", "start_time", "end_time", "total", "finished", "err_msg",
		"cleanup_errors", "params", "metric", "temporary_kb_id", "temporary_knowledge_id", "owner_id",
		"lease_expires_at", "heartbeat_at", "version", "created_at", "updated_at", "deleted_at",
		"cancel_requested_at",
	} {
		var exists bool
		require.NoError(t, db.Raw(
			"SELECT EXISTS (SELECT 1 FROM information_schema.columns "+
				"WHERE table_schema = ? AND table_name = 'evaluation_tasks' AND column_name = ?)",
			schema,
			column,
		).Scan(&exists).Error)
		require.Truef(t, exists, "PostgreSQL evaluation_tasks must contain column %s", column)
	}
	for _, index := range []string{
		"idx_evaluation_tasks_tenant_started",
		"idx_evaluation_tasks_tenant_status_started",
		"idx_evaluation_tasks_active_lease",
		"idx_evaluation_tasks_retention",
	} {
		var exists bool
		require.NoError(t, db.Raw(
			"SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = ? AND indexname = ?)",
			schema,
			index,
		).Scan(&exists).Error)
		require.Truef(t, exists, "PostgreSQL evaluation_tasks must contain index %s", index)
	}
}

func assertSQLiteM2EvaluationTaskSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	require.True(t, sqliteTableExists(t, db, "evaluation_tasks"))
	for _, column := range []string{
		"owner_id", "lease_expires_at", "heartbeat_at", "version", "cancel_requested_at", "deleted_at",
	} {
		require.Truef(t, sqliteColumnExists(t, db, "evaluation_tasks", column),
			"SQLite evaluation_tasks must contain M2 column %s", column)
	}
	assertSQLitePartialIndex(t, db, "idx_evaluation_tasks_tenant_started", "WHERE deleted_at IS NULL")
	assertSQLitePartialIndex(t, db, "idx_evaluation_tasks_tenant_status_started", "WHERE deleted_at IS NULL")
	assertSQLitePartialIndex(t, db, "idx_evaluation_tasks_active_lease",
		"WHERE deleted_at IS NULL AND status IN (0, 1)")
	assertSQLitePartialIndex(t, db, "idx_evaluation_tasks_retention",
		"WHERE status IN (2, 3, 4, 5, 6) AND end_time IS NOT NULL")
}

func assertPostgresM3EvaluationSchema(t *testing.T, db *gorm.DB, schema string) {
	t.Helper()
	for _, table := range []string{
		"evaluation_datasets",
		"evaluation_dataset_versions",
		"evaluation_dataset_passages",
		"evaluation_dataset_questions",
		"evaluation_dataset_relevance",
		"evaluation_question_results",
	} {
		var exists bool
		require.NoError(t, db.Raw(
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables "+
				"WHERE table_schema = ? AND table_name = ?)", schema, table,
		).Scan(&exists).Error)
		require.Truef(t, exists, "PostgreSQL migrations must create table %s", table)
	}
	for _, column := range []string{
		"dataset_version_id", "dataset_content_sha256", "experiment_snapshot", "experiment_sha256",
	} {
		var exists bool
		require.NoError(t, db.Raw(
			"SELECT EXISTS (SELECT 1 FROM information_schema.columns "+
				"WHERE table_schema = ? AND table_name = 'evaluation_tasks' AND column_name = ?)",
			schema, column,
		).Scan(&exists).Error)
		require.Truef(t, exists, "PostgreSQL evaluation_tasks must contain M3 column %s", column)
	}
}

func assertPostgresM4EvaluationSchema(t *testing.T, db *gorm.DB, schema string) {
	t.Helper()
	var exists bool
	require.NoError(t, db.Raw(
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables "+
			"WHERE table_schema = ? AND table_name = 'evaluation_task_labels')",
		schema,
	).Scan(&exists).Error)
	require.True(t, exists)
	require.NoError(t, db.Raw(
		"SELECT EXISTS (SELECT 1 FROM pg_constraint c "+
			"JOIN pg_namespace n ON n.oid = c.connamespace "+
			"WHERE n.nspname = ? AND c.conname = 'evaluation_task_labels_label_bytes_check')",
		schema,
	).Scan(&exists).Error)
	require.True(t, exists, "PostgreSQL labels must enforce the 64-byte limit")
	for _, index := range []string{
		"idx_evaluation_task_labels_tenant_label_task",
		"idx_evaluation_tasks_tenant_dataset_started",
		"idx_evaluation_tasks_tenant_dataset_version_started",
	} {
		require.NoError(t, db.Raw(
			"SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = ? AND indexname = ?)",
			schema,
			index,
		).Scan(&exists).Error)
		require.Truef(t, exists, "PostgreSQL M4 index %s must exist", index)
	}
}
