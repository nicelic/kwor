package migration

import (
	"fmt"
	"log"
	"os"

	"github.com/alireza0/s-ui/config"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func MigrateDb() {
	if err := MigrateDbWithError(); err != nil {
		log.Fatal(err)
	}
}

func MigrateDbWithError() (err error) {
	return migrateDBAtPath(config.GetDBPath())
}

func migrateDBAtPath(path string) (err error) {
	// void running on first install
	_, err = os.Stat(path)
	if err != nil {
		fmt.Println("Database not found")
		return nil
	}

	db, err := gorm.Open(sqlite.Open(path))
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := sqlDB.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if err == nil {
			if commitErr := tx.Commit().Error; commitErr != nil {
				err = commitErr
			}
			return
		}
		_ = tx.Rollback().Error
	}()

	currentVersion := config.GetVersion()
	dbVersion := ""
	tx.Raw("SELECT value FROM settings WHERE key = ?", "version").Find(&dbVersion)
	fmt.Println("Current version:", currentVersion, "\nDatabase version:", dbVersion)

	if currentVersion == dbVersion {
		fmt.Println("Database is up to date, no need to migrate")
		return nil
	}

	fmt.Println("Start migrating database...")

	// Before 1.2
	if dbVersion == "" {
		if err = to1_1(tx); err != nil {
			return fmt.Errorf("migration to 1.1 failed: %w", err)
		}
		if err = to1_2(tx); err != nil {
			return fmt.Errorf("migration to 1.2 failed: %w", err)
		}
		dbVersion = "1.2"
	}

	// Before 1.3
	if len(dbVersion) >= 3 && dbVersion[0:3] == "1.2" {
		if err = to1_3(tx); err != nil {
			return fmt.Errorf("migration to 1.3 failed: %w", err)
		}
	}

	// Set version
	err = tx.Exec("UPDATE settings SET value = ? WHERE key = ?", currentVersion, "version").Error
	if err != nil {
		return fmt.Errorf("update version failed: %w", err)
	}
	fmt.Println("Migration done!")
	return nil
}
