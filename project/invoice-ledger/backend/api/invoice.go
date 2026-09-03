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
	AmountCents   int64  `json:"amountCents"`
	Buyer         string `json:"buyer"`
	BuyerMSPID    string `json:"buyerMspId"`
	CreatedAt     string `json:"createdAt"`
	Currency      string `json:"currency"`
	CurrentHolder string `json:"currentHolder"`
	DataHash      string `json:"dataHash"`
	HolderMSPID   string `json:"holderMspId"`
	ID            string `json:"id"`
	InvoiceNo     string `json:"invoiceNo"`
	IssueDate     string `json:"issueDate"`
	Issuer        string `json:"issuer"`
	IssuerMSPID   string `json:"issuerMspId"`
	ProjectID     string `json:"projectId"`
	Status        string `json:"status"`
	TaxCents      int64  `json:"taxCents"`
	TotalCents    int64  `json:"totalCents"`
	UpdatedAt     string `json:"updatedAt"`
	VoidReason    string `json:"voidReason"`
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
	AvailableCents       int64  `json:"availableCents"`
	BudgetCents          int64  `json:"budgetCents"`
	ClosureMaterialsHash string `json:"closureMaterialsHash"`
	Content              string `json:"content"`
	CreatedAt            string `json:"createdAt"`
	ExpectedEndDate      string `json:"expectedEndDate"`
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	PaidCents            int64  `json:"paidCents"`
	ReservedCents        int64  `json:"reservedCents"`
	ReviewOpinion        string `json:"reviewOpinion"`
	Reviewer             string `json:"reviewer"`
	Status               string `json:"status"`
	UpdatedAt            string `json:"updatedAt"`
}

type ProjectEvent struct {
	Actor     string `json:"actor"`
	ID        string `json:"id"`
	Note      string `json:"note"`
	ProjectID string `json:"projectId"`
	Timestamp string `json:"timestamp"`
	TxID      string `json:"txId"`
	Type      string `json:"type"`
}

type Reimbursement struct {
	AmountCents   int64  `json:"amountCents"`
	Applicant     string `json:"applicant"`
	CreatedAt     string `json:"createdAt"`
	EvidenceHash  string `json:"evidenceHash"`
	ID            string `json:"id"`
	InvoiceID     string `json:"invoiceId"`
	ProjectID     string `json:"projectId"`
	ReviewOpinion string `json:"reviewOpinion"`
	Reviewer      string `json:"reviewer"`
	Status        string `json:"status"`
	UpdatedAt     string `json:"updatedAt"`
}

type createInvoiceRequest struct {
	AmountCents int64  `json:"amountCents" binding:"gte=0"`
	Buyer       string `json:"buyer" binding:"required"`
	BuyerMSPID  string `json:"buyerMspId" binding:"required,oneof=Org1MSP Org2MSP"`
	Currency    string `json:"currency"`
	ID          string `json:"id" binding:"required"`
	InvoiceNo   string `json:"invoiceNo" binding:"required"`
	IssueDate   string `json:"issueDate" binding:"required"`
	Issuer      string `json:"issuer" binding:"required"`
	ProjectID   string `json:"projectId"`
	TaxCents    int64  `json:"taxCents" binding:"gte=0"`
}

type transferRequest struct {
	To      string `json:"to" binding:"required"`
	ToMSPID string `json:"toMspId" binding:"required,oneof=Org1MSP Org2MSP"`
}

type voidRequest struct {
	Reason string `json:"reason" binding:"required,min=2,max=200"`
}

type verifyRequest struct {
	DataHash string `json:"dataHash" binding:"required,len=64"`
}

type registerRequest struct {
	DisplayName    string `json:"displayName" binding:"required,min=2,max=48"`
	OrganizationID string `json:"organizationId" binding:"required,max=48"`
	Password       string `json:"password" binding:"required,min=6,max=72"`
	Role           string `json:"role" binding:"required,oneof=ISSUER HOLDER AUDITOR PROJECT_MEMBER PROJECT_REVIEWER FINANCE_ADMIN ORG_ADMIN"`
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
	BudgetCents     int64  `json:"budgetCents" binding:"gt=0"`
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
	group.GET("/invoices/:id", getInvoice)
	group.POST("/invoices/:id/transfers", auth.RequireRole("ISSUER", "HOLDER", "PROJECT_MEMBER"), transferInvoice)
	group.POST("/invoices/:id/void", auth.RequireRole("ISSUER"), voidInvoice)
	group.GET("/invoices/:id/flows", getFlows)
	group.GET("/invoices/:id/history", getHistory)
	group.POST("/invoices/:id/verify", verifyInvoice)
	group.GET("/projects", getProjects)
	group.GET("/projects/:id/events", getProjectEvents)
	group.POST("/projects", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), createProject)
	group.PUT("/projects/:id", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), updateProject)
	group.POST("/projects/:id/submit", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), submitProject)
	group.POST("/projects/:id/review", auth.RequireRole("PROJECT_REVIEWER"), reviewProject)
	group.POST("/projects/:id/closure", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), requestProjectClosure)
	group.POST("/projects/:id/closure-review", auth.RequireRole("PROJECT_REVIEWER"), reviewProjectClosure)
	group.GET("/reimbursements", getReimbursements)
	group.POST("/reimbursements", auth.RequireRole("PROJECT_MEMBER", "ISSUER"), createReimbursement)
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
	principal, ok := auth.PrincipalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	hash := invoiceHash(request)
	_, err := contract.SubmitTransaction("CreateInvoice", request.ID, request.InvoiceNo, request.IssueDate, request.Issuer, request.Buyer, request.BuyerMSPID, strconv.FormatInt(request.AmountCents, 10), strconv.FormatInt(request.TaxCents, 10), request.Currency, hash, request.ProjectID, principal.Username, principal.MSPID)
	if err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "发票已完成链上存证", "id": request.ID, "dataHash": hash})
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
	c.JSON(http.StatusOK, invoices)
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
	if _, err := contract.SubmitTransaction("CreateProject", request.ID, request.Name, request.Content, strconv.FormatInt(request.BudgetCents, 10), request.ExpectedEndDate, principal.Username); err != nil {
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
	if _, err := contract.SubmitTransaction("RequestProjectClosure", c.Param("id"), textHash(request.Materials), principal.Username); err != nil {
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
	c.JSON(http.StatusOK, projects)
}

func getProjectEvents(c *gin.Context) {
	contract, ok := contractForRequest(c)
	if !ok {
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
	if _, err := contract.SubmitTransaction("CreateReimbursement", request.ID, strings.TrimSpace(request.ProjectID), strings.TrimSpace(request.InvoiceID), principal.Username, textHash(request.Evidence)); err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "报销单已提交审核", "id": request.ID})
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
	c.JSON(http.StatusOK, reimbursements)
}

func getInvoice(c *gin.Context) {
	invoice, ok := readInvoice(c)
	if ok {
		c.JSON(http.StatusOK, invoice)
	}
}

func transferInvoice(c *gin.Context) {
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
	_, err := contract.SubmitTransaction("TransferInvoice", c.Param("id"), strings.TrimSpace(request.To), request.ToMSPID)
	if err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "发票流转已上链"})
}

func voidInvoice(c *gin.Context) {
	var request voidRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, err)
		return
	}
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	_, err := contract.SubmitTransaction("VoidInvoice", c.Param("id"), strings.TrimSpace(request.Reason))
	if err != nil {
		transactionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "发票已作废，原始链上记录及历史已保留"})
}

func getFlows(c *gin.Context) {
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
	contract, ok := contractForRequest(c)
	if !ok {
		return
	}
	result, err := contract.EvaluateTransaction("VerifyInvoice", c.Param("id"), strings.ToLower(strings.TrimSpace(request.DataHash)))
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
