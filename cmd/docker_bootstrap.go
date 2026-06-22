package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/service"
	"github.com/alireza0/s-ui/util/common"
)

const (
	dockerBootstrapUsernameEnv   = "KWOR_BOOTSTRAP_USERNAME"
	dockerBootstrapPasswordEnv   = "KWOR_BOOTSTRAP_PASSWORD"
	dockerBootstrapPanelPortEnv  = "KWOR_BOOTSTRAP_PANEL_PORT"
	dockerBootstrapPanelPathEnv  = "KWOR_BOOTSTRAP_PANEL_PATH"
	dockerBootstrapSubPortEnv    = "KWOR_BOOTSTRAP_SUB_PORT"
	dockerBootstrapSubPathEnv    = "KWOR_BOOTSTRAP_SUB_PATH"
	dockerBootstrapPasswordChars = 24
	dockerBootstrapMarkerName    = "docker-bootstrap.done"
)

func handleDockerBootstrap() {
	if err := runDockerBootstrap(); err != nil {
		fmt.Printf("[kwor] docker bootstrap failed: %v\n", err)
		os.Exit(1)
	}
}

func runDockerBootstrap() error {
	bootstrapMarkerPath := dockerBootstrapMarkerPath()
	bootstrapMarked := dockerBootstrapFileExists(bootstrapMarkerPath)

	if err := database.InitDB(config.GetDBPath()); err != nil {
		return err
	}
	if err := service.InitManagedRuntimeFileStore(); err != nil {
		return err
	}

	settingService := &service.SettingService{}
	if _, err := settingService.GetAllSetting(); err != nil {
		return err
	}
	if migrated, splitErr := service.SplitLegacySharedPanelSelfSignedCertificate(settingService); splitErr != nil {
		fmt.Printf("[kwor] bootstrap legacy shared panel tls split failed: %v\n", splitErr)
	} else if migrated {
		fmt.Println("[kwor] bootstrap legacy shared panel tls split completed")
	}
	if migrateErr := service.MigrateLegacyPanelSQLiteCertificatesToInventory(); migrateErr != nil {
		fmt.Printf("[kwor] bootstrap legacy sqlite self-signed certificate migration failed: %v\n", migrateErr)
	}
	if migrateErr := service.MigrateLegacySettingsPathCertificatesToInventory(settingService); migrateErr != nil {
		fmt.Printf("[kwor] bootstrap legacy settings-path certificate migration failed: %v\n", migrateErr)
	}
	if repairErr := (&service.CertificateInventoryService{}).RepairDisplayIDs(); repairErr != nil {
		fmt.Printf("[kwor] bootstrap certificate display id repair failed: %v\n", repairErr)
	}
	if syncErr := service.SyncPanelTLSAssignments(settingService); syncErr != nil {
		fmt.Printf("[kwor] bootstrap panel tls assignment sync failed: %v\n", syncErr)
	}

	adminExists, err := dockerBootstrapHasAdmin()
	if err != nil {
		return err
	}

	freshBootstrap := !bootstrapMarked && !adminExists

	if freshBootstrap {
		dockerBootstrapApplySettings(settingService)
	}

	if !adminExists {
		username := strings.TrimSpace(os.Getenv(dockerBootstrapUsernameEnv))
		if username == "" {
			username = "admin"
		}

		password := strings.TrimSpace(os.Getenv(dockerBootstrapPasswordEnv))
		passwordGenerated := false
		if password == "" {
			password = common.Random(dockerBootstrapPasswordChars)
			passwordGenerated = true
		}

		if err := (&service.UserService{}).UpdateFirstUser(username, password); err != nil {
			return err
		}

		fmt.Printf("[kwor] bootstrap admin username: %s\n", username)
		if passwordGenerated {
			fmt.Printf("[kwor] bootstrap admin password: %s\n", password)
		} else {
			fmt.Printf("[kwor] bootstrap admin password: using value from %s\n", dockerBootstrapPasswordEnv)
		}
	} else {
		fmt.Println("[kwor] bootstrap admin already exists, skip credential initialization")
	}

	dockerBootstrapEnsurePanelCertificates(settingService, service.PanelSelfSignedTargetPanel)
	dockerBootstrapEnsurePanelCertificates(settingService, service.PanelSelfSignedTargetSub)

	if !bootstrapMarked {
		if err := dockerBootstrapWriteMarker(bootstrapMarkerPath); err != nil {
			fmt.Printf("[kwor] bootstrap marker write failed: %v\n", err)
		}
	}

	if !adminExists {
		printFirstRunPanelURLs()
	}

	return nil
}

func dockerBootstrapHasAdmin() (bool, error) {
	user, err := (&service.UserService{}).GetFirstUser()
	if err != nil {
		if database.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if user == nil {
		return false, nil
	}
	return strings.TrimSpace(user.Username) != "" && strings.TrimSpace(user.Password) != "", nil
}

func dockerBootstrapApplySettings(settingService *service.SettingService) {
	if settingService == nil {
		return
	}

	if raw := strings.TrimSpace(os.Getenv(dockerBootstrapPanelPortEnv)); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 && port <= 65535 {
			if err := settingService.SetPort(port); err != nil {
				fmt.Printf("[kwor] bootstrap panel port apply failed: %v\n", err)
			}
		} else {
			fmt.Printf("[kwor] bootstrap panel port ignored, invalid value: %q\n", raw)
		}
	}

	if raw := strings.TrimSpace(os.Getenv(dockerBootstrapPanelPathEnv)); raw != "" {
		if err := settingService.SetWebPath(raw); err != nil {
			fmt.Printf("[kwor] bootstrap panel path apply failed: %v\n", err)
		}
	}

	if raw := strings.TrimSpace(os.Getenv(dockerBootstrapSubPortEnv)); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 && port <= 65535 {
			if err := settingService.SetSubPort(port); err != nil {
				fmt.Printf("[kwor] bootstrap subscription port apply failed: %v\n", err)
			}
		} else {
			fmt.Printf("[kwor] bootstrap subscription port ignored, invalid value: %q\n", raw)
		}
	}

	if raw := strings.TrimSpace(os.Getenv(dockerBootstrapSubPathEnv)); raw != "" {
		if err := settingService.SetSubPath(raw); err != nil {
			fmt.Printf("[kwor] bootstrap subscription path apply failed: %v\n", err)
		}
	}
}

func dockerBootstrapEnsurePanelCertificates(settingService *service.SettingService, target service.PanelSelfSignedTarget) {
	material, err := service.ResolvePanelTLSMaterial(settingService, target)
	if err == nil && material != nil {
		return
	}
	if err != nil {
		fmt.Printf("[kwor] bootstrap %s tls status read failed, will try to repair: %v\n", target, err)
	}

	if _, err := service.GenerateAndAssignPanelBootstrapCertificate(target, time.Now()); err != nil {
		fmt.Printf("[kwor] bootstrap %s tls certificate failed, continue in HTTP mode: %v\n", target, err)
		return
	}
	fmt.Printf("[kwor] bootstrap %s tls certificate configured\n", target)
}

func dockerBootstrapShouldRepairPanelCertificate(settingService *service.SettingService, target service.PanelSelfSignedTarget) bool {
	material, err := service.ResolvePanelTLSMaterial(settingService, target)
	return err != nil || material == nil
}

func dockerBootstrapDBFilePath(dbPath string) string {
	dbPath = strings.TrimSpace(dbPath)
	if idx := strings.Index(dbPath, "?"); idx >= 0 {
		dbPath = dbPath[:idx]
	}
	return dbPath
}

func dockerBootstrapFileExists(filePath string) bool {
	if strings.TrimSpace(filePath) == "" {
		return false
	}
	_, err := os.Stat(filePath)
	return err == nil
}

func dockerBootstrapMarkerPath() string {
	return filepath.Join(config.GetDataDir(), "runtime", dockerBootstrapMarkerName)
}

func dockerBootstrapWriteMarker(filePath string) error {
	if strings.TrimSpace(filePath) == "" {
		return fmt.Errorf("bootstrap marker path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filePath, []byte(time.Now().Format(time.RFC3339)), 0o644)
}
