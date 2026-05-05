package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	backups_core "databasus-backend/internal/features/backups/backups/core"
	"databasus-backend/internal/features/databases"
	"databasus-backend/internal/features/databases/databases/mariadb"
	"databasus-backend/internal/features/databases/databases/mongodb"
	"databasus-backend/internal/features/databases/databases/mysql"
	"databasus-backend/internal/features/databases/databases/postgresql"
	"databasus-backend/internal/features/notifiers"
	"databasus-backend/internal/features/storages"
	"databasus-backend/internal/util/tools"
)

type fakeSender struct {
	calls []*CollectRequest
	err   error
}

func (f *fakeSender) Send(_ context.Context, req *CollectRequest) error {
	f.calls = append(f.calls, req)
	return f.err
}

type fakeDatabaseLister struct {
	databases []*databases.Database
	err       error
}

func (f *fakeDatabaseLister) GetAllDatabases() ([]*databases.Database, error) {
	return f.databases, f.err
}

type fakeStorageLister struct {
	storages []*storages.Storage
	err      error
}

func (f *fakeStorageLister) GetAllStorages() ([]*storages.Storage, error) {
	return f.storages, f.err
}

type fakeNotifierLister struct {
	notifiers []*notifiers.Notifier
	err       error
}

func (f *fakeNotifierLister) GetAllNotifiers() ([]*notifiers.Notifier, error) {
	return f.notifiers, f.err
}

type fakeBackupChecker struct {
	hasBackupSince map[uuid.UUID]bool
	latestBackups  map[uuid.UUID]*backups_core.Backup
	err            error
	latestErr      error
}

func (f *fakeBackupChecker) HasSuccessfulBackupSince(
	databaseID uuid.UUID,
	_ time.Time,
) (bool, error) {
	if f.err != nil {
		return false, f.err
	}

	return f.hasBackupSince[databaseID], nil
}

func (f *fakeBackupChecker) GetLatestCompletedBackup(
	databaseID uuid.UUID,
) (*backups_core.Backup, error) {
	if f.latestErr != nil {
		return nil, f.latestErr
	}

	return f.latestBackups[databaseID], nil
}

func newServiceUnderTest(
	t *testing.T,
	databaseLister databaseLister,
	storageLister storageLister,
	notifierLister notifierLister,
	backupChecker backupChecker,
	sender TelemetrySender,
) *TelemetryService {
	t.Helper()
	loader := NewInstanceFileLoader(
		filepath.Join(t.TempDir(), "instance.json"),
		slog.New(slog.DiscardHandler),
	)

	return NewTelemetryService(
		loader,
		sender,
		databaseLister,
		storageLister,
		notifierLister,
		backupChecker,
		"9.9.9",
		slog.New(slog.DiscardHandler),
	)
}

func availableStatus() *databases.HealthStatus {
	s := databases.HealthStatusAvailable
	return &s
}

func unavailableStatus() *databases.HealthStatus {
	s := databases.HealthStatusUnavailable
	return &s
}

func postgresDatabase(name string, status *databases.HealthStatus) *databases.Database {
	return &databases.Database{
		ID:           uuid.New(),
		Name:         name,
		Type:         databases.DatabaseTypePostgres,
		HealthStatus: status,
		Postgresql: &postgresql.PostgresqlDatabase{
			Version: tools.PostgresqlVersion("16"),
		},
	}
}

func Test_BuildAndSend_ProducesExpectedRequest(t *testing.T) {
	pgDB := postgresDatabase("pg", availableStatus())
	mysqlDB := &databases.Database{
		ID:           uuid.New(),
		Name:         "my",
		Type:         databases.DatabaseTypeMysql,
		HealthStatus: availableStatus(),
		Mysql:        &mysql.MysqlDatabase{Version: tools.MysqlVersion("8.0")},
	}
	mariaDB := &databases.Database{
		ID:           uuid.New(),
		Name:         "maria",
		Type:         databases.DatabaseTypeMariadb,
		HealthStatus: availableStatus(),
		Mariadb:      &mariadb.MariadbDatabase{Version: tools.MariadbVersion("10.6")},
	}
	mongoDB := &databases.Database{
		ID:           uuid.New(),
		Name:         "mongo",
		Type:         databases.DatabaseTypeMongodb,
		HealthStatus: availableStatus(),
		Mongodb:      &mongodb.MongodbDatabase{Version: tools.MongodbVersion("6.0")},
	}

	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{databases: []*databases.Database{pgDB, mysqlDB, mariaDB, mongoDB}},
		&fakeStorageLister{storages: []*storages.Storage{
			{Type: storages.StorageTypeS3},
			{Type: storages.StorageTypeLocal},
		}},
		&fakeNotifierLister{notifiers: []*notifiers.Notifier{
			{NotifierType: notifiers.NotifierTypeEmail},
			{NotifierType: notifiers.NotifierTypeTelegram},
		}},
		&fakeBackupChecker{},
		sender,
	)

	require.NoError(t, service.BuildAndSend(context.Background()))
	require.Len(t, sender.calls, 1)

	req := sender.calls[0]
	assert.Equal(t, "9.9.9", req.AppVersion)
	assert.Equal(t, runtime.GOOS, req.OS)
	assert.Equal(t, runtime.GOARCH, req.Arch)
	require.Len(t, req.Databases, 4)

	types := make([]string, 0, len(req.Databases))
	for _, d := range req.Databases {
		types = append(types, d.Type)
	}
	assert.ElementsMatch(t,
		[]string{"POSTGRES", "MYSQL", "MARIADB", "MONGODB"},
		types,
	)

	assert.Equal(t, []string{"LOCAL", "S3"}, req.Storages)
	assert.Equal(t, []string{"EMAIL", "TELEGRAM"}, req.Notifiers)
	assert.Equal(t, time.Now().UTC().Format("2006-01-02"), req.InstalledAt)
	_, err := uuid.Parse(req.InstanceID)
	require.NoError(t, err)
}

func Test_BuildAndSend_PreservesStorageAndNotifierDuplicates(t *testing.T) {
	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{},
		&fakeStorageLister{storages: []*storages.Storage{
			{Type: storages.StorageTypeS3},
			{Type: storages.StorageTypeS3},
			{Type: storages.StorageTypeS3},
			{Type: storages.StorageTypeLocal},
		}},
		&fakeNotifierLister{notifiers: []*notifiers.Notifier{
			{NotifierType: notifiers.NotifierTypeEmail},
			{NotifierType: notifiers.NotifierTypeEmail},
			{NotifierType: notifiers.NotifierTypeTelegram},
		}},
		&fakeBackupChecker{},
		sender,
	)

	require.NoError(t, service.BuildAndSend(context.Background()))
	require.Len(t, sender.calls, 1)

	assert.Equal(t, []string{"LOCAL", "S3", "S3", "S3"}, sender.calls[0].Storages)
	assert.Equal(t, []string{"EMAIL", "EMAIL", "TELEGRAM"}, sender.calls[0].Notifiers)
}

func Test_BuildAndSend_WhenInstanceFileFails_DoesNotCallSender(t *testing.T) {
	// Construct a loader pointing at an unwritable path so LoadOrCreate returns false.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, writeFileForTest(blocker))

	sender := &fakeSender{}
	loader := NewInstanceFileLoader(
		filepath.Join(blocker, "nested", "instance.json"),
		slog.New(slog.DiscardHandler),
	)

	service := NewTelemetryService(
		loader,
		sender,
		&fakeDatabaseLister{},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{},
		"9.9.9",
		slog.New(slog.DiscardHandler),
	)

	require.NoError(t, service.BuildAndSend(context.Background()))
	assert.Empty(t, sender.calls)
}

func Test_BuildAndSend_WhenSenderFails_PropagatesError(t *testing.T) {
	sendErr := errors.New("network down")
	sender := &fakeSender{err: sendErr}

	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{},
		sender,
	)

	err := service.BuildAndSend(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, sendErr)
}

func Test_BuildAndSend_WhenDbHealthStatusAvailable_DbIncluded(t *testing.T) {
	db := postgresDatabase("pg", availableStatus())

	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{databases: []*databases.Database{db}},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{},
		sender,
	)

	require.NoError(t, service.BuildAndSend(context.Background()))
	require.Len(t, sender.calls, 1)
	assert.Len(t, sender.calls[0].Databases, 1)
}

func Test_BuildAndSend_WhenDbHealthStatusUnavailable_DbExcluded(t *testing.T) {
	db := postgresDatabase("pg", unavailableStatus())

	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{databases: []*databases.Database{db}},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{},
		sender,
	)

	require.NoError(t, service.BuildAndSend(context.Background()))
	require.Len(t, sender.calls, 1)
	assert.Empty(t, sender.calls[0].Databases)
}

func Test_BuildAndSend_WhenHealthcheckOffAndRecentBackup_DbIncluded(t *testing.T) {
	db := postgresDatabase("pg", nil)

	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{databases: []*databases.Database{db}},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{hasBackupSince: map[uuid.UUID]bool{db.ID: true}},
		sender,
	)

	require.NoError(t, service.BuildAndSend(context.Background()))
	require.Len(t, sender.calls, 1)
	require.Len(t, sender.calls[0].Databases, 1)
	assert.Equal(t, "POSTGRES", sender.calls[0].Databases[0].Type)
}

func Test_BuildAndSend_WhenHealthcheckOffAndNoRecentBackup_DbExcluded(t *testing.T) {
	db := postgresDatabase("pg", nil)

	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{databases: []*databases.Database{db}},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{hasBackupSince: map[uuid.UUID]bool{}},
		sender,
	)

	require.NoError(t, service.BuildAndSend(context.Background()))
	require.Len(t, sender.calls, 1)
	assert.Empty(t, sender.calls[0].Databases)
}

func Test_BuildAndSend_WhenBackupCheckerFails_ReturnsError(t *testing.T) {
	db := postgresDatabase("pg", nil)
	checkerErr := errors.New("db down")

	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{databases: []*databases.Database{db}},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{err: checkerErr},
		sender,
	)

	err := service.BuildAndSend(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, checkerErr)
	assert.Empty(t, sender.calls)
}

func Test_BuildAndSend_WhenLatestBackupHasBothSizes_IncludesBoth(t *testing.T) {
	db := postgresDatabase("pg", availableStatus())

	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{databases: []*databases.Database{db}},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{
			latestBackups: map[uuid.UUID]*backups_core.Backup{
				db.ID: {BackupSizeMb: 870.4, BackupRawDbSizeMb: 4321.7},
			},
		},
		sender,
	)

	require.NoError(t, service.BuildAndSend(context.Background()))
	require.Len(t, sender.calls, 1)
	require.Len(t, sender.calls[0].Databases, 1)

	entry := sender.calls[0].Databases[0]
	assert.Equal(t, int64(870), entry.BackupSizeMb)
	assert.Equal(t, int64(4322), entry.RawSizeMb)
}

func Test_BuildAndSend_WhenRawSizeZero_IncludesOnlyBackupSize(t *testing.T) {
	db := postgresDatabase("pg", availableStatus())

	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{databases: []*databases.Database{db}},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{
			latestBackups: map[uuid.UUID]*backups_core.Backup{
				db.ID: {BackupSizeMb: 100, BackupRawDbSizeMb: 0},
			},
		},
		sender,
	)

	require.NoError(t, service.BuildAndSend(context.Background()))
	require.Len(t, sender.calls, 1)
	require.Len(t, sender.calls[0].Databases, 1)

	entry := sender.calls[0].Databases[0]
	assert.Equal(t, int64(100), entry.BackupSizeMb)
	assert.Equal(t, int64(0), entry.RawSizeMb)

	encoded, err := json.Marshal(entry)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "rawSizeMb")
	assert.Contains(t, string(encoded), "backupSizeMb")
}

func Test_BuildAndSend_WhenBackupSizeZero_IncludesOnlyRawSize(t *testing.T) {
	db := postgresDatabase("pg", availableStatus())

	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{databases: []*databases.Database{db}},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{
			latestBackups: map[uuid.UUID]*backups_core.Backup{
				db.ID: {BackupSizeMb: 0, BackupRawDbSizeMb: 999},
			},
		},
		sender,
	)

	require.NoError(t, service.BuildAndSend(context.Background()))
	require.Len(t, sender.calls, 1)
	require.Len(t, sender.calls[0].Databases, 1)

	entry := sender.calls[0].Databases[0]
	assert.Equal(t, int64(0), entry.BackupSizeMb)
	assert.Equal(t, int64(999), entry.RawSizeMb)
}

func Test_BuildAndSend_WhenNoCompletedBackup_OmitsBothSizes(t *testing.T) {
	db := postgresDatabase("pg", availableStatus())

	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{databases: []*databases.Database{db}},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{},
		sender,
	)

	require.NoError(t, service.BuildAndSend(context.Background()))
	require.Len(t, sender.calls, 1)
	require.Len(t, sender.calls[0].Databases, 1)

	entry := sender.calls[0].Databases[0]
	assert.Equal(t, int64(0), entry.BackupSizeMb)
	assert.Equal(t, int64(0), entry.RawSizeMb)

	encoded, err := json.Marshal(entry)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "rawSizeMb")
	assert.NotContains(t, string(encoded), "backupSizeMb")
}

func Test_BuildAndSend_WhenLatestBackupLookupFails_ReturnsError(t *testing.T) {
	db := postgresDatabase("pg", availableStatus())
	lookupErr := errors.New("query exploded")

	sender := &fakeSender{}
	service := newServiceUnderTest(
		t,
		&fakeDatabaseLister{databases: []*databases.Database{db}},
		&fakeStorageLister{},
		&fakeNotifierLister{},
		&fakeBackupChecker{latestErr: lookupErr},
		sender,
	)

	err := service.BuildAndSend(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, lookupErr)
	assert.Empty(t, sender.calls)
}

func writeFileForTest(path string) error {
	return os.WriteFile(path, []byte("x"), 0o600)
}
