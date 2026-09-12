package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service/ddns"
)

type DNSService struct{}

type DNSOverviewResponse struct {
	Accounts  []model.DNSAccount        `json:"accounts"`
	Providers []ddns.ProviderMeta       `json:"providers"`
	Favorites []model.DNSDomainFavorite `json:"favorites"`
}

func (s *DNSService) GetOverview() (*DNSOverviewResponse, error) {
	db := database.GetDDNSDB()
	if db == nil {
		return nil, errors.New("failed to connect to ddns database")
	}

	var accounts []model.DNSAccount
	if err := db.Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, err
	}

	var favorites []model.DNSDomainFavorite
	if err := db.Order("is_default DESC, id ASC").Find(&favorites).Error; err != nil {
		favorites = []model.DNSDomainFavorite{}
	}

	return &DNSOverviewResponse{
		Accounts:  accounts,
		Providers: ddns.GetProviderCatalog(),
		Favorites: favorites,
	}, nil
}

func (s *DNSService) getGeneralProvider(accountID uint) (ddns.GeneralDNSProvider, map[string]string, error) {
	db := database.GetDDNSDB()
	if db == nil {
		return nil, nil, errors.New("failed to connect to ddns database")
	}

	var account model.DNSAccount
	if err := db.First(&account, accountID).Error; err != nil {
		return nil, nil, errors.New("selected DNS account does not exist")
	}

	var env map[string]string
	if err := json.Unmarshal([]byte(account.EnvJSON), &env); err != nil {
		return nil, nil, fmt.Errorf("failed to parse account credentials: %w", err)
	}

	baseProvider, err := ddns.GetProvider(account.ProviderCode)
	if err != nil {
		return nil, nil, err
	}

	gp, ok := baseProvider.(ddns.GeneralDNSProvider)
	if !ok {
		return nil, nil, fmt.Errorf("provider '%s' does not support general DNS management", baseProvider.Name())
	}

	return gp, env, nil
}

func (s *DNSService) SaveAccount(account *model.DNSAccount) error {
	if account == nil {
		return errors.New("account cannot be nil")
	}
	account.Name = strings.TrimSpace(account.Name)
	if account.Name == "" {
		return errors.New("account name cannot be empty")
	}
	account.ProviderCode = strings.TrimSpace(account.ProviderCode)
	if account.ProviderCode == "" {
		return errors.New("provider code cannot be empty")
	}
	account.Remark = strings.TrimSpace(account.Remark)

	p, err := ddns.GetProvider(account.ProviderCode)
	if err != nil {
		return err
	}
	if _, ok := p.(ddns.GeneralDNSProvider); !ok {
		return fmt.Errorf("provider '%s' does not support general DNS management", p.Name())
	}

	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}

	if account.Id == 0 {
		return db.Create(account).Error
	}
	return db.Model(&model.DNSAccount{}).Where("id = ?", account.Id).Updates(map[string]any{
		"name":          account.Name,
		"provider_code": account.ProviderCode,
		"env_json":      account.EnvJSON,
		"remark":        account.Remark,
	}).Error
}

func (s *DNSService) DeleteAccount(id uint) error {
	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}
	_ = db.Where("account_id = ?", id).Delete(&model.DNSDomainFavorite{}).Error
	return db.Delete(&model.DNSAccount{}, id).Error
}

func (s *DNSService) TestAccountAuth(account *model.DNSAccount) error {
	p, err := ddns.GetProvider(account.ProviderCode)
	if err != nil {
		return err
	}
	var env map[string]string
	if err := json.Unmarshal([]byte(account.EnvJSON), &env); err != nil {
		return errors.New("invalid credentials format")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return p.TestAuth(ctx, env)
}

func (s *DNSService) ListRecords(accountID uint, domain string) ([]ddns.DNSRecord, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, errors.New("domain name cannot be empty")
	}

	gp, env, err := s.getGeneralProvider(accountID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	return gp.ListRecords(ctx, env, domain)
}

func (s *DNSService) CreateRecord(accountID uint, domain string, record ddns.DNSRecord) (string, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return "", errors.New("domain name cannot be empty")
	}
	record.Name = strings.TrimSpace(record.Name)
	record.Type = strings.TrimSpace(strings.ToUpper(record.Type))
	record.Value = strings.TrimSpace(record.Value)
	if record.Type == "" {
		return "", errors.New("record type cannot be empty")
	}
	if record.Value == "" {
		return "", errors.New("record value cannot be empty")
	}

	gp, env, err := s.getGeneralProvider(accountID)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	return gp.CreateRecord(ctx, env, domain, record)
}

func (s *DNSService) UpdateRecord(accountID uint, domain string, record ddns.DNSRecord) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return errors.New("domain name cannot be empty")
	}
	if record.ID == "" {
		return errors.New("record ID cannot be empty")
	}
	record.Name = strings.TrimSpace(record.Name)
	record.Type = strings.TrimSpace(strings.ToUpper(record.Type))
	record.Value = strings.TrimSpace(record.Value)
	if record.Type == "" {
		return errors.New("record type cannot be empty")
	}
	if record.Value == "" {
		return errors.New("record value cannot be empty")
	}

	gp, env, err := s.getGeneralProvider(accountID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	return gp.UpdateRecord(ctx, env, domain, record)
}

func (s *DNSService) DeleteRecord(accountID uint, domain string, recordID string) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return errors.New("domain name cannot be empty")
	}
	recordID = strings.TrimSpace(recordID)
	if recordID == "" {
		return errors.New("record ID cannot be empty")
	}

	gp, env, err := s.getGeneralProvider(accountID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	return gp.DeleteRecordByID(ctx, env, domain, recordID)
}

func (s *DNSService) SaveFavoriteDomain(fav *model.DNSDomainFavorite) error {
	if fav == nil {
		return errors.New("domain cannot be nil")
	}
	fav.Domain = strings.TrimSpace(fav.Domain)
	if fav.Domain == "" {
		return errors.New("domain cannot be empty")
	}

	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}

	if fav.Id == 0 {
		var existing model.DNSDomainFavorite
		if err := db.Where("LOWER(domain) = ? AND account_id = ?", strings.ToLower(fav.Domain), fav.AccountID).First(&existing).Error; err == nil {
			fav.Id = existing.Id
		}
	}

	if fav.IsDefault {
		_ = db.Model(&model.DNSDomainFavorite{}).Where("id != ?", fav.Id).Update("is_default", false).Error
	}

	if fav.Id == 0 {
		return db.Create(fav).Error
	}
	return db.Model(&model.DNSDomainFavorite{}).Where("id = ?", fav.Id).Updates(map[string]any{
		"account_id": fav.AccountID,
		"domain":     fav.Domain,
		"remark":     fav.Remark,
		"is_default": fav.IsDefault,
	}).Error
}

func (s *DNSService) DeleteFavoriteDomain(id uint) error {
	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}
	return db.Delete(&model.DNSDomainFavorite{}, id).Error
}

func (s *DNSService) ListDomains(accountID uint) ([]string, error) {
	gp, env, err := s.getGeneralProvider(accountID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return gp.ListDomains(ctx, env)
}
