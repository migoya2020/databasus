package mariadb

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"

	"databasus-backend/internal/config"
	"databasus-backend/internal/util/tools"
)

func Test_TestConnection_InsufficientPermissions_ReturnsError(t *testing.T) {
	env := config.GetEnv()
	cases := []struct {
		name    string
		version tools.MariadbVersion
		port    string
	}{
		{"MariaDB 5.5", tools.MariadbVersion55, env.TestMariadb55Port},
		{"MariaDB 10.1", tools.MariadbVersion101, env.TestMariadb101Port},
		{"MariaDB 10.2", tools.MariadbVersion102, env.TestMariadb102Port},
		{"MariaDB 10.3", tools.MariadbVersion103, env.TestMariadb103Port},
		{"MariaDB 10.4", tools.MariadbVersion104, env.TestMariadb104Port},
		{"MariaDB 10.5", tools.MariadbVersion105, env.TestMariadb105Port},
		{"MariaDB 10.6", tools.MariadbVersion106, env.TestMariadb106Port},
		{"MariaDB 10.11", tools.MariadbVersion1011, env.TestMariadb1011Port},
		{"MariaDB 11.4", tools.MariadbVersion114, env.TestMariadb114Port},
		{"MariaDB 11.8", tools.MariadbVersion118, env.TestMariadb118Port},
		{"MariaDB 12.0", tools.MariadbVersion120, env.TestMariadb120Port},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			container := connectToMariadbContainer(t, tc.port, tc.version)
			defer container.DB.Close()

			_, err := container.DB.Exec(`DROP TABLE IF EXISTS permission_test`)
			assert.NoError(t, err)

			_, err = container.DB.Exec(`CREATE TABLE permission_test (
				id INT AUTO_INCREMENT PRIMARY KEY,
				data VARCHAR(255) NOT NULL
			)`)
			assert.NoError(t, err)

			_, err = container.DB.Exec(`INSERT INTO permission_test (data) VALUES ('test1')`)
			assert.NoError(t, err)

			limitedUsername := fmt.Sprintf("limited_%s", uuid.New().String()[:8])
			limitedPassword := "limitedpassword123"

			_, err = container.DB.Exec(fmt.Sprintf(
				"CREATE USER '%s'@'%%' IDENTIFIED BY '%s'",
				limitedUsername,
				limitedPassword,
			))
			assert.NoError(t, err)

			_, err = container.DB.Exec(fmt.Sprintf(
				"GRANT SELECT ON `%s`.* TO '%s'@'%%'",
				container.Database,
				limitedUsername,
			))
			assert.NoError(t, err)

			_, err = container.DB.Exec("FLUSH PRIVILEGES")
			assert.NoError(t, err)

			defer dropUserSafe(container.DB, limitedUsername)

			mariadbModel := &MariadbDatabase{
				Version:  tc.version,
				Host:     container.Host,
				Port:     container.Port,
				Username: limitedUsername,
				Password: limitedPassword,
				Database: &container.Database,
				IsHttps:  false,
			}

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			err = mariadbModel.TestConnection(logger, nil, uuid.New())
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "insufficient permissions")
		})
	}
}

func Test_TestConnection_SufficientPermissions_Success(t *testing.T) {
	env := config.GetEnv()
	cases := []struct {
		name    string
		version tools.MariadbVersion
		port    string
	}{
		{"MariaDB 5.5", tools.MariadbVersion55, env.TestMariadb55Port},
		{"MariaDB 10.1", tools.MariadbVersion101, env.TestMariadb101Port},
		{"MariaDB 10.2", tools.MariadbVersion102, env.TestMariadb102Port},
		{"MariaDB 10.3", tools.MariadbVersion103, env.TestMariadb103Port},
		{"MariaDB 10.4", tools.MariadbVersion104, env.TestMariadb104Port},
		{"MariaDB 10.5", tools.MariadbVersion105, env.TestMariadb105Port},
		{"MariaDB 10.6", tools.MariadbVersion106, env.TestMariadb106Port},
		{"MariaDB 10.11", tools.MariadbVersion1011, env.TestMariadb1011Port},
		{"MariaDB 11.4", tools.MariadbVersion114, env.TestMariadb114Port},
		{"MariaDB 11.8", tools.MariadbVersion118, env.TestMariadb118Port},
		{"MariaDB 12.0", tools.MariadbVersion120, env.TestMariadb120Port},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			container := connectToMariadbContainer(t, tc.port, tc.version)
			defer container.DB.Close()

			_, err := container.DB.Exec(`DROP TABLE IF EXISTS backup_test`)
			assert.NoError(t, err)

			_, err = container.DB.Exec(`CREATE TABLE backup_test (
				id INT AUTO_INCREMENT PRIMARY KEY,
				data VARCHAR(255) NOT NULL
			)`)
			assert.NoError(t, err)

			_, err = container.DB.Exec(`INSERT INTO backup_test (data) VALUES ('test1')`)
			assert.NoError(t, err)

			backupUsername := fmt.Sprintf("backup_%s", uuid.New().String()[:8])
			backupPassword := "backuppassword123"

			_, err = container.DB.Exec(fmt.Sprintf(
				"CREATE USER '%s'@'%%' IDENTIFIED BY '%s'",
				backupUsername,
				backupPassword,
			))
			assert.NoError(t, err)

			_, err = container.DB.Exec(fmt.Sprintf(
				"GRANT SELECT, SHOW VIEW, LOCK TABLES, TRIGGER, EVENT ON `%s`.* TO '%s'@'%%'",
				container.Database,
				backupUsername,
			))
			assert.NoError(t, err)

			_, err = container.DB.Exec(fmt.Sprintf(
				"GRANT PROCESS ON *.* TO '%s'@'%%'",
				backupUsername,
			))
			assert.NoError(t, err)

			_, err = container.DB.Exec("FLUSH PRIVILEGES")
			assert.NoError(t, err)

			defer dropUserSafe(container.DB, backupUsername)

			mariadbModel := &MariadbDatabase{
				Version:  tc.version,
				Host:     container.Host,
				Port:     container.Port,
				Username: backupUsername,
				Password: backupPassword,
				Database: &container.Database,
				IsHttps:  false,
			}

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			err = mariadbModel.TestConnection(logger, nil, uuid.New())
			assert.NoError(t, err)
		})
	}
}

func Test_IsUserReadOnly_AdminUser_ReturnsFalse(t *testing.T) {
	env := config.GetEnv()
	cases := []struct {
		name    string
		version tools.MariadbVersion
		port    string
	}{
		{"MariaDB 5.5", tools.MariadbVersion55, env.TestMariadb55Port},
		{"MariaDB 10.1", tools.MariadbVersion101, env.TestMariadb101Port},
		{"MariaDB 10.2", tools.MariadbVersion102, env.TestMariadb102Port},
		{"MariaDB 10.3", tools.MariadbVersion103, env.TestMariadb103Port},
		{"MariaDB 10.4", tools.MariadbVersion104, env.TestMariadb104Port},
		{"MariaDB 10.5", tools.MariadbVersion105, env.TestMariadb105Port},
		{"MariaDB 10.6", tools.MariadbVersion106, env.TestMariadb106Port},
		{"MariaDB 10.11", tools.MariadbVersion1011, env.TestMariadb1011Port},
		{"MariaDB 11.4", tools.MariadbVersion114, env.TestMariadb114Port},
		{"MariaDB 11.8", tools.MariadbVersion118, env.TestMariadb118Port},
		{"MariaDB 12.0", tools.MariadbVersion120, env.TestMariadb120Port},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			container := connectToMariadbContainer(t, tc.port, tc.version)
			defer container.DB.Close()

			mariadbModel := createMariadbModel(container)
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			ctx := t.Context()

			isReadOnly, privileges, err := mariadbModel.IsUserReadOnly(ctx, logger, nil, uuid.New())
			assert.NoError(t, err)
			assert.False(t, isReadOnly, "Root user should not be read-only")
			assert.NotEmpty(t, privileges, "Root user should have privileges")
		})
	}
}

func Test_IsUserReadOnly_ReadOnlyUser_ReturnsTrue(t *testing.T) {
	env := config.GetEnv()
	container := connectToMariadbContainer(t, env.TestMariadb1011Port, tools.MariadbVersion1011)
	defer container.DB.Close()

	_, err := container.DB.Exec(`DROP TABLE IF EXISTS readonly_check_test`)
	assert.NoError(t, err)

	_, err = container.DB.Exec(`CREATE TABLE readonly_check_test (
			id INT AUTO_INCREMENT PRIMARY KEY,
			data VARCHAR(255) NOT NULL
		)`)
	assert.NoError(t, err)

	_, err = container.DB.Exec(`INSERT INTO readonly_check_test (data) VALUES ('test1')`)
	assert.NoError(t, err)

	mariadbModel := createMariadbModel(container)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := t.Context()

	username, password, err := mariadbModel.CreateReadOnlyUser(ctx, logger, nil, uuid.New())
	assert.NoError(t, err)

	readOnlyModel := &MariadbDatabase{
		Version:  mariadbModel.Version,
		Host:     mariadbModel.Host,
		Port:     mariadbModel.Port,
		Username: username,
		Password: password,
		Database: mariadbModel.Database,
		IsHttps:  false,
	}

	isReadOnly, privileges, err := readOnlyModel.IsUserReadOnly(ctx, logger, nil, uuid.New())
	assert.NoError(t, err)
	assert.True(t, isReadOnly, "Read-only user should be read-only")
	assert.Empty(t, privileges, "Read-only user should have no write privileges")

	dropUserSafe(container.DB, username)
}

func Test_CreateReadOnlyUser_UserCanReadButNotWrite(t *testing.T) {
	env := config.GetEnv()
	cases := []struct {
		name    string
		version tools.MariadbVersion
		port    string
	}{
		{"MariaDB 5.5", tools.MariadbVersion55, env.TestMariadb55Port},
		{"MariaDB 10.1", tools.MariadbVersion101, env.TestMariadb101Port},
		{"MariaDB 10.2", tools.MariadbVersion102, env.TestMariadb102Port},
		{"MariaDB 10.3", tools.MariadbVersion103, env.TestMariadb103Port},
		{"MariaDB 10.4", tools.MariadbVersion104, env.TestMariadb104Port},
		{"MariaDB 10.5", tools.MariadbVersion105, env.TestMariadb105Port},
		{"MariaDB 10.6", tools.MariadbVersion106, env.TestMariadb106Port},
		{"MariaDB 10.11", tools.MariadbVersion1011, env.TestMariadb1011Port},
		{"MariaDB 11.4", tools.MariadbVersion114, env.TestMariadb114Port},
		{"MariaDB 11.8", tools.MariadbVersion118, env.TestMariadb118Port},
		{"MariaDB 12.0", tools.MariadbVersion120, env.TestMariadb120Port},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			container := connectToMariadbContainer(t, tc.port, tc.version)
			defer container.DB.Close()

			_, err := container.DB.Exec(`DROP TABLE IF EXISTS readonly_test`)
			assert.NoError(t, err)
			_, err = container.DB.Exec(`DROP TABLE IF EXISTS hack_table`)
			assert.NoError(t, err)
			_, err = container.DB.Exec(`DROP TABLE IF EXISTS future_table`)
			assert.NoError(t, err)

			_, err = container.DB.Exec(`
				CREATE TABLE readonly_test (
					id INT AUTO_INCREMENT PRIMARY KEY,
					data VARCHAR(255) NOT NULL
				)
			`)
			assert.NoError(t, err)

			_, err = container.DB.Exec(
				`INSERT INTO readonly_test (data) VALUES ('test1'), ('test2')`,
			)
			assert.NoError(t, err)

			mariadbModel := createMariadbModel(container)
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			ctx := t.Context()

			username, password, err := mariadbModel.CreateReadOnlyUser(ctx, logger, nil, uuid.New())
			assert.NoError(t, err)
			assert.NotEmpty(t, username)
			assert.NotEmpty(t, password)
			assert.True(t, strings.HasPrefix(username, "pgs-"))

			if err != nil {
				return
			}

			readOnlyModel := &MariadbDatabase{
				Version:  mariadbModel.Version,
				Host:     mariadbModel.Host,
				Port:     mariadbModel.Port,
				Username: username,
				Password: password,
				Database: mariadbModel.Database,
				IsHttps:  false,
			}

			isReadOnly, privileges, err := readOnlyModel.IsUserReadOnly(
				ctx,
				logger,
				nil,
				uuid.New(),
			)
			assert.NoError(t, err)
			assert.True(t, isReadOnly, "Created user should be read-only")
			assert.Empty(t, privileges, "Read-only user should have no write privileges")

			readOnlyDSN := fmt.Sprintf(
				"%s:%s@tcp(%s:%d)/%s?parseTime=true",
				username,
				password,
				container.Host,
				container.Port,
				container.Database,
			)
			readOnlyConn, err := sqlx.Connect("mysql", readOnlyDSN)
			assert.NoError(t, err)
			defer readOnlyConn.Close()

			var count int
			err = readOnlyConn.Get(&count, "SELECT COUNT(*) FROM readonly_test")
			assert.NoError(t, err)
			assert.Equal(t, 2, count)

			_, err = readOnlyConn.Exec("INSERT INTO readonly_test (data) VALUES ('should-fail')")
			assert.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), "denied")

			_, err = readOnlyConn.Exec("UPDATE readonly_test SET data = 'hacked' WHERE id = 1")
			assert.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), "denied")

			_, err = readOnlyConn.Exec("DELETE FROM readonly_test WHERE id = 1")
			assert.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), "denied")

			_, err = readOnlyConn.Exec("CREATE TABLE hack_table (id INT)")
			assert.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), "denied")

			dropUserSafe(container.DB, username)
		})
	}
}

func Test_ReadOnlyUser_FutureTables_NoSelectPermission(t *testing.T) {
	env := config.GetEnv()
	container := connectToMariadbContainer(t, env.TestMariadb1011Port, tools.MariadbVersion1011)
	defer container.DB.Close()

	mariadbModel := createMariadbModel(container)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := t.Context()

	username, password, err := mariadbModel.CreateReadOnlyUser(ctx, logger, nil, uuid.New())
	assert.NoError(t, err)

	_, err = container.DB.Exec(`DROP TABLE IF EXISTS future_table`)
	assert.NoError(t, err)
	_, err = container.DB.Exec(`
		CREATE TABLE future_table (
			id INT AUTO_INCREMENT PRIMARY KEY,
			data VARCHAR(255) NOT NULL
		)
	`)
	assert.NoError(t, err)
	_, err = container.DB.Exec(`INSERT INTO future_table (data) VALUES ('future_data')`)
	assert.NoError(t, err)

	readOnlyDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		username, password, container.Host, container.Port, container.Database)
	readOnlyConn, err := sqlx.Connect("mysql", readOnlyDSN)
	assert.NoError(t, err)
	defer readOnlyConn.Close()

	var data string
	err = readOnlyConn.Get(&data, "SELECT data FROM future_table LIMIT 1")
	assert.NoError(t, err)
	assert.Equal(t, "future_data", data)

	dropUserSafe(container.DB, username)
}

func Test_CreateReadOnlyUser_DatabaseNameWithDash_Success(t *testing.T) {
	env := config.GetEnv()
	container := connectToMariadbContainer(t, env.TestMariadb1011Port, tools.MariadbVersion1011)
	defer container.DB.Close()

	dashDbName := "test-db-with-dash"

	_, err := container.DB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dashDbName))
	assert.NoError(t, err)

	_, err = container.DB.Exec(fmt.Sprintf("CREATE DATABASE `%s`", dashDbName))
	assert.NoError(t, err)

	defer func() {
		_, _ = container.DB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dashDbName))
	}()

	dashDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		container.Username, container.Password, container.Host, container.Port, dashDbName)
	dashDB, err := sqlx.Connect("mysql", dashDSN)
	assert.NoError(t, err)
	defer dashDB.Close()

	_, err = dashDB.Exec(`
		CREATE TABLE dash_test (
			id INT AUTO_INCREMENT PRIMARY KEY,
			data VARCHAR(255) NOT NULL
		)
	`)
	assert.NoError(t, err)

	_, err = dashDB.Exec(`INSERT INTO dash_test (data) VALUES ('test1'), ('test2')`)
	assert.NoError(t, err)

	mariadbModel := &MariadbDatabase{
		Version:  tools.MariadbVersion1011,
		Host:     container.Host,
		Port:     container.Port,
		Username: container.Username,
		Password: container.Password,
		Database: &dashDbName,
		IsHttps:  false,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := t.Context()

	username, password, err := mariadbModel.CreateReadOnlyUser(ctx, logger, nil, uuid.New())
	assert.NoError(t, err)
	assert.NotEmpty(t, username)
	assert.NotEmpty(t, password)
	assert.True(t, strings.HasPrefix(username, "pgs-"))

	readOnlyDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		username, password, container.Host, container.Port, dashDbName)
	readOnlyConn, err := sqlx.Connect("mysql", readOnlyDSN)
	assert.NoError(t, err)
	defer readOnlyConn.Close()

	var count int
	err = readOnlyConn.Get(&count, "SELECT COUNT(*) FROM dash_test")
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

	_, err = readOnlyConn.Exec("INSERT INTO dash_test (data) VALUES ('should-fail')")
	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "denied")

	dropUserSafe(dashDB, username)
}

func Test_ReadOnlyUser_CannotDropOrAlterTables(t *testing.T) {
	env := config.GetEnv()
	container := connectToMariadbContainer(t, env.TestMariadb1011Port, tools.MariadbVersion1011)
	defer container.DB.Close()

	_, err := container.DB.Exec(`DROP TABLE IF EXISTS drop_test`)
	assert.NoError(t, err)
	_, err = container.DB.Exec(`
		CREATE TABLE drop_test (
			id INT AUTO_INCREMENT PRIMARY KEY,
			data VARCHAR(255) NOT NULL
		)
	`)
	assert.NoError(t, err)
	_, err = container.DB.Exec(`INSERT INTO drop_test (data) VALUES ('test1')`)
	assert.NoError(t, err)

	mariadbModel := createMariadbModel(container)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := t.Context()

	username, password, err := mariadbModel.CreateReadOnlyUser(ctx, logger, nil, uuid.New())
	assert.NoError(t, err)

	readOnlyDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		username, password, container.Host, container.Port, container.Database)
	readOnlyConn, err := sqlx.Connect("mysql", readOnlyDSN)
	assert.NoError(t, err)
	defer readOnlyConn.Close()

	_, err = readOnlyConn.Exec("DROP TABLE drop_test")
	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "denied")

	_, err = readOnlyConn.Exec("ALTER TABLE drop_test ADD COLUMN new_col VARCHAR(100)")
	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "denied")

	_, err = readOnlyConn.Exec("TRUNCATE TABLE drop_test")
	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "denied")

	dropUserSafe(container.DB, username)
}

func Test_TestConnection_DatabaseSpecificPrivilegesWithGlobalProcess_Success(t *testing.T) {
	env := config.GetEnv()
	cases := []struct {
		name    string
		version tools.MariadbVersion
		port    string
	}{
		{"MariaDB 5.5", tools.MariadbVersion55, env.TestMariadb55Port},
		{"MariaDB 10.1", tools.MariadbVersion101, env.TestMariadb101Port},
		{"MariaDB 10.2", tools.MariadbVersion102, env.TestMariadb102Port},
		{"MariaDB 10.3", tools.MariadbVersion103, env.TestMariadb103Port},
		{"MariaDB 10.4", tools.MariadbVersion104, env.TestMariadb104Port},
		{"MariaDB 10.5", tools.MariadbVersion105, env.TestMariadb105Port},
		{"MariaDB 10.6", tools.MariadbVersion106, env.TestMariadb106Port},
		{"MariaDB 10.11", tools.MariadbVersion1011, env.TestMariadb1011Port},
		{"MariaDB 11.4", tools.MariadbVersion114, env.TestMariadb114Port},
		{"MariaDB 11.8", tools.MariadbVersion118, env.TestMariadb118Port},
		{"MariaDB 12.0", tools.MariadbVersion120, env.TestMariadb120Port},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			container := connectToMariadbContainer(t, tc.port, tc.version)
			defer container.DB.Close()

			_, err := container.DB.Exec(`DROP TABLE IF EXISTS privilege_test`)
			assert.NoError(t, err)

			_, err = container.DB.Exec(`CREATE TABLE privilege_test (
				id INT AUTO_INCREMENT PRIMARY KEY,
				data VARCHAR(255) NOT NULL
			)`)
			assert.NoError(t, err)

			_, err = container.DB.Exec(`INSERT INTO privilege_test (data) VALUES ('test1')`)
			assert.NoError(t, err)

			specificUsername := fmt.Sprintf("spec_%s", uuid.New().String()[:8])
			specificPassword := "specificpass123"

			_, err = container.DB.Exec(fmt.Sprintf(
				"CREATE USER '%s'@'%%' IDENTIFIED BY '%s'",
				specificUsername,
				specificPassword,
			))
			assert.NoError(t, err)

			_, err = container.DB.Exec(fmt.Sprintf(
				"GRANT SELECT, SHOW VIEW ON %s.* TO '%s'@'%%'",
				container.Database,
				specificUsername,
			))
			assert.NoError(t, err)

			_, err = container.DB.Exec(fmt.Sprintf(
				"GRANT PROCESS ON *.* TO '%s'@'%%'",
				specificUsername,
			))
			assert.NoError(t, err)

			_, err = container.DB.Exec("FLUSH PRIVILEGES")
			assert.NoError(t, err)

			defer dropUserSafe(container.DB, specificUsername)

			mariadbModel := &MariadbDatabase{
				Version:  tc.version,
				Host:     container.Host,
				Port:     container.Port,
				Username: specificUsername,
				Password: specificPassword,
				Database: &container.Database,
				IsHttps:  false,
			}

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			err = mariadbModel.TestConnection(logger, nil, uuid.New())
			assert.NoError(t, err)
		})
	}
}

func Test_TestConnection_DatabaseWithUnderscores_Success(t *testing.T) {
	env := config.GetEnv()
	container := connectToMariadbContainer(t, env.TestMariadb1011Port, tools.MariadbVersion1011)
	defer container.DB.Close()

	underscoreDbName := "test_db_name"

	_, err := container.DB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", underscoreDbName))
	assert.NoError(t, err)

	_, err = container.DB.Exec(fmt.Sprintf("CREATE DATABASE `%s`", underscoreDbName))
	assert.NoError(t, err)

	defer func() {
		_, _ = container.DB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", underscoreDbName))
	}()

	underscoreDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		container.Username, container.Password, container.Host, container.Port, underscoreDbName)
	underscoreDB, err := sqlx.Connect("mysql", underscoreDSN)
	assert.NoError(t, err)
	defer underscoreDB.Close()

	_, err = underscoreDB.Exec(`
		CREATE TABLE underscore_test (
			id INT AUTO_INCREMENT PRIMARY KEY,
			data VARCHAR(255) NOT NULL
		)
	`)
	assert.NoError(t, err)

	_, err = underscoreDB.Exec(`INSERT INTO underscore_test (data) VALUES ('test1')`)
	assert.NoError(t, err)

	underscoreUsername := fmt.Sprintf("under%s", uuid.New().String()[:8])
	underscorePassword := "underscorepass123"

	_, err = underscoreDB.Exec(fmt.Sprintf(
		"CREATE USER '%s'@'%%' IDENTIFIED BY '%s'",
		underscoreUsername,
		underscorePassword,
	))
	assert.NoError(t, err)

	_, err = underscoreDB.Exec(fmt.Sprintf(
		"GRANT SELECT, SHOW VIEW ON `%s`.* TO '%s'@'%%'",
		underscoreDbName,
		underscoreUsername,
	))
	assert.NoError(t, err)

	_, err = underscoreDB.Exec("FLUSH PRIVILEGES")
	assert.NoError(t, err)

	defer dropUserSafe(underscoreDB, underscoreUsername)

	mariadbModel := &MariadbDatabase{
		Version:  tools.MariadbVersion1011,
		Host:     container.Host,
		Port:     container.Port,
		Username: underscoreUsername,
		Password: underscorePassword,
		Database: &underscoreDbName,
		IsHttps:  false,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	err = mariadbModel.TestConnection(logger, nil, uuid.New())
	assert.NoError(t, err)
}

func Test_TestConnection_DatabaseWithUnderscoresAndAllPrivileges_Success(t *testing.T) {
	env := config.GetEnv()
	cases := []struct {
		name    string
		version tools.MariadbVersion
		port    string
	}{
		{"MariaDB 5.5", tools.MariadbVersion55, env.TestMariadb55Port},
		{"MariaDB 10.1", tools.MariadbVersion101, env.TestMariadb101Port},
		{"MariaDB 10.2", tools.MariadbVersion102, env.TestMariadb102Port},
		{"MariaDB 10.3", tools.MariadbVersion103, env.TestMariadb103Port},
		{"MariaDB 10.4", tools.MariadbVersion104, env.TestMariadb104Port},
		{"MariaDB 10.5", tools.MariadbVersion105, env.TestMariadb105Port},
		{"MariaDB 10.6", tools.MariadbVersion106, env.TestMariadb106Port},
		{"MariaDB 10.11", tools.MariadbVersion1011, env.TestMariadb1011Port},
		{"MariaDB 11.4", tools.MariadbVersion114, env.TestMariadb114Port},
		{"MariaDB 11.8", tools.MariadbVersion118, env.TestMariadb118Port},
		{"MariaDB 12.0", tools.MariadbVersion120, env.TestMariadb120Port},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			container := connectToMariadbContainer(t, tc.port, tc.version)
			defer container.DB.Close()

			underscoreDbName := "test_all_db"

			_, err := container.DB.Exec(
				fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", underscoreDbName),
			)
			assert.NoError(t, err)

			_, err = container.DB.Exec(fmt.Sprintf("CREATE DATABASE `%s`", underscoreDbName))
			assert.NoError(t, err)

			defer func() {
				_, _ = container.DB.Exec(
					fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", underscoreDbName),
				)
			}()

			underscoreDSN := fmt.Sprintf(
				"%s:%s@tcp(%s:%d)/%s?parseTime=true",
				container.Username,
				container.Password,
				container.Host,
				container.Port,
				underscoreDbName,
			)
			underscoreDB, err := sqlx.Connect("mysql", underscoreDSN)
			assert.NoError(t, err)
			defer underscoreDB.Close()

			_, err = underscoreDB.Exec(`
				CREATE TABLE all_priv_test (
					id INT AUTO_INCREMENT PRIMARY KEY,
					data VARCHAR(255) NOT NULL
				)
			`)
			assert.NoError(t, err)

			_, err = underscoreDB.Exec(`INSERT INTO all_priv_test (data) VALUES ('test1')`)
			assert.NoError(t, err)

			allPrivUsername := fmt.Sprintf("allpriv%s", uuid.New().String()[:8])
			allPrivPassword := "allprivpass123"

			_, err = underscoreDB.Exec(fmt.Sprintf(
				"CREATE USER '%s'@'%%' IDENTIFIED BY '%s'",
				allPrivUsername,
				allPrivPassword,
			))
			assert.NoError(t, err)

			_, err = underscoreDB.Exec(fmt.Sprintf(
				"GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'",
				underscoreDbName,
				allPrivUsername,
			))
			assert.NoError(t, err)

			_, err = underscoreDB.Exec("FLUSH PRIVILEGES")
			assert.NoError(t, err)

			defer dropUserSafe(underscoreDB, allPrivUsername)

			mariadbModel := &MariadbDatabase{
				Version:  tc.version,
				Host:     container.Host,
				Port:     container.Port,
				Username: allPrivUsername,
				Password: allPrivPassword,
				Database: &underscoreDbName,
				IsHttps:  false,
			}

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			err = mariadbModel.TestConnection(logger, nil, uuid.New())
			assert.NoError(t, err)
			assert.NotEmpty(t, mariadbModel.Privileges)
			assert.Contains(t, mariadbModel.Privileges, "SELECT")
			assert.Contains(t, mariadbModel.Privileges, "SHOW VIEW")
		})
	}
}

type MariadbContainer struct {
	Host     string
	Port     int
	Username string
	Password string
	Database string
	Version  tools.MariadbVersion
	DB       *sqlx.DB
}

func Test_GetRawDbSizeMb_Mariadb_ReturnsPositiveSize(t *testing.T) {
	env := config.GetEnv()
	container := connectToMariadbContainer(t, env.TestMariadb1011Port, tools.MariadbVersion1011)
	defer container.DB.Close()

	tableName := fmt.Sprintf("size_test_%s", uuid.New().String()[:8])
	_, err := container.DB.Exec(fmt.Sprintf(
		`CREATE TABLE %s (id INT AUTO_INCREMENT PRIMARY KEY, payload TEXT NOT NULL)`,
		tableName,
	))
	assert.NoError(t, err)
	defer func() {
		_, _ = container.DB.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
	}()

	for i := 0; i < 1000; i++ {
		_, err = container.DB.Exec(
			fmt.Sprintf("INSERT INTO %s (payload) VALUES (?)", tableName),
			strings.Repeat("x", 1024),
		)
		assert.NoError(t, err)
	}

	_, err = container.DB.Exec("ANALYZE TABLE " + tableName)
	assert.NoError(t, err)

	mariadbModel := createMariadbModel(container)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sizeMB, err := mariadbModel.GetRawDbSizeMb(t.Context(), logger, nil, uuid.New())
	assert.NoError(t, err)
	assert.Greater(t, sizeMB, 0.0, "raw db size should be > 0 after inserting data")
}

func connectToMariadbContainer(
	t *testing.T,
	port string,
	version tools.MariadbVersion,
) *MariadbContainer {
	if port == "" {
		t.Skipf("MariaDB port not configured for version %s", version)
	}

	dbName := "testdb"
	host := config.GetEnv().TestLocalhost
	username := "root"
	password := "rootpassword"

	portInt, err := strconv.Atoi(port)
	assert.NoError(t, err)

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		username, password, host, portInt, dbName)

	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Skipf("Failed to connect to MariaDB %s: %v", version, err)
	}

	return &MariadbContainer{
		Host:     host,
		Port:     portInt,
		Username: username,
		Password: password,
		Database: dbName,
		Version:  version,
		DB:       db,
	}
}

func createMariadbModel(container *MariadbContainer) *MariadbDatabase {
	return &MariadbDatabase{
		Version:  container.Version,
		Host:     container.Host,
		Port:     container.Port,
		Username: container.Username,
		Password: container.Password,
		Database: &container.Database,
		IsHttps:  false,
	}
}

func dropUserSafe(db *sqlx.DB, username string) {
	_, _ = db.Exec(fmt.Sprintf("DROP USER '%s'@'%%'", username))
}
