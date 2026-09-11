package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const principalKey = "principal"

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

type Principal struct {
	DisplayName    string `json:"displayName"`
	MSPID          string `json:"mspId"`
	OrganizationID string `json:"organizationId"`
	Role           string `json:"role"`
	Username       string `json:"username"`
}

type account struct {
	Principal
	PasswordHash string `json:"passwordHash"`
	Salt         string `json:"salt"`
}

type session struct {
	principal Principal
	expiresAt time.Time
}

// Service keeps passwords only in a local, hashed account file. The business
// identity itself is registered separately in the Fabric ledger.
type Service struct {
	accounts    map[string]account
	accountFile string
	mu          sync.RWMutex
	sessions    map[string]session
}

func NewService() (*Service, error) {
	service := &Service{
		accounts:    make(map[string]account),
		accountFile: envOr("AUTH_DATA_FILE", "data/accounts.json"),
		sessions:    make(map[string]session),
	}
	service.addDemoAccount("issuer-org1", "Org1 开票员", "Org1MSP", "ISSUER", envOr("DEMO_ISSUER_PASSWORD", "issuer123"))
	service.addDemoAccount("holder-org2", "Org2 跨组织流转员", "Org2MSP", "HOLDER", envOr("DEMO_HOLDER_PASSWORD", "holder123"))
	service.addDemoAccount("auditor", "审计查看员", "Org1MSP", "AUDITOR", envOr("DEMO_AUDITOR_PASSWORD", "auditor123"))
	service.addDemoAccount("project-member", "项目组成员", "Org1MSP", "PROJECT_MEMBER", envOr("DEMO_PROJECT_MEMBER_PASSWORD", "member123"))
	service.addDemoAccount("project-reviewer", "项目管理审核员", "Org1MSP", "PROJECT_REVIEWER", envOr("DEMO_PROJECT_REVIEWER_PASSWORD", "review123"))
	service.addDemoAccount("finance-admin", "财务管理员", "Org2MSP", "FINANCE_ADMIN", envOr("DEMO_FINANCE_ADMIN_PASSWORD", "finance123"))
	service.addDemoAccount("org-admin", "组织管理员", "Org1MSP", "ORG_ADMIN", envOr("DEMO_ORG_ADMIN_PASSWORD", "orgadmin123"))
	service.addDemoAccount("org-admin-org2", "Org2 组织管理员", "Org2MSP", "ORG_ADMIN", envOr("DEMO_ORG_ADMIN_ORG2_PASSWORD", "orgadmin234"))
	if err := service.loadAccounts(); err != nil {
		return nil, fmt.Errorf("检查本地账户库失败（为避免覆盖或丢失已有账号，服务未启动）: %w", err)
	}
	return service, nil
}

func (s *Service) Login(c *gin.Context) {
	var request struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入账户和密码"})
		return
	}
	s.mu.RLock()
	current, ok := s.accounts[request.Username]
	s.mu.RUnlock()
	if !ok || !passwordMatches(request.Password, current) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账户或密码错误"})
		return
	}
	s.startSession(c, current.Principal)
}

// RegisterAccount persists the local login credential after the caller has
// successfully registered the corresponding business user on Fabric.
func (s *Service) RegisterAccount(principal Principal, password string) error {
	if err := s.ValidateRegistration(principal, password); err != nil {
		return err
	}
	if !validMSPID(principal.MSPID) {
		return fmt.Errorf("注册信息中的 Fabric 组织不合法")
	}
	created, err := newAccount(principal, password)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.accounts[principal.Username]; exists {
		return fmt.Errorf("登录账号 %s 已存在", principal.Username)
	}
	s.accounts[principal.Username] = created
	if err := s.saveRegisteredAccountsLocked(); err != nil {
		delete(s.accounts, principal.Username)
		return fmt.Errorf("保存本地登录账户失败: %w", err)
	}
	return nil
}

// ValidateRegistration is called before the irreversible Fabric transaction,
// so a malformed local login cannot leave a chain-only account behind.
func (s *Service) ValidateRegistration(principal Principal, password string) error {
	if !usernamePattern.MatchString(principal.Username) {
		return fmt.Errorf("登录账号须为 3-32 位字母、数字、下划线或连字符")
	}
	if len(password) < 6 {
		return fmt.Errorf("密码至少需要 6 位")
	}
	if principal.DisplayName == "" || principal.OrganizationID == "" || !validRole(principal.Role) {
		return fmt.Errorf("注册信息不完整或不合法")
	}
	s.mu.RLock()
	_, exists := s.accounts[principal.Username]
	s.mu.RUnlock()
	if exists {
		return fmt.Errorf("登录账号 %s 已存在", principal.Username)
	}
	return nil
}

func (s *Service) StartRegisteredSession(c *gin.Context, principal Principal) {
	s.startSession(c, principal)
}

func (s *Service) Logout(c *gin.Context) {
	if token, err := c.Cookie("invoice_session"); err == nil {
		s.mu.Lock()
		delete(s.sessions, token)
		s.mu.Unlock()
	}
	c.SetCookie("invoice_session", "", -1, "/", "", false, true)
	c.Status(http.StatusNoContent)
}

func (s *Service) Me(c *gin.Context) {
	principal, ok := s.principalFromCookie(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	c.JSON(http.StatusOK, principal)
}

func (s *Service) Require() gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := s.principalFromCookie(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
			return
		}
		c.Set(principalKey, principal)
		c.Next()
	}
}

func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := PrincipalFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
			return
		}
		for _, role := range roles {
			if principal.Role == role {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "当前角色没有此操作权限"})
	}
}

func PrincipalFromContext(c *gin.Context) (Principal, bool) {
	principal, ok := c.Get(principalKey)
	if !ok {
		return Principal{}, false
	}
	value, ok := principal.(Principal)
	return value, ok
}

func (s *Service) addDemoAccount(username, displayName, mspID, role, password string) {
	created, err := newAccount(Principal{Username: username, DisplayName: displayName, MSPID: mspID, Role: role}, password)
	if err == nil {
		s.accounts[username] = created
	}
}

func (s *Service) startSession(c *gin.Context, principal Principal) {
	token, err := newToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建登录会话失败"})
		return
	}
	s.mu.Lock()
	s.sessions[token] = session{principal: principal, expiresAt: time.Now().Add(8 * time.Hour)}
	s.mu.Unlock()
	c.SetCookie("invoice_session", token, 8*60*60, "/", "", false, true)
	c.JSON(http.StatusOK, principal)
}

func (s *Service) principalFromCookie(c *gin.Context) (Principal, bool) {
	token, err := c.Cookie("invoice_session")
	if err != nil {
		return Principal{}, false
	}
	s.mu.RLock()
	current, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok || time.Now().After(current.expiresAt) {
		return Principal{}, false
	}
	return current.principal, true
}

func (s *Service) loadAccounts() error {
	data, err := os.ReadFile(s.accountFile)
	if os.IsNotExist(err) {
		data, err = os.ReadFile(s.accountFile + ".bak")
		if os.IsNotExist(err) {
			return nil
		}
	}
	if err != nil {
		return err
	}
	stored, err := decodeAccounts(data)
	if err != nil {
		backupData, backupErr := os.ReadFile(s.accountFile + ".bak")
		if backupErr != nil {
			return fmt.Errorf("主文件损坏且备份不可用: %w", err)
		}
		stored, backupErr = decodeAccounts(backupData)
		if backupErr != nil {
			return fmt.Errorf("主文件与备份均无法解析: 主文件=%v，备份=%v", err, backupErr)
		}
		fmt.Printf("warning: 本地账户主文件无法解析，已从备份恢复登录账户；请检查 %s\n", s.accountFile)
	}
	for _, current := range stored {
		if usernamePattern.MatchString(current.Username) && current.PasswordHash != "" && current.Salt != "" {
			s.accounts[current.Username] = current
		}
	}
	return nil
}

func decodeAccounts(data []byte) ([]account, error) {
	var stored []account
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, fmt.Errorf("账户文件必须是 JSON 数组，不能为 null")
	}
	seen := make(map[string]bool)
	for _, current := range stored {
		if !usernamePattern.MatchString(current.Username) || current.PasswordHash == "" || current.Salt == "" || seen[current.Username] {
			return nil, fmt.Errorf("账户文件包含无效或重复账户")
		}
		seen[current.Username] = true
	}
	return stored, nil
}

func (s *Service) saveRegisteredAccountsLocked() error {
	stored := make([]account, 0, len(s.accounts))
	for username, current := range s.accounts {
		if username != "issuer-org1" && username != "holder-org2" && username != "auditor" && username != "project-member" && username != "project-reviewer" && username != "finance-admin" && username != "org-admin" && username != "org-admin-org2" {
			stored = append(stored, current)
		}
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.accountFile), 0700); err != nil {
		return err
	}
	// Preserve the last complete generation before atomically replacing it.
	// A crash can therefore recover from .bak instead of silently losing every
	// locally registered password.
	if previous, err := os.ReadFile(s.accountFile); err == nil {
		// Never replace a usable backup with the damaged primary we recovered from.
		if _, validErr := decodeAccounts(previous); validErr == nil {
			backupTmp := s.accountFile + ".bak.tmp"
			if err := os.WriteFile(backupTmp, previous, 0600); err != nil {
				return err
			}
			if err := os.Rename(backupTmp, s.accountFile+".bak"); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	temporary := s.accountFile + ".tmp"
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, s.accountFile)
}

func newAccount(principal Principal, password string) (account, error) {
	salt, err := newToken()
	if err != nil {
		return account{}, err
	}
	return account{Principal: principal, Salt: salt, PasswordHash: passwordHash(salt, password)}, nil
}

func passwordMatches(password string, current account) bool {
	return passwordHash(current.Salt, password) == current.PasswordHash
}

func passwordHash(salt, password string) string {
	digest := sha256.Sum256([]byte(salt + ":" + password))
	return hex.EncodeToString(digest[:])
}

func newToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func validMSPID(mspID string) bool { return mspID == "Org1MSP" || mspID == "Org2MSP" }
func validRole(role string) bool {
	return role == "ISSUER" || role == "HOLDER" || role == "AUDITOR" || role == "PROJECT_MEMBER" || role == "PROJECT_REVIEWER" || role == "FINANCE_ADMIN" || role == "ORG_ADMIN"
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
