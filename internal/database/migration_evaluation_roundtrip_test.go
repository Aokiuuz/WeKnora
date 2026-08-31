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

	require.NoError(t, migrator.Steps(-4))
	version, dirty, err := migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(12), version)
	require.False(t, dirty)

	inspectionDB := openSQLiteDB(t, dbPath)
	require.False(t, sqliteTableExists(t, inspectionDB, "evaluation_tasks"))

	require.NoError(t, migrator.Steps(4))
	version, dirty, err = migrator.Version()
	require.NoError(t, err)
	require.Equal(t, uint(expectedSQLiteMigrationVersion), version)
	require.False(t, dirty)
	assertSQLiteEvaluationTaskSchema(t, inspectionDB)
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

	schema := fmt.Sprintf("m2_migration_roundtrip_%d", time.Now().UnixNano())
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
	require.Equal(t, uint(93), version)
	require.False(t, dirty)
	assertPostgresEvaluationSchema(t, adminDB, schema)

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
	for _, column := range versionedSQLiteColumns["evaluation_tasks"] {
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
