package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const (
	invoicePrefix              = "INVOICE#"
	flowPrefix                 = "FLOW#"
	projectPrefix              = "PROJECT#"
	projectEventPrefix         = "PROJECT_EVENT#"
	reimbursementPrefix        = "REIMBURSEMENT#"
	reimbursementInvoicePrefix = "REIMBURSEMENT_INVOICE#"
	voidRequestPrefix          = "VOID_REQUEST#"
	userPrefix                 = "USER#"
	organizationPrefix         = "ORGANIZATION#"
)

// Invoice is the on-chain business record. Monetary values use cents to avoid
// floating point precision loss in endorsement and verification.
type Invoice struct {
	AmountCents   int64  `json:"amountCents"`
	Buyer         string `json:"buyer"`
	BuyerMSPID    string `json:"buyerMspId"`
	CreatedAt     string `json:"createdAt"`
	Currency      string `json:"currency"`
	CurrentHolder string `json:"currentHolder"`
	HolderMSPID   string `json:"holderMspId"`
	DataHash      string `json:"dataHash"`
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
	// Keep this field in every JSON response (including legacy, non-voided
	// invoices) because Fabric Contract API validates response schemas.
	VoidReason string `json:"voidReason"`
}

// InvoiceFlow is append-only evidence for one handoff.
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

// InvoiceVoidRequest records a project member's request to cancel an invoice.
// The request itself does not change the invoice. Only an issuer can approve it
// and create the final VOIDED state.
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

// BusinessUser is an application-level participant. Its record is stored on
// the ledger, while the actual transaction is still signed by the test
// certificate representing the participant's organization.
type BusinessUser struct {
	CreatedAt      string `json:"createdAt"`
	DisplayName    string `json:"displayName"`
	MSPID          string `json:"mspId"`
	OrganizationID string `json:"organizationId"`
	Role           string `json:"role"`
	Status         string `json:"status"`
	Username       string `json:"username"`
}

// BusinessOrganization represents an application-level organization. It is
// distinct from Fabric MSPs: the course network still has only Org1MSP and
// Org2MSP as technical blockchain organizations.
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

// Project is a budget-controlled project application and delivery record.
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

type VerificationResult struct {
	DataHashMatched bool     `json:"dataHashMatched"`
	Invoice         *Invoice `json:"invoice"`
	Reason          string   `json:"reason"`
	Valid           bool     `json:"valid"`
}

type HistoryRecord struct {
	IsDelete  bool     `json:"isDelete"`
	Timestamp string   `json:"timestamp"`
	TxID      string   `json:"txId"`
	Value     *Invoice `json:"value,omitempty"`
}

type InvoiceContract struct {
	contractapi.Contract
}

func invoiceKey(id string) string                     { return invoicePrefix + id }
func userKey(username string) string                  { return userPrefix + username }
func organizationKey(id string) string                { return organizationPrefix + id }
func projectKey(id string) string                     { return projectPrefix + id }
func reimbursementKey(id string) string               { return reimbursementPrefix + id }
func reimbursementInvoiceKey(invoiceID string) string { return reimbursementInvoicePrefix + invoiceID }
func voidRequestKey(invoiceID string) string          { return voidRequestPrefix + invoiceID }

// CreateInvoice writes an invoice into the normal project/reimbursement flow.
// The buyer is an invoice business field, while initialHolder is the current
// responsible person. This keeps optional cross-organization handoff separate
// from the fixed reimbursement process.
func (s *InvoiceContract) CreateInvoice(
	ctx contractapi.TransactionContextInterface,
	id, invoiceNo, issueDate, issuer, buyer, buyerMSPID, amountCentsText, taxCentsText, currency, dataHash, projectID, initialHolder, initialHolderMSPID string,
) error {
	id = strings.TrimSpace(id)
	invoiceNo = strings.TrimSpace(invoiceNo)
	issuer = strings.TrimSpace(issuer)
	buyer = strings.TrimSpace(buyer)
	buyerMSPID = strings.TrimSpace(buyerMSPID)
	initialHolder = strings.TrimSpace(initialHolder)
	initialHolderMSPID = strings.TrimSpace(initialHolderMSPID)
	if id == "" || invoiceNo == "" || issueDate == "" || issuer == "" || buyer == "" || initialHolder == "" {
		return fmt.Errorf("invoice fields must not be empty")
	}
	if strings.ContainsAny(id, "#\x00") {
		return fmt.Errorf("invoice id contains an unsupported character")
	}
	issuerMSPID, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if err := validateBusinessMSPID(issuerMSPID); err != nil {
		return err
	}
	if err := validateBusinessMSPID(buyerMSPID); err != nil {
		return fmt.Errorf("buyer organization: %w", err)
	}
	if err := validateBusinessMSPID(initialHolderMSPID); err != nil {
		return fmt.Errorf("initial holder organization: %w", err)
	}
	if issuerMSPID != initialHolderMSPID {
		return fmt.Errorf("initial holder must belong to the submitting organization %s", issuerMSPID)
	}
	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		project, err := s.ReadProject(ctx, projectID)
		if err != nil {
			return fmt.Errorf("linked project: %w", err)
		}
		if project.Status != "EXECUTING" && project.Status != "CLOSURE_APPROVED" {
			return fmt.Errorf("linked project %s is not available for reimbursement", projectID)
		}
		if project.ApplicantMSPID != issuerMSPID {
			return fmt.Errorf("linked project belongs to %s, not issuer organization %s", project.ApplicantMSPID, issuerMSPID)
		}
	}
	if len(dataHash) != 64 {
		return fmt.Errorf("data hash must be a 64-character SHA-256 hex string")
	}
	amountCents, err := parseNonNegative(amountCentsText, "amount")
	if err != nil {
		return err
	}
	taxCents, err := parseNonNegative(taxCentsText, "tax")
	if err != nil {
		return err
	}
	exists, err := s.InvoiceExists(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("invoice %s already exists", id)
	}
	numberExists, err := s.InvoiceNumberExists(ctx, invoiceNo)
	if err != nil {
		return err
	}
	if numberExists {
		return fmt.Errorf("invoice number %s already exists", invoiceNo)
	}

	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	invoice := Invoice{
		ID: id, InvoiceNo: invoiceNo, IssueDate: issueDate, Issuer: issuer, Buyer: buyer, BuyerMSPID: buyerMSPID,
		AmountCents: amountCents, TaxCents: taxCents, TotalCents: amountCents + taxCents,
		Currency: strings.ToUpper(strings.TrimSpace(currency)), DataHash: strings.ToLower(dataHash), ProjectID: projectID,
		CurrentHolder: initialHolder, HolderMSPID: initialHolderMSPID, IssuerMSPID: issuerMSPID,
		Status: "ISSUED", CreatedAt: now, UpdatedAt: now,
	}
	if invoice.Currency == "" {
		invoice.Currency = "CNY"
	}
	if err := putJSON(ctx, invoiceKey(id), invoice); err != nil {
		return err
	}
	return s.createFlow(ctx, id, "ISSUE", issuer, initialHolder, issuerMSPID, now)
}

// TransferInvoice changes the holder only when the certificate caller belongs
// to the organization that currently holds the invoice.
func (s *InvoiceContract) TransferInvoice(ctx contractapi.TransactionContextInterface, id, to, toMSPID string) error {
	invoice, err := s.ReadInvoice(ctx, id)
	if err != nil {
		return err
	}
	to = strings.TrimSpace(to)
	toMSPID = strings.TrimSpace(toMSPID)
	if to == "" || to == invoice.CurrentHolder {
		return fmt.Errorf("transfer participants are invalid")
	}
	if invoice.Status == "VOIDED" {
		return fmt.Errorf("a voided invoice cannot be transferred")
	}
	if err := validateBusinessMSPID(toMSPID); err != nil {
		return fmt.Errorf("target organization: %w", err)
	}
	if err := s.requireActiveHolder(ctx, to, toMSPID); err != nil {
		return fmt.Errorf("target holder: %w", err)
	}
	callerMSP, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if invoice.HolderMSPID == "" {
		return fmt.Errorf("legacy invoice has no holder organization; create a new invoice after upgrading")
	}
	if callerMSP != invoice.HolderMSPID {
		return fmt.Errorf("only %s can transfer this invoice", invoice.HolderMSPID)
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	from := invoice.CurrentHolder
	invoice.CurrentHolder = to
	invoice.HolderMSPID = toMSPID
	invoice.Status = "IN_CIRCULATION"
	invoice.UpdatedAt = now
	if err := putJSON(ctx, invoiceKey(id), invoice); err != nil {
		return err
	}
	return s.createFlow(ctx, id, "TRANSFER", from, to, callerMSP, now)
}

// VoidInvoice is an append-only cancellation action. It deliberately keeps the
// original invoice and its history instead of deleting on-chain evidence.
func (s *InvoiceContract) VoidInvoice(ctx contractapi.TransactionContextInterface, id, reason string) error {
	invoice, err := s.ReadInvoice(ctx, id)
	if err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("void reason must not be empty")
	}
	if invoice.Status == "VOIDED" {
		return fmt.Errorf("invoice %s is already voided", id)
	}
	callerMSP, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if invoice.IssuerMSPID == "" {
		return fmt.Errorf("legacy invoice has no issuer organization; create a new invoice after upgrading")
	}
	if callerMSP != invoice.IssuerMSPID {
		return fmt.Errorf("only issuer organization %s can void this invoice", invoice.IssuerMSPID)
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	from := invoice.CurrentHolder
	invoice.Status = "VOIDED"
	invoice.VoidReason = reason
	invoice.UpdatedAt = now
	if err := putJSON(ctx, invoiceKey(id), invoice); err != nil {
		return err
	}
	return s.createFlow(ctx, id, "VOID", from, "VOIDED", callerMSP, now)
}

// RequestInvoiceVoid lets the applicant of an invoice-linked project request
// cancellation. The issuer reviews it before the invoice can be voided.
func (s *InvoiceContract) RequestInvoiceVoid(ctx contractapi.TransactionContextInterface, id, reason, applicant string) error {
	invoice, err := s.ReadInvoice(ctx, id)
	if err != nil {
		return err
	}
	reason, applicant = strings.TrimSpace(reason), strings.TrimSpace(applicant)
	if reason == "" || applicant == "" {
		return fmt.Errorf("void request reason and applicant must not be empty")
	}
	if invoice.Status == "VOIDED" {
		return fmt.Errorf("invoice %s is already voided", id)
	}
	if invoice.ProjectID == "" {
		return fmt.Errorf("only an invoice linked to a project can use the void-request process")
	}
	project, err := s.ReadProject(ctx, invoice.ProjectID)
	if err != nil {
		return fmt.Errorf("read linked project: %w", err)
	}
	if project.Applicant != applicant {
		return fmt.Errorf("only the applicant of project %s can request this invoice to be voided", project.Name)
	}
	callerMSP, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if callerMSP != project.ApplicantMSPID {
		return fmt.Errorf("only project organization %s can submit this void request", project.ApplicantMSPID)
	}
	request, err := s.ReadInvoiceVoidRequest(ctx, id)
	if err != nil && !isMissingVoidRequest(err) {
		return err
	}
	if request != nil && request.Status == "PENDING_REVIEW" {
		return fmt.Errorf("this invoice already has a pending void request")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	if request == nil {
		request = &InvoiceVoidRequest{InvoiceID: id, CreatedAt: now}
	}
	request.Applicant, request.Reason, request.Status, request.UpdatedAt = applicant, reason, "PENDING_REVIEW", now
	request.Reviewer, request.ReviewOpinion = "", ""
	if err := putJSON(ctx, voidRequestKey(id), request); err != nil {
		return err
	}
	return s.createFlow(ctx, id, "VOID_REQUEST", applicant, "ISSUER_REVIEW", callerMSP, now)
}

// ReviewInvoiceVoid keeps cancellation authority with the issuer organization.
func (s *InvoiceContract) ReviewInvoiceVoid(ctx contractapi.TransactionContextInterface, id, decision, opinion, reviewer string) error {
	invoice, err := s.ReadInvoice(ctx, id)
	if err != nil {
		return err
	}
	request, err := s.ReadInvoiceVoidRequest(ctx, id)
	if err != nil {
		return err
	}
	decision, opinion, reviewer = strings.ToUpper(strings.TrimSpace(decision)), strings.TrimSpace(opinion), strings.TrimSpace(reviewer)
	if request.Status != "PENDING_REVIEW" {
		return fmt.Errorf("void request for invoice %s is not pending review", id)
	}
	if (decision != "APPROVE" && decision != "REJECT") || opinion == "" || reviewer == "" {
		return fmt.Errorf("void review requires APPROVE or REJECT, an opinion, and a reviewer")
	}
	callerMSP, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if callerMSP != invoice.IssuerMSPID {
		return fmt.Errorf("only issuer organization %s can review this void request", invoice.IssuerMSPID)
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	request.Reviewer, request.ReviewOpinion, request.UpdatedAt = reviewer, opinion, now
	if decision == "REJECT" {
		request.Status = "REJECTED"
		if err := putJSON(ctx, voidRequestKey(id), request); err != nil {
			return err
		}
		return s.createFlow(ctx, id, "VOID_REJECTED", reviewer, request.Applicant, callerMSP, now)
	}
	if invoice.Status == "VOIDED" {
		return fmt.Errorf("invoice %s is already voided", id)
	}
	request.Status = "APPROVED"
	invoice.Status, invoice.VoidReason, invoice.UpdatedAt = "VOIDED", request.Reason, now
	if err := putJSON(ctx, invoiceKey(id), invoice); err != nil {
		return err
	}
	if err := putJSON(ctx, voidRequestKey(id), request); err != nil {
		return err
	}
	return s.createFlow(ctx, id, "VOID", invoice.CurrentHolder, "VOIDED", callerMSP, now)
}

func (s *InvoiceContract) ReadInvoiceVoidRequest(ctx contractapi.TransactionContextInterface, id string) (*InvoiceVoidRequest, error) {
	data, err := ctx.GetStub().GetState(voidRequestKey(strings.TrimSpace(id)))
	if err != nil {
		return nil, fmt.Errorf("read void request: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("void request for invoice %s does not exist", id)
	}
	var request InvoiceVoidRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, fmt.Errorf("decode void request: %w", err)
	}
	return &request, nil
}

func isMissingVoidRequest(err error) bool {
	return err != nil && strings.Contains(err.Error(), "does not exist")
}

func (s *InvoiceContract) ReadInvoice(ctx contractapi.TransactionContextInterface, id string) (*Invoice, error) {
	data, err := ctx.GetStub().GetState(invoiceKey(id))
	if err != nil {
		return nil, fmt.Errorf("read invoice: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("invoice %s does not exist", id)
	}
	var invoice Invoice
	if err := json.Unmarshal(data, &invoice); err != nil {
		return nil, fmt.Errorf("decode invoice: %w", err)
	}
	return &invoice, nil
}

func (s *InvoiceContract) InvoiceExists(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
	data, err := ctx.GetStub().GetState(invoiceKey(id))
	return data != nil, err
}

// RegisterBusinessUser creates an immutable business-user registration. This
// is deliberately different from issuing an X.509 certificate: in this course
// network a representative User1 certificate signs for each organization.
func (s *InvoiceContract) RegisterBusinessUser(ctx contractapi.TransactionContextInterface, username, displayName, mspID, role, organizationID string) error {
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	mspID = strings.TrimSpace(mspID)
	role = strings.ToUpper(strings.TrimSpace(role))
	organizationID = strings.TrimSpace(organizationID)
	if username == "" || displayName == "" {
		return fmt.Errorf("username and display name must not be empty")
	}
	if strings.ContainsAny(username, "#\x00 \t\n") {
		return fmt.Errorf("username contains an unsupported character")
	}
	if err := validateBusinessMSPID(mspID); err != nil {
		return err
	}
	if !validBusinessRole(role) {
		return fmt.Errorf("unsupported business role %s", role)
	}
	callerMSP, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if callerMSP != mspID {
		return fmt.Errorf("only %s can register a user for %s", mspID, mspID)
	}
	organization, err := s.ReadBusinessOrganization(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("business organization: %w", err)
	}
	if organization.Status != "ACTIVE" {
		return fmt.Errorf("business organization %s is not active", organization.Name)
	}
	if organization.MSPID != mspID {
		return fmt.Errorf("business organization %s belongs to %s, not %s", organization.Name, organization.MSPID, mspID)
	}
	exists, err := s.BusinessUserExists(ctx, username)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("business user %s already exists", username)
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	return putJSON(ctx, userKey(username), BusinessUser{Username: username, DisplayName: displayName, MSPID: mspID, OrganizationID: organizationID, Role: role, Status: "ACTIVE", CreatedAt: now})
}

func (s *InvoiceContract) ReadBusinessUser(ctx contractapi.TransactionContextInterface, username string) (*BusinessUser, error) {
	username = strings.TrimSpace(username)
	data, err := ctx.GetStub().GetState(userKey(username))
	if err != nil {
		return nil, fmt.Errorf("read business user: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("business user %s does not exist; please register first", username)
	}
	var user BusinessUser
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("decode business user: %w", err)
	}
	return &user, nil
}

func (s *InvoiceContract) BusinessUserExists(ctx contractapi.TransactionContextInterface, username string) (bool, error) {
	data, err := ctx.GetStub().GetState(userKey(strings.TrimSpace(username)))
	return data != nil, err
}

func (s *InvoiceContract) GetAllBusinessUsers(ctx contractapi.TransactionContextInterface) ([]*BusinessUser, error) {
	iterator, err := ctx.GetStub().GetStateByRange(userPrefix, "USER$")
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	var users []*BusinessUser
	for iterator.HasNext() {
		item, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		var user BusinessUser
		if err := json.Unmarshal(item.Value, &user); err != nil {
			return nil, err
		}
		users = append(users, &user)
	}
	return users, nil
}

// CreateBusinessOrganization registers a business organization on the ledger.
// It does not create a Fabric peer, MSP, or certificate; those are network
// administrator operations outside this web application.
func (s *InvoiceContract) CreateBusinessOrganization(ctx contractapi.TransactionContextInterface, id, name, organizationType, parentID, description, creator string) error {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	organizationType = strings.ToUpper(strings.TrimSpace(organizationType))
	parentID = strings.TrimSpace(parentID)
	description = strings.TrimSpace(description)
	creator = strings.TrimSpace(creator)
	if id == "" || name == "" || creator == "" {
		return fmt.Errorf("organization id, name and creator must not be empty")
	}
	if strings.ContainsAny(id, "#\x00") {
		return fmt.Errorf("organization id contains an unsupported character")
	}
	if !validBusinessOrganizationType(organizationType) {
		return fmt.Errorf("unsupported organization type %s", organizationType)
	}
	if organizationType == "PROJECT_TEAM" {
		if parentID == "" {
			return fmt.Errorf("a project team must select its parent primary organization")
		}
		parent, err := s.ReadBusinessOrganization(ctx, parentID)
		if err != nil {
			return fmt.Errorf("parent organization: %w", err)
		}
		if parent.Type != "PRIMARY" {
			return fmt.Errorf("a project team must belong to a primary organization")
		}
	} else if parentID != "" {
		return fmt.Errorf("only a project team can have a parent organization")
	}
	exists, err := s.BusinessOrganizationExists(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("business organization %s already exists", id)
	}
	mspID, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	organization := BusinessOrganization{ID: id, Name: name, Type: organizationType, ParentID: parentID, Description: description, MSPID: mspID, Status: "ACTIVE", CreatedBy: creator, CreatedAt: now}
	return putJSON(ctx, organizationKey(id), organization)
}

func (s *InvoiceContract) ReadBusinessOrganization(ctx contractapi.TransactionContextInterface, id string) (*BusinessOrganization, error) {
	data, err := ctx.GetStub().GetState(organizationKey(strings.TrimSpace(id)))
	if err != nil {
		return nil, fmt.Errorf("read business organization: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("business organization %s does not exist", id)
	}
	var organization BusinessOrganization
	if err := json.Unmarshal(data, &organization); err != nil {
		return nil, fmt.Errorf("decode business organization: %w", err)
	}
	return &organization, nil
}

func (s *InvoiceContract) BusinessOrganizationExists(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
	data, err := ctx.GetStub().GetState(organizationKey(strings.TrimSpace(id)))
	return data != nil, err
}

func (s *InvoiceContract) GetAllBusinessOrganizations(ctx contractapi.TransactionContextInterface) ([]*BusinessOrganization, error) {
	iterator, err := ctx.GetStub().GetStateByRange(organizationPrefix, "ORGANIZATION$")
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	organizations := make([]*BusinessOrganization, 0)
	for iterator.HasNext() {
		item, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		var organization BusinessOrganization
		if err := json.Unmarshal(item.Value, &organization); err != nil {
			return nil, err
		}
		organizations = append(organizations, &organization)
	}
	return organizations, nil
}

func (s *InvoiceContract) requireActiveHolder(ctx contractapi.TransactionContextInterface, username, mspID string) error {
	user, err := s.ReadBusinessUser(ctx, username)
	if err != nil {
		return err
	}
	if user.Status != "ACTIVE" {
		return fmt.Errorf("business user %s is not active", user.Username)
	}
	if user.MSPID != mspID {
		return fmt.Errorf("business user %s belongs to %s, not %s", user.Username, user.MSPID, mspID)
	}
	if user.Role != "HOLDER" {
		return fmt.Errorf("business user %s must have HOLDER role to receive an invoice", user.Username)
	}
	return nil
}

// CreateProject starts a draft project application. The applicant may edit it
// until it is explicitly submitted for review.
func (s *InvoiceContract) CreateProject(ctx contractapi.TransactionContextInterface, id, name, content, budgetCentsText, expectedEndDate, applicant string) error {
	id, name, content = strings.TrimSpace(id), strings.TrimSpace(name), strings.TrimSpace(content)
	expectedEndDate, applicant = strings.TrimSpace(expectedEndDate), strings.TrimSpace(applicant)
	if id == "" || name == "" || content == "" || expectedEndDate == "" || applicant == "" {
		return fmt.Errorf("project fields must not be empty")
	}
	if strings.ContainsAny(id, "#\x00") {
		return fmt.Errorf("project id contains an unsupported character")
	}
	budgetCents, err := parseNonNegative(budgetCentsText, "project budget")
	if err != nil || budgetCents == 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("project budget must be greater than zero")
	}
	exists, err := s.ProjectExists(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("project %s already exists", id)
	}
	mspID, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project := Project{ID: id, Name: name, Content: content, BudgetCents: budgetCents, ExpectedEndDate: expectedEndDate, Applicant: applicant, ApplicantMSPID: mspID, Status: "DRAFT", CreatedAt: now, UpdatedAt: now}
	if err := putJSON(ctx, projectKey(id), project); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, id, "CREATE_DRAFT", applicant, "")
}

func (s *InvoiceContract) UpdateProject(ctx contractapi.TransactionContextInterface, id, name, content, budgetCentsText, expectedEndDate, applicant string) error {
	project, err := s.ReadProject(ctx, id)
	if err != nil {
		return err
	}
	if project.Status != "DRAFT" && project.Status != "REVISION_REQUIRED" {
		return fmt.Errorf("project %s cannot be edited in status %s", id, project.Status)
	}
	if err := s.requireProjectApplicant(ctx, project, applicant); err != nil {
		return err
	}
	name, content, expectedEndDate = strings.TrimSpace(name), strings.TrimSpace(content), strings.TrimSpace(expectedEndDate)
	if name == "" || content == "" || expectedEndDate == "" {
		return fmt.Errorf("project fields must not be empty")
	}
	budgetCents, err := parseNonNegative(budgetCentsText, "project budget")
	if err != nil || budgetCents == 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("project budget must be greater than zero")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project.Name, project.Content, project.BudgetCents = name, content, budgetCents
	project.ExpectedEndDate, project.Status, project.UpdatedAt = expectedEndDate, "DRAFT", now
	project.ReviewOpinion, project.Reviewer = "", ""
	if err := putJSON(ctx, projectKey(id), project); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, id, "UPDATE_DRAFT", applicant, "")
}

func (s *InvoiceContract) SubmitProject(ctx contractapi.TransactionContextInterface, id, applicant string) error {
	project, err := s.ReadProject(ctx, id)
	if err != nil {
		return err
	}
	if project.Status != "DRAFT" && project.Status != "REVISION_REQUIRED" {
		return fmt.Errorf("project %s cannot be submitted in status %s", id, project.Status)
	}
	if err := s.requireProjectApplicant(ctx, project, applicant); err != nil {
		return err
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project.Status, project.UpdatedAt = "PENDING_REVIEW", now
	if err := putJSON(ctx, projectKey(id), project); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, id, "SUBMIT_APPLICATION", applicant, "")
}

// ReviewProject either activates the project and its funding pool or returns
// it for revision. A written opinion is required for both decisions.
func (s *InvoiceContract) ReviewProject(ctx contractapi.TransactionContextInterface, id, decision, opinion, reviewer string) error {
	project, err := s.ReadProject(ctx, id)
	if err != nil {
		return err
	}
	if project.Status != "PENDING_REVIEW" {
		return fmt.Errorf("project %s is not pending review", id)
	}
	decision, opinion, reviewer = strings.ToUpper(strings.TrimSpace(decision)), strings.TrimSpace(opinion), strings.TrimSpace(reviewer)
	if reviewer == "" || opinion == "" {
		return fmt.Errorf("reviewer and review opinion must not be empty")
	}
	if decision != "APPROVE" && decision != "REVISION" {
		return fmt.Errorf("review decision must be APPROVE or REVISION")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project.Reviewer, project.ReviewOpinion, project.UpdatedAt = reviewer, opinion, now
	if decision == "APPROVE" {
		project.Status = "EXECUTING"
		project.AvailableCents = project.BudgetCents
	} else {
		project.Status = "REVISION_REQUIRED"
	}
	if err := putJSON(ctx, projectKey(id), project); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, id, "PROJECT_"+decision, reviewer, opinion)
}

func (s *InvoiceContract) RequestProjectClosure(ctx contractapi.TransactionContextInterface, id, materialsHash, applicant string) error {
	project, err := s.ReadProject(ctx, id)
	if err != nil {
		return err
	}
	if project.Status != "EXECUTING" {
		return fmt.Errorf("project %s cannot request closure in status %s", id, project.Status)
	}
	if err := s.requireProjectApplicant(ctx, project, applicant); err != nil {
		return err
	}
	materialsHash = strings.ToLower(strings.TrimSpace(materialsHash))
	if len(materialsHash) != 64 {
		return fmt.Errorf("closure materials hash must be a 64-character SHA-256 hex string")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project.Status, project.ClosureMaterialsHash, project.UpdatedAt = "CLOSURE_REVIEW", materialsHash, now
	if err := putJSON(ctx, projectKey(id), project); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, id, "REQUEST_CLOSURE", applicant, materialsHash)
}

func (s *InvoiceContract) ReviewProjectClosure(ctx contractapi.TransactionContextInterface, id, decision, opinion, reviewer string) error {
	project, err := s.ReadProject(ctx, id)
	if err != nil {
		return err
	}
	if project.Status != "CLOSURE_REVIEW" {
		return fmt.Errorf("project %s is not pending closure review", id)
	}
	decision, opinion, reviewer = strings.ToUpper(strings.TrimSpace(decision)), strings.TrimSpace(opinion), strings.TrimSpace(reviewer)
	if reviewer == "" || opinion == "" {
		return fmt.Errorf("reviewer and review opinion must not be empty")
	}
	if decision != "APPROVE" && decision != "REVISION" {
		return fmt.Errorf("review decision must be APPROVE or REVISION")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project.Reviewer, project.ReviewOpinion, project.UpdatedAt = reviewer, opinion, now
	if decision == "APPROVE" {
		project.Status = "CLOSURE_APPROVED"
	} else {
		project.Status = "EXECUTING"
	}
	if err := putJSON(ctx, projectKey(id), project); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, id, "CLOSURE_"+decision, reviewer, opinion)
}

func (s *InvoiceContract) ReadProject(ctx contractapi.TransactionContextInterface, id string) (*Project, error) {
	data, err := ctx.GetStub().GetState(projectKey(strings.TrimSpace(id)))
	if err != nil {
		return nil, fmt.Errorf("read project: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("project %s does not exist", id)
	}
	var project Project
	if err := json.Unmarshal(data, &project); err != nil {
		return nil, fmt.Errorf("decode project: %w", err)
	}
	return &project, nil
}

func (s *InvoiceContract) ProjectExists(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
	data, err := ctx.GetStub().GetState(projectKey(strings.TrimSpace(id)))
	return data != nil, err
}

func (s *InvoiceContract) GetAllProjects(ctx contractapi.TransactionContextInterface) ([]*Project, error) {
	iterator, err := ctx.GetStub().GetStateByRange(projectPrefix, "PROJECT$")
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	projects := make([]*Project, 0)
	for iterator.HasNext() {
		item, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		var project Project
		if err := json.Unmarshal(item.Value, &project); err != nil {
			return nil, err
		}
		projects = append(projects, &project)
	}
	return projects, nil
}

func (s *InvoiceContract) GetProjectEvents(ctx contractapi.TransactionContextInterface, projectID string) ([]*ProjectEvent, error) {
	prefix := projectEventPrefix + strings.TrimSpace(projectID) + "#"
	iterator, err := ctx.GetStub().GetStateByRange(prefix, prefix+"\uffff")
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	events := make([]*ProjectEvent, 0)
	for iterator.HasNext() {
		item, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		var event ProjectEvent
		if err := json.Unmarshal(item.Value, &event); err != nil {
			return nil, err
		}
		events = append(events, &event)
	}
	return events, nil
}

func (s *InvoiceContract) CreateReimbursement(ctx contractapi.TransactionContextInterface, id, projectID, invoiceID, applicant, evidenceHash string) error {
	id, projectID, invoiceID, applicant = strings.TrimSpace(id), strings.TrimSpace(projectID), strings.TrimSpace(invoiceID), strings.TrimSpace(applicant)
	if id == "" || projectID == "" || invoiceID == "" || applicant == "" {
		return fmt.Errorf("reimbursement fields must not be empty")
	}
	if strings.ContainsAny(id, "#\x00") {
		return fmt.Errorf("reimbursement id contains an unsupported character")
	}
	exists, err := s.ReimbursementExists(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("reimbursement %s already exists", id)
	}
	project, err := s.ReadProject(ctx, projectID)
	if err != nil {
		return err
	}
	if project.Status != "EXECUTING" && project.Status != "CLOSURE_APPROVED" {
		return fmt.Errorf("project %s is not open for reimbursement", projectID)
	}
	if err := s.requireProjectApplicant(ctx, project, applicant); err != nil {
		return err
	}
	invoice, err := s.ReadInvoice(ctx, invoiceID)
	if err != nil {
		return err
	}
	if invoice.Status == "VOIDED" {
		return fmt.Errorf("voided invoice %s cannot be reimbursed", invoiceID)
	}
	if invoice.ProjectID != projectID {
		return fmt.Errorf("invoice %s is not linked to project %s", invoiceID, projectID)
	}
	linked, err := ctx.GetStub().GetState(reimbursementInvoiceKey(invoiceID))
	if err != nil {
		return err
	}
	if linked != nil {
		return fmt.Errorf("invoice %s is already linked to reimbursement %s", invoiceID, string(linked))
	}
	evidenceHash = strings.ToLower(strings.TrimSpace(evidenceHash))
	if len(evidenceHash) != 64 {
		return fmt.Errorf("reimbursement evidence hash must be a 64-character SHA-256 hex string")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	reimbursement := Reimbursement{ID: id, ProjectID: projectID, InvoiceID: invoiceID, Applicant: applicant, AmountCents: invoice.TotalCents, EvidenceHash: evidenceHash, Status: "PENDING_REVIEW", CreatedAt: now, UpdatedAt: now}
	if err := putJSON(ctx, reimbursementKey(id), reimbursement); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(reimbursementInvoiceKey(invoiceID), []byte(id)); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, projectID, "SUBMIT_REIMBURSEMENT", applicant, id)
}

func (s *InvoiceContract) ReviewReimbursement(ctx contractapi.TransactionContextInterface, id, decision, opinion, reviewer string) error {
	reimbursement, err := s.ReadReimbursement(ctx, id)
	if err != nil {
		return err
	}
	if reimbursement.Status != "PENDING_REVIEW" {
		return fmt.Errorf("reimbursement %s is not pending review", id)
	}
	decision, opinion, reviewer = strings.ToUpper(strings.TrimSpace(decision)), strings.TrimSpace(opinion), strings.TrimSpace(reviewer)
	if reviewer == "" || opinion == "" {
		return fmt.Errorf("reviewer and review opinion must not be empty")
	}
	if decision != "APPROVE" && decision != "REVISION" {
		return fmt.Errorf("review decision must be APPROVE or REVISION")
	}
	project, err := s.ReadProject(ctx, reimbursement.ProjectID)
	if err != nil {
		return err
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	reimbursement.Reviewer, reimbursement.ReviewOpinion, reimbursement.UpdatedAt = reviewer, opinion, now
	if decision == "APPROVE" {
		if project.AvailableCents < reimbursement.AmountCents {
			return fmt.Errorf("project available balance is insufficient: available %d cents, requested %d cents", project.AvailableCents, reimbursement.AmountCents)
		}
		project.AvailableCents -= reimbursement.AmountCents
		project.ReservedCents += reimbursement.AmountCents
		project.UpdatedAt = now
		reimbursement.Status = "APPROVED_RESERVED"
		if err := putJSON(ctx, projectKey(project.ID), project); err != nil {
			return err
		}
	} else {
		reimbursement.Status = "REVISION_REQUIRED"
	}
	if err := putJSON(ctx, reimbursementKey(id), reimbursement); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, reimbursement.ProjectID, "REIMBURSEMENT_"+decision, reviewer, opinion)
}

func (s *InvoiceContract) ResubmitReimbursement(ctx contractapi.TransactionContextInterface, id, evidenceHash, applicant string) error {
	reimbursement, err := s.ReadReimbursement(ctx, id)
	if err != nil {
		return err
	}
	if reimbursement.Status != "REVISION_REQUIRED" {
		return fmt.Errorf("reimbursement %s cannot be resubmitted in status %s", id, reimbursement.Status)
	}
	project, err := s.ReadProject(ctx, reimbursement.ProjectID)
	if err != nil {
		return err
	}
	if err := s.requireProjectApplicant(ctx, project, applicant); err != nil {
		return err
	}
	evidenceHash = strings.ToLower(strings.TrimSpace(evidenceHash))
	if len(evidenceHash) != 64 {
		return fmt.Errorf("reimbursement evidence hash must be a 64-character SHA-256 hex string")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	reimbursement.EvidenceHash, reimbursement.Status, reimbursement.UpdatedAt = evidenceHash, "PENDING_REVIEW", now
	reimbursement.Reviewer, reimbursement.ReviewOpinion = "", ""
	if err := putJSON(ctx, reimbursementKey(id), reimbursement); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, reimbursement.ProjectID, "RESUBMIT_REIMBURSEMENT", applicant, id)
}

func (s *InvoiceContract) PayReimbursement(ctx contractapi.TransactionContextInterface, id, financeOperator string) error {
	reimbursement, err := s.ReadReimbursement(ctx, id)
	if err != nil {
		return err
	}
	if reimbursement.Status != "APPROVED_RESERVED" {
		return fmt.Errorf("reimbursement %s is not approved for payment", id)
	}
	financeOperator = strings.TrimSpace(financeOperator)
	if financeOperator == "" {
		return fmt.Errorf("finance operator must not be empty")
	}
	project, err := s.ReadProject(ctx, reimbursement.ProjectID)
	if err != nil {
		return err
	}
	if project.ReservedCents < reimbursement.AmountCents {
		return fmt.Errorf("project reserved balance is inconsistent")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project.ReservedCents -= reimbursement.AmountCents
	project.PaidCents += reimbursement.AmountCents
	project.UpdatedAt = now
	reimbursement.Status, reimbursement.UpdatedAt = "PAID", now
	if err := putJSON(ctx, projectKey(project.ID), project); err != nil {
		return err
	}
	if err := putJSON(ctx, reimbursementKey(id), reimbursement); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, project.ID, "PAY_REIMBURSEMENT", financeOperator, id)
}

func (s *InvoiceContract) ReadReimbursement(ctx contractapi.TransactionContextInterface, id string) (*Reimbursement, error) {
	data, err := ctx.GetStub().GetState(reimbursementKey(strings.TrimSpace(id)))
	if err != nil {
		return nil, fmt.Errorf("read reimbursement: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("reimbursement %s does not exist", id)
	}
	var reimbursement Reimbursement
	if err := json.Unmarshal(data, &reimbursement); err != nil {
		return nil, fmt.Errorf("decode reimbursement: %w", err)
	}
	return &reimbursement, nil
}

func (s *InvoiceContract) ReimbursementExists(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
	data, err := ctx.GetStub().GetState(reimbursementKey(strings.TrimSpace(id)))
	return data != nil, err
}

func (s *InvoiceContract) GetAllReimbursements(ctx contractapi.TransactionContextInterface) ([]*Reimbursement, error) {
	iterator, err := ctx.GetStub().GetStateByRange(reimbursementPrefix, "REIMBURSEMENT$")
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	reimbursements := make([]*Reimbursement, 0)
	for iterator.HasNext() {
		item, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		var reimbursement Reimbursement
		if err := json.Unmarshal(item.Value, &reimbursement); err != nil {
			return nil, err
		}
		reimbursements = append(reimbursements, &reimbursement)
	}
	return reimbursements, nil
}

func (s *InvoiceContract) requireProjectApplicant(ctx contractapi.TransactionContextInterface, project *Project, applicant string) error {
	if strings.TrimSpace(applicant) != project.Applicant {
		return fmt.Errorf("only project applicant %s can perform this action", project.Applicant)
	}
	caller, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if caller != project.ApplicantMSPID {
		return fmt.Errorf("only project organization %s can perform this action", project.ApplicantMSPID)
	}
	return nil
}

func (s *InvoiceContract) createProjectEvent(ctx contractapi.TransactionContextInterface, projectID, eventType, actor, note string) error {
	timestamp, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	txID := ctx.GetStub().GetTxID()
	event := ProjectEvent{ID: projectEventPrefix + projectID + "#" + txID, ProjectID: projectID, Type: eventType, Actor: actor, Note: note, TxID: txID, Timestamp: timestamp}
	return putJSON(ctx, event.ID, event)
}

// InvoiceNumberExists prevents the same business invoice number from being
// stored under different application IDs. It scans invoice state so that
// records created before this rule was introduced are covered as well.
func (s *InvoiceContract) InvoiceNumberExists(ctx contractapi.TransactionContextInterface, invoiceNo string) (bool, error) {
	iterator, err := ctx.GetStub().GetStateByRange(invoicePrefix, "INVOICE$")
	if err != nil {
		return false, err
	}
	defer iterator.Close()
	for iterator.HasNext() {
		item, err := iterator.Next()
		if err != nil {
			return false, err
		}
		var invoice Invoice
		if err := json.Unmarshal(item.Value, &invoice); err != nil {
			return false, err
		}
		if invoice.InvoiceNo == invoiceNo {
			return true, nil
		}
	}
	return false, nil
}

func (s *InvoiceContract) GetAllInvoices(ctx contractapi.TransactionContextInterface) ([]*Invoice, error) {
	iterator, err := ctx.GetStub().GetStateByRange(invoicePrefix, "INVOICE$")
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	var invoices []*Invoice
	for iterator.HasNext() {
		item, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		var invoice Invoice
		if err := json.Unmarshal(item.Value, &invoice); err != nil {
			return nil, err
		}
		invoices = append(invoices, &invoice)
	}
	return invoices, nil
}

func (s *InvoiceContract) GetInvoiceFlows(ctx contractapi.TransactionContextInterface, id string) ([]*InvoiceFlow, error) {
	prefix := flowPrefix + id + "#"
	iterator, err := ctx.GetStub().GetStateByRange(prefix, prefix+"\uffff")
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	var flows []*InvoiceFlow
	for iterator.HasNext() {
		item, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		var flow InvoiceFlow
		if err := json.Unmarshal(item.Value, &flow); err != nil {
			return nil, err
		}
		flows = append(flows, &flow)
	}
	return flows, nil
}

// VerifyInvoice compares a presented QR/content hash with the hash stored on-chain.
func (s *InvoiceContract) VerifyInvoice(ctx contractapi.TransactionContextInterface, id, presentedHash string) (*VerificationResult, error) {
	invoice, err := s.ReadInvoice(ctx, id)
	if err != nil {
		return nil, err
	}
	matched := strings.EqualFold(strings.TrimSpace(presentedHash), invoice.DataHash)
	result := &VerificationResult{Invoice: invoice, DataHashMatched: matched, Valid: matched}
	if matched {
		result.Reason = "content hash matches the on-chain invoice record"
	} else {
		result.Reason = "content hash does not match the on-chain record"
	}
	return result, nil
}

func (s *InvoiceContract) GetInvoiceHistory(ctx contractapi.TransactionContextInterface, id string) ([]*HistoryRecord, error) {
	iterator, err := ctx.GetStub().GetHistoryForKey(invoiceKey(id))
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	var records []*HistoryRecord
	for iterator.HasNext() {
		entry, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		record := &HistoryRecord{TxID: entry.TxId, IsDelete: entry.IsDelete, Timestamp: timestampText(entry.Timestamp.Seconds, int64(entry.Timestamp.Nanos))}
		if !entry.IsDelete && entry.Value != nil {
			var invoice Invoice
			if err := json.Unmarshal(entry.Value, &invoice); err != nil {
				return nil, err
			}
			record.Value = &invoice
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *InvoiceContract) createFlow(ctx contractapi.TransactionContextInterface, invoiceID, flowType, from, to, operator, timestamp string) error {
	txID := ctx.GetStub().GetTxID()
	flow := InvoiceFlow{ID: flowPrefix + invoiceID + "#" + txID, InvoiceID: invoiceID, Type: flowType, From: from, To: to, Operator: operator, TxID: txID, Timestamp: timestamp}
	return putJSON(ctx, flow.ID, flow)
}

func parseNonNegative(value, field string) (int64, error) {
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil || result < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer in cents", field)
	}
	return result, nil
}

func putJSON(ctx contractapi.TransactionContextInterface, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(key, data)
}

func transactionTime(ctx contractapi.TransactionContextInterface) (string, error) {
	timestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return "", err
	}
	return timestampText(timestamp.Seconds, int64(timestamp.Nanos)), nil
}

func timestampText(seconds, nanos int64) string {
	return time.Unix(seconds, nanos).UTC().Format(time.RFC3339)
}

func callerMSPID(ctx contractapi.TransactionContextInterface) (string, error) {
	mspID, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return "", fmt.Errorf("read caller MSP: %w", err)
	}
	return mspID, nil
}

func validateBusinessMSPID(mspID string) error {
	if mspID != "Org1MSP" && mspID != "Org2MSP" {
		return fmt.Errorf("unsupported organization %s", mspID)
	}
	return nil
}

func validBusinessRole(role string) bool {
	return role == "ISSUER" || role == "HOLDER" || role == "AUDITOR" || role == "PROJECT_MEMBER" || role == "PROJECT_REVIEWER" || role == "FINANCE_ADMIN" || role == "ORG_ADMIN"
}

func validBusinessOrganizationType(organizationType string) bool {
	return organizationType == "PRIMARY" || organizationType == "PROJECT_TEAM" || organizationType == "EXTERNAL"
}
