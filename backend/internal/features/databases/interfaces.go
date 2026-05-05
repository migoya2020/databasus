package databases

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"databasus-backend/internal/util/encryption"
)

type DatabaseValidator interface {
	Validate() error
}

type DatabaseConnector interface {
	TestConnection(
		logger *slog.Logger,
		encryptor encryption.FieldEncryptor,
		databaseID uuid.UUID,
	) error

	GetRawDbSizeMb(
		ctx context.Context,
		logger *slog.Logger,
		encryptor encryption.FieldEncryptor,
		databaseID uuid.UUID,
	) (float64, error)

	HideSensitiveData()
}

type DatabaseCreationListener interface {
	OnDatabaseCreated(databaseID uuid.UUID)
}

type DatabaseRemoveListener interface {
	OnBeforeDatabaseRemove(databaseID uuid.UUID) error
}

type DatabaseCopyListener interface {
	OnDatabaseCopied(originalDatabaseID, newDatabaseID uuid.UUID)
}
