package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
)

const (
	acmeRuntimeTempPrefix      = "s-ui-acme-runtime-"
	acmeRuntimeStateMaxFiles   = 128
	acmeRuntimeStateMaxFileLen = 2 * 1024 * 1024
	acmeRuntimeStateMaxBytes   = 8 * 1024 * 1024
	acmeIPRuntimeProfile       = "ip_acme"
	acmeTempWorkspaceStaleAge  = 30 * time.Minute
)

// acmeRuntimeState is deliberately limited to the acme.sh ca/ tree. The
// account.conf file is not persisted because dnsapi scripts can write DNS
// credentials to it; DNS credentials have their own database account instead.
type acmeRuntimeState struct {
	Files map[string]string `json:"files"`
}

// acmeOperationRuntime owns an operation-scoped config home. The installed
// acme.sh home is kept separately so acme.sh can still load its dnsapi files.
type acmeOperationRuntime struct {
	root       string
	configHome string
	account    *model.AcmeAccount
}

func newAcmeOperationRuntime(account *model.AcmeAccount) (*acmeOperationRuntime, error) {
	if account == nil {
		return nil, common.NewError("acme runtime account is nil")
	}
	root, err := os.MkdirTemp("", acmeRuntimeTempPrefix)
	if err != nil {
		return nil, common.NewError("create temporary acme runtime failed: ", err)
	}
	runtime := &acmeOperationRuntime{
		root:       root,
		configHome: filepath.Join(root, "config"),
		account:    account,
	}
	if err := os.Chmod(root, 0o700); err != nil {
		runtime.cleanup()
		return nil, common.NewError("secure temporary acme runtime failed: ", err)
	}
	if err := os.MkdirAll(runtime.configHome, 0o700); err != nil {
		runtime.cleanup()
		return nil, common.NewError("create temporary acme config home failed: ", err)
	}
	if err := restoreAcmeRuntimeState(runtime.configHome, account.RuntimeState); err != nil {
		runtime.cleanup()
		return nil, err
	}
	return runtime, nil
}

func (r *acmeOperationRuntime) commandArgs(homeDir string) []string {
	args := append([]string{}, acmeHomeArgs(homeDir)...)
	args = append(args, "--config-home", r.configHome)
	return args
}

func (r *acmeOperationRuntime) snapshot() error {
	if r == nil || r.account == nil {
		return common.NewError("acme runtime account is nil")
	}
	state, registered, err := captureAcmeRuntimeState(r.configHome, r.account.Server)
	if err != nil {
		return err
	}
	if !registered {
		if !r.account.Registered && len(r.account.RuntimeState) == 0 {
			return nil
		}
		// acme.sh creates account.key before EAB and newAccount complete. Never
		// persist that partial state as a registered account; clearing it here
		// lets the next operation start with a clean CA runtime.
		r.account.RuntimeState = nil
		r.account.Registered = false
		if err := database.GetDB().Save(r.account).Error; err != nil {
			return err
		}
		return nil
	}
	r.account.RuntimeState = state
	r.account.Registered = true
	if err := database.GetDB().Save(r.account).Error; err != nil {
		return err
	}
	return nil
}

// hasVerifiedAccountState verifies the CA-specific runtime instead of trusting
// only the database flag. account.key is created before an ACME registration
// completes, so it is insufficient evidence of a usable account.
func (r *acmeOperationRuntime) hasVerifiedAccountState(server string) bool {
	if r == nil {
		return false
	}
	return hasVerifiedAcmeRuntimeState(r.configHome, server)
}

func (r *acmeOperationRuntime) cleanup() {
	if r == nil {
		return
	}
	root := filepath.Clean(strings.TrimSpace(r.root))
	if root == "" || root == "." || root == string(filepath.Separator) {
		return
	}
	base := filepath.Base(root)
	if !strings.HasPrefix(base, acmeRuntimeTempPrefix) {
		return
	}
	_ = os.RemoveAll(root)
}

// CleanupStaleAcmeTempWorkspaces removes only old, panel-owned ACME temporary
// directories left behind by a crash, forced termination, or power loss.
// Active operations are protected by the age threshold and are never removed
// during normal startup.
func CleanupStaleAcmeTempWorkspaces() error {
	return cleanupStaleAcmeTempWorkspacesIn(os.TempDir())
}

func cleanupStaleAcmeTempWorkspacesIn(tempRoot string) error {
	root := filepath.Clean(strings.TrimSpace(tempRoot))
	if root == "" || root == "." || root == string(filepath.Separator) {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read ACME temporary directory failed: %w", err)
	}

	cutoff := time.Now().Add(-acmeTempWorkspaceStaleAge)
	var firstErr error
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, acmeRuntimeTempPrefix) && !strings.HasPrefix(name, "sui-acme-") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("inspect ACME temporary workspace %q failed: %w", name, infoErr)
			}
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		path := filepath.Join(root, name)
		if removeErr := os.RemoveAll(path); removeErr != nil && !os.IsNotExist(removeErr) && firstErr == nil {
			firstErr = fmt.Errorf("remove stale ACME temporary workspace %q failed: %w", name, removeErr)
		}
	}
	return firstErr
}

func restoreAcmeRuntimeState(configHome string, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	state := acmeRuntimeState{}
	if err := json.Unmarshal(raw, &state); err != nil {
		return common.NewError("parse stored acme account runtime failed: ", err)
	}
	if len(state.Files) > acmeRuntimeStateMaxFiles {
		return common.NewError("stored acme account runtime contains too many files")
	}
	totalSize := 0
	for relative, encoded := range state.Files {
		relative, err := validateAcmeRuntimeRelativePath(relative)
		if err != nil {
			return err
		}
		content, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil {
			return common.NewError("decode stored acme account runtime failed: ", decodeErr)
		}
		if len(content) > acmeRuntimeStateMaxFileLen {
			return common.NewError("stored acme account runtime file is too large")
		}
		if totalSize+len(content) > acmeRuntimeStateMaxBytes {
			return common.NewError("stored acme account runtime is too large")
		}
		totalSize += len(content)
		target := filepath.Join(configHome, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return common.NewError("restore acme runtime directory failed: ", err)
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return common.NewError("restore acme runtime file failed: ", err)
		}
	}
	return nil
}

func captureAcmeRuntimeState(configHome string, server string) ([]byte, bool, error) {
	caRoot, err := acmeRuntimeCADir(configHome, server)
	if err != nil {
		return nil, false, err
	}
	if !hasVerifiedAcmeRuntimeState(configHome, server) {
		return nil, false, nil
	}
	if _, err := os.Stat(caRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, common.NewError("inspect temporary acme runtime failed: ", err)
	}

	state := acmeRuntimeState{Files: map[string]string{}}
	totalSize := 0
	err = filepath.WalkDir(caRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return common.NewError("temporary acme runtime cannot contain symbolic links")
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if len(state.Files) >= acmeRuntimeStateMaxFiles {
			return common.NewError("temporary acme runtime contains too many files")
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Size() > acmeRuntimeStateMaxFileLen || totalSize+int(info.Size()) > acmeRuntimeStateMaxBytes {
			return common.NewError("temporary acme runtime is too large")
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(configHome, path)
		if relErr != nil {
			return relErr
		}
		relative, relErr = validateAcmeRuntimeRelativePath(filepath.ToSlash(relative))
		if relErr != nil {
			return relErr
		}
		state.Files[relative] = base64.StdEncoding.EncodeToString(content)
		totalSize += len(content)
		return nil
	})
	if err != nil {
		return nil, false, common.NewError("capture temporary acme runtime failed: ", err)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, false, common.NewError("encode acme account runtime failed: ", err)
	}
	return raw, true, nil
}

func acmeRuntimeCADir(configHome string, server string) (string, error) {
	server = normalizeSupportedAcmeDomainServer(server)
	var directory string
	switch server {
	case "letsencrypt":
		directory = acmeLEProductionDirectory
	case "zerossl":
		directory = acmeZeroSSLDirectory
	default:
		return "", common.NewError("unsupported acme runtime CA")
	}

	parsed, err := url.Parse(directory)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return "", common.NewError("parse acme runtime CA failed")
	}
	pathPart := strings.Trim(strings.TrimSpace(parsed.Path), "/")
	if pathPart == "" {
		return "", common.NewError("acme runtime CA path is empty")
	}
	return filepath.Join(strings.TrimSpace(configHome), "ca", parsed.Host, filepath.FromSlash(pathPart)), nil
}

func hasVerifiedAcmeRuntimeState(configHome string, server string) bool {
	caDir, err := acmeRuntimeCADir(configHome, server)
	if err != nil {
		return false
	}
	keyInfo, err := os.Stat(filepath.Join(caDir, "account.key"))
	if err != nil || !keyInfo.Mode().IsRegular() || keyInfo.Size() == 0 {
		return false
	}

	config, err := readAcmeRuntimeCAConfig(caDir)
	if err != nil {
		return false
	}
	accountURL := strings.TrimSpace(config["ACCOUNT_URL"])
	parsedURL, err := url.ParseRequestURI(accountURL)
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || strings.TrimSpace(parsedURL.Host) == "" {
		return false
	}
	if strings.TrimSpace(config["CA_KEY_HASH"]) == "" {
		return false
	}
	if normalizeSupportedAcmeDomainServer(server) == "zerossl" &&
		(strings.TrimSpace(config["CA_EAB_KEY_ID"]) == "" || strings.TrimSpace(config["CA_EAB_HMAC_KEY"]) == "") {
		return false
	}
	return true
}

func readAcmeRuntimeCAConfig(caDir string) (map[string]string, error) {
	raw, err := os.ReadFile(filepath.Join(caDir, "ca.conf"))
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\r", "\n"), "\n")
	for _, line := range lines {
		key := parseAcmeEnvLineKey(line)
		if key == "" {
			continue
		}
		result[key] = parseAcmeEnvLineValue(line)
	}
	return result, nil
}

func clearAcmeRuntimeCAState(configHome string, server string) error {
	caDir, err := acmeRuntimeCADir(configHome, server)
	if err != nil {
		return err
	}
	caRoot := filepath.Join(strings.TrimSpace(configHome), "ca")
	relative, err := filepath.Rel(caRoot, caDir)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return common.NewError("invalid acme runtime CA directory")
	}
	if err := os.RemoveAll(caDir); err != nil && !os.IsNotExist(err) {
		return common.NewError("clear temporary acme runtime failed: ", err)
	}
	return nil
}

func validateAcmeRuntimeRelativePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.HasPrefix(value, "/") || !strings.HasPrefix(value, "ca/") {
		return "", common.NewError("invalid stored acme runtime path")
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", common.NewError("invalid stored acme runtime path")
		}
	}
	return value, nil
}

func (s *AcmeService) ensureIPRuntimeAccount(email string) (*model.AcmeAccount, error) {
	db := database.GetDB()
	account := &model.AcmeAccount{}
	err := db.Where("system = ? AND name = ?", true, acmeIPRuntimeProfile).First(account).Error
	if err != nil && !database.IsNotFound(err) {
		return nil, err
	}
	if database.IsNotFound(err) {
		account = &model.AcmeAccount{
			Name:             acmeIPRuntimeProfile,
			Email:            strings.TrimSpace(email),
			Server:           "letsencrypt",
			KeyLength:        defaultAcmeKeyLength,
			AccountKeyLength: defaultAcmeKeyLength,
			System:           true,
		}
		if err := db.Create(account).Error; err != nil {
			return nil, err
		}
		return account, nil
	}
	if strings.TrimSpace(email) != strings.TrimSpace(account.Email) {
		account.Email = strings.TrimSpace(email)
		if err := db.Save(account).Error; err != nil {
			return nil, err
		}
	}
	return account, nil
}

// ensureOperationRuntimeAccount restores exactly one account into an
// operation-scoped config home and synchronizes its registration/contact state
// before --issue or --renew runs. No account state is ever written to the
// installed acme.sh directory.
func (s *AcmeService) ensureOperationRuntimeAccount(scriptPath string, homeDir string, runtime *acmeOperationRuntime, account *model.AcmeAccount, server string, logSession *acmeLogSession) error {
	return s.ensureOperationRuntimeAccountWithRunner(scriptPath, homeDir, runtime, account, server, logSession, nil)
}

func (s *AcmeService) ensureOperationRuntimeAccountWithRunner(scriptPath string, homeDir string, runtime *acmeOperationRuntime, account *model.AcmeAccount, server string, logSession *acmeLogSession, runner acmeCommandRunner) error {
	if runtime == nil || account == nil {
		return common.NewError("acme runtime account is nil")
	}
	server = strings.TrimSpace(server)
	if server == "" {
		return common.NewError("acme CA platform is empty")
	}
	email, err := validateAcmeAccountEmailForServer(account.Email, account.Server)
	if err != nil {
		return err
	}
	argsPrefix := runtime.commandArgs(homeDir)
	runner = defaultAcmeCommandRunner(runner)
	run := func(args []string) error {
		_, runErr := runner(90*time.Second, scriptPath, append(argsPrefix, args...), nil, logSession)
		return runErr
	}

	hasState := account.Registered && len(account.RuntimeState) > 0 && runtime.hasVerifiedAccountState(server)
	if hasState {
		cachedEmail, readErr := readAcmeRuntimeAccountEmail(runtime.configHome, server)
		if readErr != nil {
			return readErr
		}
		if acmeRuntimeAccountEmailsEqual(cachedEmail, email, server) {
			if logSession != nil {
				logSession.append("ACME 账号运行态已验证，联系人未变化，跳过账号更新")
			}
			return nil
		}
		if email == "" {
			// acme.sh falls back to CA_EMAIL when -m is empty. Remove the
			// temporary cached contact first so a blank Let's Encrypt contact
			// is synchronized as an empty contact list instead of being ignored.
			if err := clearAcmeRuntimeAccountEmail(runtime.configHome, server); err != nil {
				return err
			}
		}
		if logSession != nil {
			logSession.append("ACME 账号联系人已变化，正在同步")
		}
		args := []string{"--update-account", "-m", email, "--server", server}
		if err := run(args); err == nil {
			return nil
		} else if isAcmeUnsupportedEmailFlagError(err) {
			return run([]string{"--update-account", "--accountemail", email, "--server", server})
		} else if !isAcmeAccountNotRegisteredError(err) {
			return err
		}
		// A damaged or manually removed account state is repaired by creating a
		// fresh account inside the same temporary config home below.
	}
	if err := clearAcmeRuntimeCAState(runtime.configHome, server); err != nil {
		return err
	}

	if logSession != nil {
		logSession.append("注册独立 ACME 账号运行态")
	}
	args := []string{"--register-account"}
	if email != "" {
		args = append(args, "-m", email)
	}
	args = append(args, "--server", server, "--accountkeylength", effectiveAcmeAccountKeyLength(account))
	if err := run(args); err == nil {
		return nil
	} else if email != "" && isAcmeUnsupportedEmailFlagError(err) {
		legacy := []string{"--register-account", "--accountemail", email, "--server", server, "--accountkeylength", effectiveAcmeAccountKeyLength(account)}
		return run(legacy)
	} else {
		return err
	}
}

func readAcmeRuntimeAccountEmail(configHome string, server string) (string, error) {
	caDir, err := acmeRuntimeCADir(configHome, server)
	if err != nil {
		return "", err
	}
	config, err := readAcmeRuntimeCAConfig(caDir)
	if err != nil {
		return "", common.NewError("read temporary acme account config failed: ", err)
	}
	return strings.TrimSpace(config["CA_EMAIL"]), nil
}

func acmeRuntimeAccountEmailsEqual(left string, right string, server string) bool {
	leftNormalized, leftErr := validateAcmeAccountEmailForServer(left, server)
	rightNormalized, rightErr := validateAcmeAccountEmailForServer(right, server)
	if leftErr != nil || rightErr != nil {
		return strings.TrimSpace(left) == strings.TrimSpace(right)
	}
	return leftNormalized == rightNormalized
}

// clearAcmeRuntimeAccountEmail removes the cached CA contact from a temporary
// runtime before acme.sh --update-account is invoked with an empty email.
// Runtime snapshots contain only the ca/ tree, so touching it never mutates
// the installed acme.sh directory or any user-owned account configuration.
func clearAcmeRuntimeAccountEmail(configHome string, server string) error {
	caRoot, err := acmeRuntimeCADir(configHome, server)
	if err != nil {
		return err
	}
	if _, err := os.Stat(caRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return common.NewError("inspect temporary acme account config failed: ", err)
	}

	err = filepath.WalkDir(caRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry == nil || entry.IsDir() || filepath.Base(path) != "ca.conf" {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return common.NewError("temporary acme runtime cannot contain symbolic links")
		}
		if !entry.Type().IsRegular() {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		normalized := strings.ReplaceAll(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\r", "\n")
		lines := strings.Split(normalized, "\n")
		kept := make([]string, 0, len(lines))
		changed := false
		for _, line := range lines {
			key := parseAcmeEnvLineKey(line)
			if key == "CA_EMAIL" || key == "SAVED_CA_EMAIL" {
				changed = true
				continue
			}
			kept = append(kept, line)
		}
		if !changed {
			return nil
		}

		output := strings.Join(kept, "\n")
		if !strings.HasSuffix(output, "\n") {
			output += "\n"
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(output), info.Mode().Perm()); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return common.NewError("clear temporary acme account email failed: ", err)
	}
	return nil
}
