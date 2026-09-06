package api

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"invoice-ledger-api/auth"
	"invoice-ledger-api/fabric"
	invoiceocr "invoice-ledger-api/ocr"

	"github.com/gin-gonic/gin"
	"github.com/hyperledger/fabric-gateway/pkg/client"
	gatewaypb "github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"google.golang.org/grpc/status"
)

type Invoice struct {
	AmountCents          int64  `json:"amountCents"`
	Buyer                string `json:"buyer"`
	BuyerMSPID           string `json:"buyerMspId"`
	CreatedAt            string `json:"createdAt"`
	Currency             string `json:"currency"`
	CurrentHolder        string `json:"currentHolder"`
	CorrectionOf         string `json:"correctionOf"`
	DataHash             string `json:"dataHash"`
	HashVersion          string `json:"hashVersion"`
	HolderMSPID          string `json:"holderMspId"`
	ID                   string `json:"id"`
	InvoiceNo            string `json:"invoiceNo"`
	IssueDate            string `json:"issueDate"`
	Issuer               string `json:"issuer"`
	IssuerOrganizationID string `json:"issuerOrganizationId"`
	IssuerMSPID          string `json:"issuerMspId"`
	HolderOrganizationID string `json:"holderOrganizationId"`
	ProjectID            string `json:"projectId"`
	Status               string `json:"status"`
	TaxCents             int64  `json:"taxCents"`
	TotalCents           int64  `json:"totalCents"`
	UpdatedAt            string `json:"updatedAt"`
	VoidReason           string `json:"voidReason"`
}

type InvoiceFlow struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	InvoiceID string `json:"invoiceId"`
	Operator  string `json:"operator"`
	Timestamp string `json:"timestamp"`
	To        string `json:"to"`
	TxID      string `json:"txId"`
	Type      string `json:"type"`
}

type InvoiceVoidRequest struct {
	Applicant     string `json:"applicant"`
	CreatedAt     string `json:"createdAt"`
	InvoiceID     string `json:"invoiceId"`
	Reason        string `json:"reason"`
	ReviewOpinion string `json:"reviewOpinion"`
	Reviewer      string `json:"reviewer"`
	Status        string `json:"status"`
	UpdatedAt     string `json:"updatedAt"`
}

type HistoryRecord struct {
	IsDelete  bool     `json:"isDelete"`
	Timestamp string   `json:"timestamp"`
	TxID      string   `json:"txId"`
	Value     *Invoice `json:"value,omitempty"`
}

type BusinessUser struct {
	CreatedAt      string `json:"createdAt"`
	DisplayName    string `json:"displayName"`
	MSPID          string `json:"mspId"`
	OrganizationID string `json:"organizationId"`
	Role           string `json:"role"`
	Status         string `json:"status"`
	Username       string `json:"username"`
}

type BusinessOrganization struct {
	CreatedAt   string `json:"createdAt"`
	CreatedBy   string `json:"createdBy"`
	Description string `json:"description"`
	ID          string `json:"id"`
	MSPID       string `json:"mspId"`
	Name        string `json:"name"`
	ParentID    string `json:"parentId"`
	Status      string `json:"status"`
	Type        string `json:"type"`
}

type Project struct {
	Applicant            string `json:"applicant"`
	ApplicantMSPID       string `json:"applicantMspId"`
	OrganizationID       string `json:"organizationId"`
	AvailableCents       int64  `json:"availableCents"`
	BudgetCents          int64  `json:"budgetCents"`
	ClosureMaterialsHash string `json:"closureMaterialsHash"`
	ClosureMaterials     string `json:"closureMaterials"`
	Content              string `json:"content"`
	CreatedAt            string `json:"createdAt"`
	ExpectedEndDate      string `json:"expectedEndDate"`
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	PaidCents            int64  `json:"paidCents"`
	ReservedCents        int64  `json:"reservedCents"`
	RecoveredCents       int64  `json:"recoveredCents"`
	ReviewOpinion        string `json:"reviewOpinion"`
	Reviewer             string `json:"reviewer"`
	Status               string `json:"status"`
	UpdatedAt            string `json:"updatedAt"`
}

type ProjectMember struct {
	ProjectID string `json:"projectId"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	AddedAt   string `json:"addedAt"`
	AddedBy   string `json:"addedBy"`
}
type InvoiceTransfer struct {
	InvoiceID string `json:"invoiceId"`
	From      string `json:"from"`
	To        string `json:"to"`
	ToMSPID   string `json:"toMspId"`
	Note      string `json:"note"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Response  string `json:"response"`
}

type ProjectEvent struct {
	Actor       string `json:"actor"`
	ID          string `json:"id"`
	Note        string `json:"note"`
	ProjectID   string `json:"projectId"`
	ReferenceID string `json:"referenceId"`
	Timestamp   string `json:"timestamp"`
	TxID        string `json:"txId"`
	Type        string `json:"type"`
}

type Reimbursement struct {
	AmountCents   int64  `json:"amountCents"`
	Applicant     string `json:"applicant"`
	CreatedAt     string `json:"createdAt"`
	EvidenceHash  string `json:"evidenceHash"`
	Evidence      string `json:"evidence"`
	ID            string `json:"id"`
	InvoiceID     string `json:"invoiceId"`
	ProjectID     string `json:"projectId"`
	ReviewOpinion string `json:"reviewOpinion"`
	Reviewer      string `json:"reviewer"`
	Status        string `json:"status"`
	UpdatedAt     string `json:"updatedAt"`
}

type createInvoiceRequest struct {
	AmountCents int64  `json:"amountCents" binding:"gte=0,lte=10000000000"`
	Buyer       string `json:"buyer" binding:"required"`
	BuyerMSPID  string `json:"buyerMspId" binding:"required,oneof=Org1MSP Org2MSP"`
	Currency    string `json:"currency" binding:"omitempty,oneof=CNY"`
	ID          string `json:"id" binding:"max=48"`
	InvoiceNo   string `json:"invoiceNo" binding:"required"`
	IssueDate   string `json:"issueDate" binding:"required"`
	Issuer      string `json:"issuer" binding:"required"`
	ProjectID   string `json:"projectId"`
	TaxCents    int64  `json:"taxCents" binding:"gte=0,lte=10000000000"`
}

type transferRequest struct {
	To      string `json:"to" binding:"required"`
	ToMSPID string `json:"toMspId" binding:"required,oneof=Org1MSP Org2MSP"`
	Note    string `json:"note" binding:"max=500"`
}

type transferResponseRequest struct {
	Decision string `json:"decision" binding:"required,oneof=ACCEPT REJECT"`
	Response string `json:"response" binding:"max=500"`
}
type projectMemberRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
}
type linkProjectRequest struct {
	ProjectID string `json:"projectId" binding:"required,max=48"`
}

type voidRequest struct {
	Reason string `json:"reason" binding:"required,min=2,max=200"`
}

type voidReviewRequest struct {
	Decision string `json:"decision" binding:"required,oneof=APPROVE REJECT"`
	Opinion  string `json:"opinion" binding:"required,min=2,max=1000"`
}

type verifyRequest struct {
	AmountCents int64  `json:"amountCents" binding:"gte=0,lte=10000000000"`
	Buyer       string `json:"buyer" binding:"required"`
	BuyerMSPID  string `json:"buyerMspId" binding:"required,oneof=Org1MSP Org2MSP"`
	Currency    string `json:"currency" binding:"omitempty,oneof=CNY"`
	InvoiceNo   string `json:"invoiceNo" binding:"required"`
	IssueDate   string `json:"issueDate" binding:"required"`
	Issuer      string `json:"issuer" binding:"required"`
	TaxCents    int64  `json:"taxCents" binding:"gte=0,lte=10000000000"`
}

type registerRequest struct {
	DisplayName    string `json:"displayName" binding:"required,min=2,max=48"`
	OrganizationID string `json:"organizationId" binding:"required,max=48"`
	Password       string `json:"password" binding:"required,min=6,max=72"`
	Role           string `json:"role" binding:"omitempty,oneof=PROJECT_MEMBER"`
	Username       string `json:"username" binding:"required,min=3,max=32"`
}

type organizationRequest struct {
	Description string `json:"description" binding:"max=1000"`
	MSPID       string `json:"mspId" binding:"required,oneof=Org1MSP Org2MSP"`
	Name        string `json:"name" binding:"required,min=2,max=100"`
	ParentID    string `json:"parentId" binding:"max=48"`
	Type        string `json:"type" binding:"required,oneof=PRIMARY PROJECT_TEAM EXTERNAL"`
}

type projectRequest struct {
	BudgetCents     int64  `json:"budgetCents" binding:"gt=0,lte=10000000000"`
	Content         string `json:"content" binding:"required,min=2,max=2000"`
	ExpectedEndDate string `json:"expectedEndDate" binding:"required"`
	ID              string `json:"id" binding:"max=48"`
	Name            string `json:"name" binding:"required,min=2,max=100"`
}

type reviewRequest struct {
	Decision string `json:"decision" binding:"required,oneof=APPROVE REVISION"`
	Opinion  string `json:"opinion" binding:"required,min=2,max=1000"`
}

type closureRequest struct {
	Materials string `json:"materials" binding:"required,min=2,max=4000"`
}

type reimbursementRequest struct {
	Evidence  string `json:"evidence" binding:"required,min=2,max=4000"`
	ID        string `json:"id" binding:"max=48"`
	InvoiceID string `json:"invoiceId" binding:"required,max=48"`
	ProjectID string `json:"projectId" binding:"required,max=48"`
}

func RegisterRoutes(router *gin.Engine, authService *auth.Service, ocrService *invoiceocr.Service) {
	authGroup := router.Group("/api/auth")
	authGroup.POST("/login", authService.Login)
	authGroup.POST("/register", registerUser(authService))
	authGroup.POST("/logout", authService.Logout)
	authGroup.GET("/me", authService.Me)
	authGroup.GET("/organizations", getPublicBusinessOrganizations)

	group := router.Group("/api")
	group.Use(authService.Require())
	group.GET("/users", getBusinessUsers)
	group.GET("/organizations", getBusinessOrganizations)
	group.POST("/organizations", auth.RequireRole("ORG_ADMIN"), createBusinessOrganization)
	group.POST("/ocr/invoice", auth.RequireRole("ISSUER", "PROJECT_MEMBER"), recognizeInvoice(ocrService))
	group.GET("/invoices", getInvoices)
	group.POST("/invoices", auth.RequireRole("ISSUER", "PROJECT_MEMBER"), createInvoice)
	group.POST("/invoices/:id/corrections", auth.RequireRole("ISSUER", "PROJECT_MEMBER"), correctInvoice)
	group.GET("/invoices/:id", getInvoice)
	group.POST("/invoices/:id/project", auth.RequireRole("ISSUER", "PROJECT_MEMBER"), linkInvoiceProject)
	group.DELETE("/invoices/:id/project", auth.RequireRole("ISSUER", "PROJECT_MEMBER"), unlinkInvoiceProject)
	group.GET("/invoices/:id/transfer", getInvoiceTransfer)
	group.POST("/invoices/:id/transfers", auth.RequireRole("ISSUER", "HOLDER", "PROJECT_MEMBER"), requestInvoiceTransfer)
	group.POST("/invoices/:id/transfers/respond", auth.RequireRole("ISSUER", "HOLDER", "PROJECT_MEMBER"), respondInvoiceTransfer)
	group.POST("/invoices/:id/transfers/cancel", auth.RequireRole("ISSUER", "HOLDER", "PROJECT_MEMBER"), cancelInvoiceTransfer)
	group.POST("/invoices/:id/void", auth.RequireRole("ISSUER"), voidInvoice)
	group.GET("/invoices/:id/void-request", getInvoiceVoidRequest)
	group.POST("/invoices/:id/void-request", auth.RequireRole("PROJECT_MEMBER"), requestInvoiceVoid)
	group.POST("/invoices/:id/void-request/review", auth.RequireRole("ISSUER"), reviewInvoiceVoid)
	group.GET("/invoices/:id/flows", getFlows)
	group.GET("/invoices/:id/history", getHistory)
	group.POST("/invoices/:id/verify", verifyInvoice)
	group.GET("/projects", getProjects)
	group.GET("/projects/:id/invoices", getProjectInvoices)
	group.GET("/projects/:id/events", getProjectEvents)
	group.GET("/projects/:id/members", getProjectMembers)
	group.POST("/projects/:id/members", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), addProjectMember)
	group.POST("/projects", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), createProject)
	group.PUT("/projects/:id", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), updateProject)
	group.POST("/projects/:id/submit", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), submitProject)
	group.POST("/projects/:id/review", auth.RequireRole("PROJECT_REVIEWER"), reviewProject)
	group.POST("/projects/:id/closure", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), requestProjectClosure)
	group.POST("/projects/:id/closure-review", auth.RequireRole("PROJECT_REVIEWER"), reviewProjectClosure)
	group.POST("/projects/:id/finalize", auth.RequireRole("FINANCE_ADMIN"), finalizeProjectSettlement)
	group.GET("/reimbursements", getReimbursements)
	group.POST("/reimbursements", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), createReimbursement)
	group.POST("/reimbursements/:id/resubmit", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), resubmitReimbursement)
	group.POST("/reimbursements/:id/withdraw", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), withdrawReimbursement)
	group.POST("/reimbursements/:id/review", auth.RequireRole("PROJECT_REVIEWER"), reviewReimbursement)
	group.POST("/reimbursements/:id/pay", auth.RequireRole("FINANCE_ADMIN"), payReimbursement)
}

// recognizeInvoice accepts a source document only for the duration of OCR.
// It deliberately does not save the original file or write it to Fabric.
func recognizeInvoice(service *invoiceocr.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !service.Enabled() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "OCR 服务尚未配置。请联系管理员设置阿里云 AccessKey 后重试。"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 11<<20)
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请选择一张不超过 10MB 的发票图片或 PDF 文件。"})
			return
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, 10<<20+1))
		if err != nil {
			serverError(c, err)
			return
		}
		result, err := service.RecognizeInvoice(header.Filename, content)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// registerUser records the business participant in Fabric first, then stores
// only its password hash in the local application account file.
func registerUser(authService *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request registerRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			badRequest(c, err)
			return
		}
		request.Username = strings.TrimSpace(request.Username)
		request.DisplayName = strings.TrimSpace(request.DisplayName)
		request.OrganizationID = strings.TrimSpace(request.OrganizationID)
		request.Role = "PROJECT_MEMBER" // public registration never grants management privileges.
		if err := authService.ValidateRegistration(auth.Principal{Username: request.Username, DisplayName: request.DisplayName, OrganizationID: request.OrganizationID, Role: request.Role}, request.Password); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		directory, err := fabric.ContractFor("Org1MSP")
		if err != nil {
			serverError(c, err)
			return
		}
		organizationResult, err := directory.EvaluateTransaction("ReadBusinessOrganization", request.OrganizationID)
		if err != nil {
			transactionError(c, err)
			return
		}
		var organization BusinessOrganization
		if err := json.Unmarshal(organizationResult, &organization); err != nil {
			serverError(c, err)
			return
		}
		if organization.Status != "ACTIVE" {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "所选业务组织已停用，请选择其他组织。"})
			return
		}
		contract, err := fabric.ContractFor(organization.MSPID)
		if err != nil {
			serverError(c, err)
			return
		}
		if _, err := contract.SubmitTransaction("RegisterBusinessUser", request.Username, request.DisplayName, organization.MSPID, request.Role, organization.ID); err != nil {
			transactionError(c, err)
			return
		}
		principal := auth.Principal{Username: request.Username, DisplayName: request.DisplayName, MSPID: organization.MSPID, OrganizationID: organization.ID, Role: request.Role}
		if err := authService.RegisterAccount(principal, request.Password); err != nil {
			serverError(c, err)
			return
		}
		authService.StartRegisteredSession(c, principal)
	}
}

func createInvoice(c *gin.Context) {
	var request createInvoiceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	request.normalize()
	if request.ID == "" {
		request.ID = newBusinessID("INV")
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if request.Currency == "" {
		request.Currency = "CNY"
	}
	hash := invoiceHash(request)
	if request.AmountCents+request.TaxCents <= 0 || request.AmountCents > 10_000_000_000-request.TaxCents {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "发票金额与税额之和必须大于 0，且不能超过系统上限。"})
		return
	}
	_, err := contract.SubmitTransaction("CreateInvoice", request.ID, request.InvoiceNo, request.IssueDate, request.Issuer, request.Buyer, request.BuyerMSPID, strconv.FormatInt(request.AmountCents, 10), strconv.FormatInt(request.TaxCents, 10), request.Currency, hash, request.ProjectID, principal.Username, principal.MSPID, principal.OrganizationID, "")
	if err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "发票已完成链上存证", "id": request.ID, "dataHash": hash})
}

func correctInvoice(c *gin.Context) {
	var request createInvoiceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	request.normalize()
	request.ID = newBusinessID("INV")
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	if request.AmountCents+request.TaxCents <= 0 || request.AmountCents > 10_000_000_000-request.TaxCents {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "更正后的金额与税额之和必须大于 0，且不能超过系统上限。"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	hash := invoiceHash(request)
	if _, err := contract.SubmitTransaction("CreateInvoice", request.ID, request.InvoiceNo, request.IssueDate, request.Issuer, request.Buyer, request.BuyerMSPID, strconv.FormatInt(request.AmountCents, 10), strconv.FormatInt(request.TaxCents, 10), request.Currency, hash, request.ProjectID, principal.Username, principal.MSPID, principal.OrganizationID, c.Param("id")); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "已创建更正版本并保留原作废存证", "id": request.ID})
}

func linkInvoiceProject(c *gin.Context) {
	var request linkProjectRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("LinkInvoiceToProject", c.Param("id"), strings.TrimSpace(request.ProjectID), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "发票已关联项目"})
}

func unlinkInvoiceProject(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("UnlinkInvoiceFromProject", c.Param("id"), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已取消发票与项目的关联"})
}

func getInvoices(c *gin.Context) {
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	result, err := contract.EvaluateTransaction("GetAllInvoices")
	if err != nil {
		transactionError(c, err)
		return
	}
	invoices, err := decodeList[Invoice](result)
	if err != nil {
		serverError(c, err)
		return
	}
	principal, _ := auth.PrincipalFromContext(c)
	projects, err := readAllProjects(contract)
	if err != nil {
		transactionError(c, err)
		return
	}
	projectByID := make(map[string]Project, len(projects))
	for _, project := range projects {
		projectByID[project.ID] = project
	}
	organizations, err := readAllOrganizations(contract)
	if err != nil {
		transactionError(c, err)
		return
	}
	keyword := strings.ToLower(strings.TrimSpace(c.Query("q")))
	summary := struct {
		Total       int   `json:"total"`
		Circulating int   `json:"circulating"`
		AmountCents int64 `json:"amountCents"`
	}{}
	visible := make([]Invoice, 0, len(invoices))
	for _, invoice := range invoices {
		if !invoiceVisibleTo(principal, invoice, projectByID, organizations) {
			continue
		}
		summary.Total++
		if invoice.Status == "IN_CIRCULATION" {
			summary.Circulating++
		}
		if invoice.Status != "VOIDED" && invoice.Currency == "CNY" {
			summary.AmountCents += invoice.TotalCents
		}
		searchable := strings.ToLower(strings.Join([]string{invoice.ID, invoice.InvoiceNo, invoice.Issuer, invoice.Buyer, invoice.CurrentHolder}, " "))
		if invoiceVisibleTo(principal, invoice, projectByID, organizations) && (keyword == "" || strings.Contains(searchable, keyword)) {
			visible = append(visible, invoice)
		}
	}
	sort.Slice(visible, func(i, j int) bool { return visible[i].CreatedAt > visible[j].CreatedAt })
	page, pageSize := positiveQuery(c, "page", 1, 100000), positiveQuery(c, "pageSize", 20, 100)
	total := len(visible)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	c.JSON(http.StatusOK, gin.H{"items": visible[start:end], "total": total, "page": page, "pageSize": pageSize, "summary": summary, "updatedAt": time.Now().UTC().Format(time.RFC3339)})
}

func positiveQuery(c *gin.Context, name string, fallback, maximum int) int {
	value, err := strconv.Atoi(c.DefaultQuery(name, strconv.Itoa(fallback)))
	if err != nil || value < 1 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func getBusinessUsers(c *gin.Context) {
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	result, err := contract.EvaluateTransaction("GetAllBusinessUsers")
	if err != nil {
		transactionError(c, err)
		return
	}
	users, err := decodeList[BusinessUser](result)
	if err != nil {
		serverError(c, err)
		return
	}
	// The organization directory is shared ledger metadata. In particular, a
	// bootstrap organization administrator has no business OrganizationID, so
	// filtering this list by the current account would incorrectly make every
	// newly registered member disappear from its view.
	c.JSON(http.StatusOK, users)
}

func createBusinessOrganization(c *gin.Context) {
	var request organizationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Type = strings.ToUpper(strings.TrimSpace(request.Type))
	request.ParentID = strings.TrimSpace(request.ParentID)
	request.Description = strings.TrimSpace(request.Description)
	if request.MSPID != principal.MSPID {
		c.JSON(http.StatusForbidden, gin.H{"error": "组织管理员只能在自己接入的 Fabric 节点登记业务组织。"})
		return
	}
	contract, err := fabric.ContractFor(request.MSPID)
	if err != nil {
		serverError(c, err)
		return
	}
	id := newBusinessID("ORG")
	if _, err := contract.SubmitTransaction("CreateBusinessOrganization", id, request.Name, request.Type, request.ParentID, request.Description, principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "业务组织已登记并写入链上", "id": id})
}

func getPublicBusinessOrganizations(c *gin.Context) {
	contract, err := fabric.ContractFor("Org1MSP")
	if err != nil {
		serverError(c, err)
		return
	}
	result, err := contract.EvaluateTransaction("GetAllBusinessOrganizations")
	if err != nil {
		transactionError(c, err)
		return
	}
	organizations, err := decodeList[BusinessOrganization](result)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, organizations)
}

func getBusinessOrganizations(c *gin.Context) {
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	result, err := contract.EvaluateTransaction("GetAllBusinessOrganizations")
	if err != nil {
		transactionError(c, err)
		return
	}
	organizations, err := decodeList[BusinessOrganization](result)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, organizations)
}

func createProject(c *gin.Context) {
	var request projectRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	request.normalize()
	if request.ID == "" {
		request.ID = newBusinessID("PRJ")
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("CreateProject", request.ID, request.Name, request.Content, strconv.FormatInt(request.BudgetCents, 10), request.ExpectedEndDate, principal.Username, principal.OrganizationID); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "项目草稿已写入链上", "id": request.ID})
}

func updateProject(c *gin.Context) {
	var request projectRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	request.ID = c.Param("id")
	request.normalize()
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("UpdateProject", request.ID, request.Name, request.Content, strconv.FormatInt(request.BudgetCents, 10), request.ExpectedEndDate, principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "项目草稿已更新"})
}

func submitProject(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("SubmitProject", c.Param("id"), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "项目申请已提交审核"})
}

func reviewProject(c *gin.Context) {
	var request reviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("ReviewProject", c.Param("id"), request.Decision, strings.TrimSpace(request.Opinion), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "项目审核结果已上链"})
}

func requestProjectClosure(c *gin.Context) {
	var request closureRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("RequestProjectClosure", c.Param("id"), strings.TrimSpace(request.Materials), textHash(request.Materials), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "结项申请与材料摘要已提交审核", "materialsHash": textHash(request.Materials)})
}

func reviewProjectClosure(c *gin.Context) {
	var request reviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("ReviewProjectClosure", c.Param("id"), request.Decision, strings.TrimSpace(request.Opinion), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "结项审核结果已上链"})
}

func finalizeProjectSettlement(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("FinalizeProjectSettlement", c.Param("id"), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "项目财务结算完成，剩余额度已回收并归档"})
}

func getProjects(c *gin.Context) {
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	result, err := contract.EvaluateTransaction("GetAllProjects")
	if err != nil {
		transactionError(c, err)
		return
	}
	projects, err := decodeList[Project](result)
	if err != nil {
		serverError(c, err)
		return
	}
	principal, _ := auth.PrincipalFromContext(c)
	organizations, err := readAllOrganizations(contract)
	if err != nil {
		transactionError(c, err)
		return
	}
	visible := make([]Project, 0, len(projects))
	for _, project := range projects {
		if projectVisibleTo(principal, project, organizations) {
			visible = append(visible, project)
		}
	}
	c.JSON(http.StatusOK, visible)
}

func getProjectInvoices(c *gin.Context) {
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	projectResult, err := contract.EvaluateTransaction("ReadProject", c.Param("id"))
	if err != nil {
		transactionError(c, err)
		return
	}
	var project Project
	if err := json.Unmarshal(projectResult, &project); err != nil {
		serverError(c, err)
		return
	}
	principal, _ := auth.PrincipalFromContext(c)
	organizations, err := readAllOrganizations(contract)
	if err != nil {
		serverError(c, err)
		return
	}
	if !projectVisibleTo(principal, project, organizations) {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权查看该项目发票"})
		return
	}
	result, err := contract.EvaluateTransaction("GetAllInvoices")
	if err != nil {
		transactionError(c, err)
		return
	}
	invoices, err := decodeList[Invoice](result)
	if err != nil {
		serverError(c, err)
		return
	}
	matched := make([]Invoice, 0)
	for _, invoice := range invoices {
		if invoice.ProjectID == project.ID {
			matched = append(matched, invoice)
		}
	}
	c.JSON(http.StatusOK, matched)
}

func getProjectEvents(c *gin.Context) {
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	projectResult, err := contract.EvaluateTransaction("ReadProject", c.Param("id"))
	if err != nil {
		transactionError(c, err)
		return
	}
	var project Project
	if err := json.Unmarshal(projectResult, &project); err != nil {
		serverError(c, err)
		return
	}
	principal, _ := auth.PrincipalFromContext(c)
	organizations, err := readAllOrganizations(contract)
	if err != nil {
		serverError(c, err)
		return
	}
	if !projectVisibleTo(principal, project, organizations) {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权查看该项目的链上事件"})
		return
	}
	result, err := contract.EvaluateTransaction("GetProjectEvents", c.Param("id"))
	if err != nil {
		transactionError(c, err)
		return
	}
	events, err := decodeList[ProjectEvent](result)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, events)
}

func getProjectMembers(c *gin.Context) {
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	projectResult, err := contract.EvaluateTransaction("ReadProject", c.Param("id"))
	if err != nil {
		transactionError(c, err)
		return
	}
	var project Project
	if err := json.Unmarshal(projectResult, &project); err != nil {
		serverError(c, err)
		return
	}
	principal, _ := auth.PrincipalFromContext(c)
	organizations, err := readAllOrganizations(contract)
	if err != nil {
		transactionError(c, err)
		return
	}
	if !projectVisibleTo(principal, project, organizations) {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权查看该项目成员名单"})
		return
	}
	result, err := contract.EvaluateTransaction("GetProjectMembers", c.Param("id"))
	if err != nil {
		transactionError(c, err)
		return
	}
	members, err := decodeList[ProjectMember](result)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, members)
}

func addProjectMember(c *gin.Context) {
	var request projectMemberRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("AddProjectMember", c.Param("id"), strings.TrimSpace(request.Username), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "项目成员已添加"})
}

func createReimbursement(c *gin.Context) {
	var request reimbursementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	request.ID = strings.TrimSpace(request.ID)
	if request.ID == "" {
		request.ID = newBusinessID("REIM")
	}
	if _, err := contract.SubmitTransaction("CreateReimbursement", request.ID, strings.TrimSpace(request.ProjectID), strings.TrimSpace(request.InvoiceID), principal.Username, strings.TrimSpace(request.Evidence), textHash(request.Evidence)); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "报销单已提交审核", "id": request.ID})
}

func resubmitReimbursement(c *gin.Context) {
	var request reimbursementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("ResubmitReimbursement", c.Param("id"), strings.TrimSpace(request.Evidence), textHash(request.Evidence), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "报销单已重新提交审核"})
}

func withdrawReimbursement(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("WithdrawReimbursement", c.Param("id"), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "报销单已撤回；如曾冻结额度，已同步释放。"})
}

func reviewReimbursement(c *gin.Context) {
	var request reviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("ReviewReimbursement", c.Param("id"), request.Decision, strings.TrimSpace(request.Opinion), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "报销审核结果已上链"})
}

func payReimbursement(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("PayReimbursement", c.Param("id"), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "报销款已支付并完成资金池扣减"})
}

func getReimbursements(c *gin.Context) {
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	result, err := contract.EvaluateTransaction("GetAllReimbursements")
	if err != nil {
		transactionError(c, err)
		return
	}
	reimbursements, err := decodeList[Reimbursement](result)
	if err != nil {
		serverError(c, err)
		return
	}
	principal, _ := auth.PrincipalFromContext(c)
	projects, err := readAllProjects(contract)
	if err != nil {
		transactionError(c, err)
		return
	}
	organizations, err := readAllOrganizations(contract)
	if err != nil {
		transactionError(c, err)
		return
	}
	projectByID := make(map[string]Project, len(projects))
	for _, project := range projects {
		projectByID[project.ID] = project
	}
	visible := make([]Reimbursement, 0, len(reimbursements))
	for _, reimbursement := range reimbursements {
		if project, exists := projectByID[reimbursement.ProjectID]; exists && projectVisibleTo(principal, project, organizations) {
			visible = append(visible, reimbursement)
		}
	}
	c.JSON(http.StatusOK, visible)
}

func readAllProjects(contract *client.Contract) ([]Project, error) {
	result, err := contract.EvaluateTransaction("GetAllProjects")
	if err != nil {
		return nil, err
	}
	return decodeList[Project](result)
}

func readAllOrganizations(contract *client.Contract) (map[string]BusinessOrganization, error) {
	result, err := contract.EvaluateTransaction("GetAllBusinessOrganizations")
	if err != nil {
		return nil, err
	}
	organizations, err := decodeList[BusinessOrganization](result)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]BusinessOrganization, len(organizations))
	for _, organization := range organizations {
		byID[organization.ID] = organization
	}
	return byID, nil
}

func projectVisibleTo(principal auth.Principal, project Project, organizations map[string]BusinessOrganization) bool {
	// Built-in course accounts bootstrap administration before a business
	// organization exists. Their chaincode authorization is role-based, so the
	// read model must not hide the very records they are allowed to review,
	// settle, audit, or administer. Registered users always have an
	// OrganizationID and continue through the scoped rules below.
	if hasBootstrapDirectoryAccess(principal) {
		return true
	}
	// The two built-in hands-on accounts deliberately have no business
	// OrganizationID. They represent the Org1 issuing/project desk, so their
	// demonstration scope is the Org1 Fabric organization rather than an empty
	// organization ID. Registered members do not enter this branch.
	if hasBuiltInOrg1WorkAccess(principal) {
		return project.ApplicantMSPID == principal.MSPID
	}
	if project.OrganizationID == "" {
		return project.ApplicantMSPID == principal.MSPID
	} // pre-upgrade legacy data
	if project.OrganizationID == principal.OrganizationID {
		return true
	}
	organization, exists := organizations[project.OrganizationID]
	return exists && principal.OrganizationID != "" && organization.ParentID == principal.OrganizationID
}

func hasBootstrapDirectoryAccess(principal auth.Principal) bool {
	if principal.OrganizationID != "" {
		return false
	}
	expected := map[string]struct{ msp, role string }{
		"project-reviewer": {"Org1MSP", "PROJECT_REVIEWER"},
		"finance-admin":    {"Org2MSP", "FINANCE_ADMIN"},
		"auditor":          {"Org1MSP", "AUDITOR"},
		"org-admin":        {"Org1MSP", "ORG_ADMIN"},
		"org-admin-org2":   {"Org2MSP", "ORG_ADMIN"},
	}
	account, exists := expected[principal.Username]
	return exists && principal.MSPID == account.msp && principal.Role == account.role
}

func hasBuiltInOrg1WorkAccess(principal auth.Principal) bool {
	if principal.OrganizationID != "" || principal.MSPID != "Org1MSP" {
		return false
	}
	return (principal.Username == "issuer-org1" && principal.Role == "ISSUER") ||
		(principal.Username == "project-member" && principal.Role == "PROJECT_MEMBER")
}

func isBuiltInOrg2Holder(principal auth.Principal) bool {
	return principal.Username == "holder-org2" && principal.MSPID == "Org2MSP" && principal.Role == "HOLDER" && principal.OrganizationID == ""
}

func invoiceVisibleTo(principal auth.Principal, invoice Invoice, projects map[string]Project, organizations map[string]BusinessOrganization) bool {
	// Review and payment screens must expose the invoice that supports a
	// reimbursement. Built-in administrators are deliberately directory-wide
	// during course-network bootstrap; they still receive only read endpoints
	// unless their role is separately authorized for a write operation.
	if hasBootstrapDirectoryAccess(principal) {
		return true
	}
	if hasBuiltInOrg1WorkAccess(principal) {
		return invoice.IssuerMSPID == principal.MSPID || invoice.HolderMSPID == principal.MSPID
	}
	// The built-in Org2 holder is a receiving desk, not an Org2-wide reader.
	// It sees only invoices whose responsibility has actually reached Org2.
	if isBuiltInOrg2Holder(principal) {
		return invoice.HolderMSPID == principal.MSPID
	}
	if invoice.ProjectID != "" {
		if project, exists := projects[invoice.ProjectID]; exists && projectVisibleTo(principal, project, organizations) {
			return true
		}
	}
	if invoice.IssuerOrganizationID != "" && invoice.IssuerOrganizationID == principal.OrganizationID {
		return true
	}
	if invoice.HolderOrganizationID != "" && invoice.HolderOrganizationID == principal.OrganizationID {
		return true
	}
	return invoice.IssuerOrganizationID == "" && (invoice.IssuerMSPID == principal.MSPID || invoice.HolderMSPID == principal.MSPID)
}

func getInvoice(c *gin.Context) {
	invoice, ok := readInvoice(c)
	if ok {
		c.JSON(http.StatusOK, invoice)
	}
}

func getInvoiceTransfer(c *gin.Context) {
	if _, ok := readInvoice(c); !ok {
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	result, err := contract.EvaluateTransaction("ReadInvoiceTransfer", c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			c.JSON(http.StatusOK, nil)
			return
		}
		transactionError(c, err)
		return
	}
	var transfer InvoiceTransfer
	if err := json.Unmarshal(result, &transfer); err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, transfer)
}

func requestInvoiceTransfer(c *gin.Context) {
	var request transferRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	invoice, ok := readInvoice(c)
	if !ok {
		return
	}
	if invoice.CurrentHolder != principal.Username {
		c.JSON(http.StatusForbidden, gin.H{"error": "只有当前责任人可以发起跨组织流转"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	_, err := contract.SubmitTransaction("RequestInvoiceTransfer", c.Param("id"), strings.TrimSpace(request.To), request.ToMSPID, strings.TrimSpace(request.Note), principal.Username)
	if err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "交接申请已提交，等待接收方确认"})
}

func respondInvoiceTransfer(c *gin.Context) {
	var request transferResponseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("RespondInvoiceTransfer", c.Param("id"), request.Decision, strings.TrimSpace(request.Response), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "交接回应已写入链上"})
}

func cancelInvoiceTransfer(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	if _, err := contract.SubmitTransaction("CancelInvoiceTransfer", c.Param("id"), principal.Username); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "交接申请已撤回"})
}

func voidInvoice(c *gin.Context) {
	var request voidRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, exists := auth.PrincipalFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	_, err := contract.SubmitTransaction("VoidInvoice", c.Param("id"), strings.TrimSpace(request.Reason), principal.Username)
	if err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "发票已作废，原始链上记录及历史已保留"})
}

func getInvoiceVoidRequest(c *gin.Context) {
	if _, ok := readInvoice(c); !ok {
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	result, err := contract.EvaluateTransaction("ReadInvoiceVoidRequest", c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			c.JSON(http.StatusOK, nil)
			return
		}
		transactionError(c, err)
		return
	}
	var request InvoiceVoidRequest
	if err := json.Unmarshal(result, &request); err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, request)
}

func requestInvoiceVoid(c *gin.Context) {
	var request voidRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	_, err := contract.SubmitTransaction("RequestInvoiceVoid", c.Param("id"), strings.TrimSpace(request.Reason), principal.Username)
	if err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "作废申请已提交，等待开票员审核"})
}

func reviewInvoiceVoid(c *gin.Context) {
	var request voidReviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	_, err := contract.SubmitTransaction("ReviewInvoiceVoid", c.Param("id"), request.Decision, strings.TrimSpace(request.Opinion), principal.Username)
	if err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "作废申请已完成审核"})
}

func getFlows(c *gin.Context) {
	if _, ok := readInvoice(c); !ok {
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	result, err := contract.EvaluateTransaction("GetInvoiceFlows", c.Param("id"))
	if err != nil {
		transactionError(c, err)
		return
	}
	flows, err := decodeList[InvoiceFlow](result)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, flows)
}

func getHistory(c *gin.Context) {
	if _, ok := readInvoice(c); !ok {
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	result, err := contract.EvaluateTransaction("GetInvoiceHistory", c.Param("id"))
	if err != nil {
		transactionError(c, err)
		return
	}
	records, err := decodeList[HistoryRecord](result)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, records)
}

func verifyInvoice(c *gin.Context) {
	var request verifyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	invoice, ok := readInvoice(c)
	if !ok {
		return
	}
	request.normalize()
	if _, err := time.Parse("2006-01-02", request.IssueDate); err != nil || request.AmountCents+request.TaxCents <= 0 || request.AmountCents > 10_000_000_000-request.TaxCents {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "请填写有效的票面内容：日期、金额和税额必须与发票一致。"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	createRequest := createInvoiceRequest{InvoiceNo: request.InvoiceNo, IssueDate: request.IssueDate, Issuer: request.Issuer, Buyer: request.Buyer, BuyerMSPID: request.BuyerMSPID, AmountCents: request.AmountCents, TaxCents: request.TaxCents, Currency: request.Currency}
	presentedHash := invoiceHash(createRequest)
	if invoice.HashVersion != "v2" {
		presentedHash = invoiceHashLegacy(createRequest)
	}
	result, err := contract.EvaluateTransaction("VerifyInvoice", c.Param("id"), presentedHash)
	if err != nil {
		transactionError(c, err)
		return
	}
	var verification any
	if err := json.Unmarshal(result, &verification); err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, verification)
}

func readInvoice(c *gin.Context) (*Invoice, bool) {
	contract, ok := contractForRequest(c)
	if !ok {
		return nil, false
	}
	result, err := contract.EvaluateTransaction("ReadInvoice", c.Param("id"))
	if err != nil {
		transactionError(c, err)
		return nil, false
	}
	var invoice Invoice
	if err := json.Unmarshal(result, &invoice); err != nil {
		serverError(c, err)
		return nil, false
	}
	principal, _ := auth.PrincipalFromContext(c)
	projects, projectErr := readAllProjects(contract)
	organizations, organizationErr := readAllOrganizations(contract)
	projectByID := make(map[string]Project, len(projects))
	for _, project := range projects {
		projectByID[project.ID] = project
	}
	if projectErr != nil {
		transactionError(c, projectErr)
		return nil, false
	}
	if organizationErr != nil {
		transactionError(c, organizationErr)
		return nil, false
	}
	if !invoiceVisibleTo(principal, invoice, projectByID, organizations) {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权查看该业务组织的发票"})
		return nil, false
	}
	return &invoice, true
}

func contractForRequest(c *gin.Context) (*client.Contract, bool) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return nil, false
	}
	contract, err := fabric.ContractFor(principal.MSPID)
	if err != nil {
		serverError(c, err)
		return nil, false
	}
	return contract, true
}

func invoiceHash(request createInvoiceRequest) string {
	// A typed JSON object prevents the delimiter ambiguity of the old "a|b"
	// concatenation scheme and stays deterministic because the struct order is fixed.
	type hashInput struct {
		Version     string `json:"version"`
		InvoiceNo   string `json:"invoiceNo"`
		IssueDate   string `json:"issueDate"`
		Issuer      string `json:"issuer"`
		Buyer       string `json:"buyer"`
		BuyerMSPID  string `json:"buyerMspId"`
		AmountCents int64  `json:"amountCents"`
		TaxCents    int64  `json:"taxCents"`
		TotalCents  int64  `json:"totalCents"`
		Currency    string `json:"currency"`
	}
	canonical, _ := json.Marshal(hashInput{"v2", request.InvoiceNo, request.IssueDate, request.Issuer, request.Buyer, request.BuyerMSPID, request.AmountCents, request.TaxCents, request.AmountCents + request.TaxCents, request.Currency})
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}

func invoiceHashLegacy(request createInvoiceRequest) string {
	canonical := strings.Join([]string{request.InvoiceNo, request.IssueDate, request.Issuer, request.Buyer, request.BuyerMSPID, strconv.FormatInt(request.AmountCents, 10), strconv.FormatInt(request.TaxCents, 10), strconv.FormatInt(request.AmountCents+request.TaxCents, 10), request.Currency}, "|")
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}

func textHash(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:])
}

func newBusinessID(prefix string) string {
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%s-%s-%d", prefix, time.Now().Format("20060102-150405"), time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%s-%s-%X", prefix, time.Now().Format("20060102-150405"), bytes)
}

func (r *createInvoiceRequest) normalize() {
	r.ID, r.InvoiceNo, r.IssueDate = strings.TrimSpace(r.ID), strings.TrimSpace(r.InvoiceNo), strings.TrimSpace(r.IssueDate)
	r.Issuer, r.Buyer, r.BuyerMSPID, r.ProjectID = strings.TrimSpace(r.Issuer), strings.TrimSpace(r.Buyer), strings.TrimSpace(r.BuyerMSPID), strings.TrimSpace(r.ProjectID)
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))
	if r.Currency == "" {
		r.Currency = "CNY"
	}
}

func (r *verifyRequest) normalize() {
	r.InvoiceNo, r.IssueDate = strings.TrimSpace(r.InvoiceNo), strings.TrimSpace(r.IssueDate)
	r.Issuer, r.Buyer, r.BuyerMSPID = strings.TrimSpace(r.Issuer), strings.TrimSpace(r.Buyer), strings.TrimSpace(r.BuyerMSPID)
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))
	if r.Currency == "" {
		r.Currency = "CNY"
	}
}

func (r *projectRequest) normalize() {
	r.ID = strings.TrimSpace(r.ID)
	r.Name = strings.TrimSpace(r.Name)
	r.Content = strings.TrimSpace(r.Content)
	r.ExpectedEndDate = strings.TrimSpace(r.ExpectedEndDate)
}

// decodeList treats an empty chaincode payload as an empty result set. Fabric's
// Go contract serializer may encode a nil slice as no bytes rather than []
// when a newly created ledger has no records yet.
func decodeList[T any](payload []byte) ([]T, error) {
	items := make([]T, 0)
	if len(bytes.TrimSpace(payload)) == 0 {
		return items, nil
	}
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, err
	}
	if items == nil {
		return make([]T, 0), nil
	}
	return items, nil
}

func badRequest(c *gin.Context, err error) {
	log.Printf("Invalid request: %v", err)
	c.JSON(http.StatusBadRequest, gin.H{"error": "提交信息不完整或格式不正确，请检查必填项、金额、日期和输入长度。"})
}
func serverError(c *gin.Context, err error) {
	log.Printf("Server error: %v", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "系统暂时无法处理该请求，请刷新页面后重试；若问题持续，请检查后端服务日志。"})
}
func transactionError(c *gin.Context, err error) {
	message := fabricErrorMessage(err)
	log.Printf("Fabric transaction rejected: %v", err)
	c.JSON(http.StatusUnprocessableEntity, gin.H{"error": message})
}

// fabricErrorMessage exposes the peer's business error rather than only the
// generic Gateway message (for example: "failed to endorse transaction").
func fabricErrorMessage(err error) string {
	grpcStatus := status.Convert(err)
	for _, detail := range grpcStatus.Details() {
		if peerDetail, ok := detail.(*gatewaypb.ErrorDetail); ok && peerDetail.GetMessage() != "" {
			return "操作未完成：" + friendlyChaincodeMessage(peerDetail.GetMessage())
		}
	}
	if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "Unavailable") {
		return "区块链网络暂不可连接，请确认 Fabric Peer 已启动后重试。"
	}
	return "区块链交易未提交，请稍后重试；若持续失败请检查后端日志。"
}

func friendlyChaincodeMessage(message string) string {
	switch {
	case strings.Contains(message, "invoice number") && strings.Contains(message, "already exists"):
		return "该发票号码已经存证，不能重复录入。"
	case strings.Contains(message, "invoice ") && strings.Contains(message, "does not exist"):
		return "所选发票不存在，请从发票列表中重新选择。"
	case strings.Contains(message, "project ") && strings.Contains(message, "does not exist"):
		return "所选项目不存在，请从项目列表中重新选择。"
	case strings.Contains(message, "already linked to reimbursement"):
		return "这张发票已经关联过报销单，不能重复报销。"
	case strings.Contains(message, "available balance is insufficient"):
		return "项目资金池余额不足，无法审核通过这笔报销。"
	case strings.Contains(message, "is not linked to project"):
		return "所选发票没有关联当前项目，请选择该项目下的发票。"
	case strings.Contains(message, "cannot be reimbursed"):
		return "该发票已作废，不能用于报销。"
	case strings.Contains(message, "not pending review"):
		return "该记录当前不处于待审核状态，可能已被其他人处理。"
	case strings.Contains(message, "cannot be submitted") || strings.Contains(message, "cannot be edited"):
		return "当前项目状态不允许此操作，请刷新后确认项目状态。"
	default:
		return message
	}
}
