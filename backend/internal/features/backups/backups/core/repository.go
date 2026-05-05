package backups_core

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"databasus-backend/internal/storage"
)

type BackupRepository struct{}

func (r *BackupRepository) Save(backup *Backup) error {
	if backup.DatabaseID == uuid.Nil || backup.StorageID == uuid.Nil {
		return errors.New("database ID and storage ID are required")
	}

	db := storage.GetDb()

	isNew := backup.ID == uuid.Nil
	if isNew {
		backup.ID = uuid.New()
		return db.Create(backup).
			Error
	}

	return db.Save(backup).
		Error
}

func (r *BackupRepository) ExistsCompletedSince(
	databaseID uuid.UUID,
	since time.Time,
) (bool, error) {
	var count int64

	if err := storage.
		GetDb().
		Model(&Backup{}).
		Where("database_id = ? AND status = ? AND created_at >= ?",
			databaseID, BackupStatusCompleted, since).
		Limit(1).
		Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

func (r *BackupRepository) FindByDatabaseID(databaseID uuid.UUID) ([]*Backup, error) {
	var backups []*Backup

	if err := storage.
		GetDb().
		Where("database_id = ?", databaseID).
		Order("created_at DESC").
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) FindByDatabaseIDWithLimit(
	databaseID uuid.UUID,
	limit int,
) ([]*Backup, error) {
	if limit <= 0 {
		return nil, errors.New("limit must be greater than 0")
	}

	var backups []*Backup

	if err := storage.
		GetDb().
		Where("database_id = ?", databaseID).
		Order("created_at DESC").
		Limit(limit).
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) FindByStorageID(storageID uuid.UUID) ([]*Backup, error) {
	var backups []*Backup

	if err := storage.
		GetDb().
		Where("storage_id = ?", storageID).
		Order("created_at DESC").
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) FindLastByDatabaseID(databaseID uuid.UUID) (*Backup, error) {
	var backup Backup

	if err := storage.
		GetDb().
		Where("database_id = ?", databaseID).
		Order("created_at DESC").
		First(&backup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &backup, nil
}

func (r *BackupRepository) FindByID(id uuid.UUID) (*Backup, error) {
	var backup Backup

	if err := storage.
		GetDb().
		Where("id = ?", id).
		First(&backup).Error; err != nil {
		return nil, err
	}

	return &backup, nil
}

func (r *BackupRepository) FindByStatus(status BackupStatus) ([]*Backup, error) {
	var backups []*Backup

	if err := storage.
		GetDb().
		Where("status = ?", status).
		Order("created_at DESC").
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) FindByStorageIdAndStatus(
	storageID uuid.UUID,
	status BackupStatus,
) ([]*Backup, error) {
	var backups []*Backup

	if err := storage.
		GetDb().
		Where("storage_id = ? AND status = ?", storageID, status).
		Order("created_at DESC").
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) FindByDatabaseIdAndStatus(
	databaseID uuid.UUID,
	status BackupStatus,
) ([]*Backup, error) {
	var backups []*Backup

	if err := storage.
		GetDb().
		Where("database_id = ? AND status = ?", databaseID, status).
		Order("created_at DESC").
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) DeleteByID(id uuid.UUID) error {
	return storage.GetDb().Delete(&Backup{}, "id = ?", id).Error
}

func (r *BackupRepository) FindBackupsBeforeDate(
	databaseID uuid.UUID,
	date time.Time,
) ([]*Backup, error) {
	var backups []*Backup

	if err := storage.
		GetDb().
		Where("database_id = ? AND created_at < ?", databaseID, date).
		Order("created_at DESC").
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) FindByDatabaseIDWithPagination(
	databaseID uuid.UUID,
	limit, offset int,
) ([]*Backup, error) {
	var backups []*Backup

	if err := storage.
		GetDb().
		Where("database_id = ?", databaseID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) CountByDatabaseID(databaseID uuid.UUID) (int64, error) {
	var count int64

	if err := storage.
		GetDb().
		Model(&Backup{}).
		Where("database_id = ?", databaseID).
		Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

func (r *BackupRepository) GetTotalSizeByDatabase(databaseID uuid.UUID) (float64, error) {
	var totalSize float64

	if err := storage.
		GetDb().
		Model(&Backup{}).
		Select("COALESCE(SUM(backup_size_mb), 0)").
		Where("database_id = ? AND status != ?", databaseID, BackupStatusInProgress).
		Scan(&totalSize).Error; err != nil {
		return 0, err
	}

	return totalSize, nil
}

func (r *BackupRepository) FindOldestByDatabaseExcludingInProgress(
	databaseID uuid.UUID,
	limit int,
) ([]*Backup, error) {
	var backups []*Backup

	if err := storage.
		GetDb().
		Where("database_id = ? AND status != ?", databaseID, BackupStatusInProgress).
		Order("created_at ASC").
		Limit(limit).
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) FindCompletedFullWalBackupByID(
	databaseID uuid.UUID,
	backupID uuid.UUID,
) (*Backup, error) {
	var backup Backup

	err := storage.
		GetDb().
		Where(
			"database_id = ? AND id = ? AND pg_wal_backup_type = ? AND status = ?",
			databaseID,
			backupID,
			PgWalBackupTypeFullBackup,
			BackupStatusCompleted,
		).
		First(&backup).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &backup, nil
}

func (r *BackupRepository) FindCompletedWalSegmentsAfter(
	databaseID uuid.UUID,
	afterSegmentName string,
) ([]*Backup, error) {
	var backups []*Backup

	err := storage.
		GetDb().
		Where(
			"database_id = ? AND pg_wal_backup_type = ? AND pg_wal_segment_name >= ? AND status = ?",
			databaseID,
			PgWalBackupTypeWalSegment,
			afterSegmentName,
			BackupStatusCompleted,
		).
		Order("pg_wal_segment_name ASC").
		Find(&backups).Error
	if err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) FindLatestCompleted(
	databaseID uuid.UUID,
) (*Backup, error) {
	var backup Backup

	err := storage.
		GetDb().
		Where("database_id = ? AND status = ?",
			databaseID, BackupStatusCompleted).
		Order("created_at DESC").
		First(&backup).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &backup, nil
}

func (r *BackupRepository) FindLastCompletedFullWalBackupByDatabaseID(
	databaseID uuid.UUID,
) (*Backup, error) {
	var backup Backup

	err := storage.
		GetDb().
		Where(
			"database_id = ? AND pg_wal_backup_type = ? AND status = ?",
			databaseID,
			PgWalBackupTypeFullBackup,
			BackupStatusCompleted,
		).
		Order("created_at DESC").
		First(&backup).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &backup, nil
}

func (r *BackupRepository) FindWalSegmentByName(
	databaseID uuid.UUID,
	segmentName string,
) (*Backup, error) {
	var backup Backup

	err := storage.
		GetDb().
		Where(
			"database_id = ? AND pg_wal_backup_type = ? AND pg_wal_segment_name = ?",
			databaseID,
			PgWalBackupTypeWalSegment,
			segmentName,
		).
		First(&backup).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &backup, nil
}

func (r *BackupRepository) FindLatestCompletedFullWalBackupBefore(
	databaseID uuid.UUID,
	before time.Time,
) (*Backup, error) {
	var backup Backup

	err := storage.
		GetDb().
		Where(
			"database_id = ? AND pg_wal_backup_type = ? AND status = ? AND created_at <= ?",
			databaseID,
			PgWalBackupTypeFullBackup,
			BackupStatusCompleted,
			before,
		).
		Order("created_at DESC").
		First(&backup).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &backup, nil
}

func (r *BackupRepository) FindStaleUploadedBasebackups(olderThan time.Time) ([]*Backup, error) {
	var backups []*Backup

	err := storage.
		GetDb().
		Where(
			"status = ? AND upload_completed_at IS NOT NULL AND upload_completed_at < ?",
			BackupStatusInProgress,
			olderThan,
		).
		Find(&backups).Error
	if err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) FindLastWalSegmentAfter(
	databaseID uuid.UUID,
	afterSegmentName string,
) (*Backup, error) {
	var backup Backup

	err := storage.
		GetDb().
		Where(
			"database_id = ? AND pg_wal_backup_type = ? AND pg_wal_segment_name > ? AND status = ?",
			databaseID,
			PgWalBackupTypeWalSegment,
			afterSegmentName,
			BackupStatusCompleted,
		).
		Order("pg_wal_segment_name DESC").
		First(&backup).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &backup, nil
}

func (r *BackupRepository) FindByDatabaseIDWithFiltersAndPagination(
	databaseID uuid.UUID,
	filters *BackupFilters,
	limit, offset int,
) ([]*Backup, error) {
	var backups []*Backup

	query := storage.
		GetDb().
		Where("database_id = ?", databaseID)

	if filters != nil {
		query = filters.applyToQuery(query)
	}

	if err := query.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *BackupRepository) CountByDatabaseIDWithFilters(
	databaseID uuid.UUID,
	filters *BackupFilters,
) (int64, error) {
	var count int64

	query := storage.
		GetDb().
		Model(&Backup{}).
		Where("database_id = ?", databaseID)

	if filters != nil {
		query = filters.applyToQuery(query)
	}

	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

func (f *BackupFilters) applyToQuery(query *gorm.DB) *gorm.DB {
	if len(f.Statuses) > 0 {
		query = query.Where("status IN ?", f.Statuses)
	}

	if f.BeforeDate != nil {
		query = query.Where("created_at < ?", *f.BeforeDate)
	}

	if f.PgWalBackupType != nil {
		query = query.Where("pg_wal_backup_type = ?", *f.PgWalBackupType)
	}

	return query
}
