package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const maxMoneyCents int64 = 10_000_000_000 // ¥100,000,000.00: enough for this course system, bounded for safety.

const (
	invoicePrefix              = "INVOICE#"
	invoiceNumberPrefix        = "INVOICE_NUMBER#"
	flowPrefix                 = "FLOW#"
	projectPrefix              = "PROJECT#"
	projectMemberPrefix        = "PROJECT_MEMBER#"
	projectEventPrefix         = "PROJECT_EVENT#"
	reimbursementPrefix        = "REIMBURSEMENT#"
	reimbursementInvoicePrefix = "REIMBURSEMENT_INVOICE#"
	voidRequestPrefix          = "VOID_REQUEST#"
	transferRequestPrefix      = "TRANSFER_REQUEST#"
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
	// HashVersion identifies the canonicalization used for DataHash. Records
	// created before v2.2 have an empty version and use the legacy format.
	HashVersion          string `json:"hashVersion"`
	ID                   string `json:"id"`
	InvoiceNo            string `json:"invoiceNo"`
	IssueDate            string `json:"issueDate"`
	Issuer               string `json:"issuer"`
	IssuerOrganizationID string `json:"issuerOrganizationId"`
	IssuerMSPID          string `json:"issuerMspId"`
	HolderOrganizationID string `json:"holderOrganizationId"`
	ProjectID            string `json:"projectId"`
	CorrectionOf         string `json:"correctionOf"`
	Status               string `json:"status"`
	TaxCents             int64  `json:"taxCents"`
	TotalCents           int64  `json:"totalCents"`
	UpdatedAt            string `json:"updatedAt"`
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

// ProjectMember is separate from the business-organization directory: being
// in an organization does not automatically grant access to every project.
type ProjectMember struct {
	ProjectID string `json:"projectId"`
	Username  string `json:"username"`
	Role      string `json:"role"` // LEADER or MEMBER
	AddedAt   string `json:"addedAt"`
	AddedBy   string `json:"addedBy"`
}

// InvoiceTransfer is a pending material handoff. The responsibility changes
// only after the selected recipient accepts it.
type InvoiceTransfer struct {
	InvoiceID string `json:"invoiceId"`
	From      string `json:"from"`
	To        string `json:"to"`
	ToMSPID   string `json:"toMspId"`
	Note      string `json:"note"`
	Status    string `json:"status"` // PENDING, ACCEPTED, REJECTED, CANCELLED
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

func invoiceKey(id string) string { return invoicePrefix + id }
func invoiceNumberKey(invoiceNo string) string {
	return invoiceNumberPrefix + strings.TrimSpace(invoiceNo)
}
func userKey(username string) string                  { return userPrefix + username }
func organizationKey(id string) string                { return organizationPrefix + id }
func projectKey(id string) string                     { return projectPrefix + id }
func reimbursementKey(id string) string               { return reimbursementPrefix + id }
func reimbursementInvoiceKey(invoiceID string) string { return reimbursementInvoicePrefix + invoiceID }
func voidRequestKey(invoiceID string) string          { return voidRequestPrefix + invoiceID }
func projectMemberKey(projectID, username string) string {
	return projectMemberPrefix + projectID + "#" + username
}
func transferRequestKey(invoiceID string) string { return transferRequestPrefix + invoiceID }

// CreateInvoice writes an invoice into the normal project/reimbursement flow.
// The buyer is an invoice business field, while initialHolder is the current
// responsible person. This keeps optional cross-organization handoff separate
// from the fixed reimbursement process.
func (s *InvoiceContract) CreateInvoice(
	ctx contractapi.TransactionContextInterface,
	id, invoiceNo, issueDate, issuer, buyer, buyerMSPID, amountCentsText, taxCentsText, currency, dataHash, projectID, initialHolder, initialHolderMSPID, issuerOrganizationID, correctionOf string,
) error {
	id = strings.TrimSpace(id)
	invoiceNo = strings.TrimSpace(invoiceNo)
	issuer = strings.TrimSpace(issuer)
	buyer = strings.TrimSpace(buyer)
	buyerMSPID = strings.TrimSpace(buyerMSPID)
	initialHolder = strings.TrimSpace(initialHolder)
	initialHolderMSPID = strings.TrimSpace(initialHolderMSPID)
	issuerOrganizationID = strings.TrimSpace(issuerOrganizationID)
	correctionOf = strings.TrimSpace(correctionOf)
	if id == "" || invoiceNo == "" || issueDate == "" || issuer == "" || buyer == "" || initialHolder == "" {
		return fmt.Errorf("invoice fields must not be empty")
	}
	if !validISODate(issueDate) {
		return fmt.Errorf("issue date must be a valid YYYY-MM-DD date")
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
	if strings.ToUpper(strings.TrimSpace(currency)) != "CNY" && strings.TrimSpace(currency) != "" {
		return fmt.Errorf("only CNY invoices are supported in the current project budget")
	}
	if err := s.requireActiveBusinessUser(ctx, initialHolder, issuerMSPID, "ISSUER", "PROJECT_MEMBER"); err != nil {
		return fmt.Errorf("invoice creator: %w", err)
	}
	creatorOrganizationID, err := s.businessUserOrganization(ctx, initialHolder)
	if err != nil {
		return err
	}
	if issuerOrganizationID == "" {
		issuerOrganizationID = creatorOrganizationID
	}
	if creatorOrganizationID != "" && creatorOrganizationID != issuerOrganizationID {
		return fmt.Errorf("invoice creator does not belong to organization %s", issuerOrganizationID)
	}
	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		project, err := s.ReadProject(ctx, projectID)
		if err != nil {
			return fmt.Errorf("linked project: %w", err)
		}
		if project.Status != "EXECUTING" && project.Status != "FINANCIAL_SETTLEMENT" && project.Status != "CLOSURE_APPROVED" {
			return fmt.Errorf("linked project %s is not available for invoice association", projectID)
		}
		if project.ApplicantMSPID != issuerMSPID {
			return fmt.Errorf("linked project belongs to %s, not issuer organization %s", project.ApplicantMSPID, issuerMSPID)
		}
		if project.OrganizationID != "" && project.OrganizationID != issuerOrganizationID {
			return fmt.Errorf("linked project belongs to business organization %s", project.OrganizationID)
		}
		if err := s.requireProjectMember(ctx, project, initialHolder); err != nil {
			return fmt.Errorf("linked project member: %w", err)
		}
	}
	if len(dataHash) != 64 {
		return fmt.Errorf("data hash must be a 64-character SHA-256 hex string")
	}
	if _, err := hex.DecodeString(dataHash); err != nil {
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
	totalCents, err := checkedAddMoney(amountCents, taxCents)
	if err != nil || totalCents <= 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("invoice total must be greater than zero")
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
	if numberExists && correctionOf == "" {
		return fmt.Errorf("invoice number %s already exists", invoiceNo)
	}
	if correctionOf != "" {
		original, err := s.ReadInvoice(ctx, correctionOf)
		if err != nil {
			return fmt.Errorf("corrected original: %w", err)
		}
		if original.Status != "VOIDED" {
			return fmt.Errorf("only a voided invoice record can be corrected")
		}
		if original.IssuerMSPID != issuerMSPID || (original.IssuerOrganizationID != "" && original.IssuerOrganizationID != issuerOrganizationID) {
			return fmt.Errorf("only the original issuer organization can create a correction")
		}
		if original.InvoiceNo != invoiceNo {
			return fmt.Errorf("corrected invoice must keep the original invoice number")
		}
		if err := s.ensureInvoiceCanBeVoided(ctx, correctionOf); err != nil {
			return fmt.Errorf("corrected original still has active reimbursement: %w", err)
		}
		if exists, err := s.InvoiceNumberExistsExcept(ctx, invoiceNo, correctionOf); err != nil {
			return err
		} else if exists {
			return fmt.Errorf("invoice number %s already has another correction version", invoiceNo)
		}
	}

	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	invoice := Invoice{
		ID: id, InvoiceNo: invoiceNo, IssueDate: issueDate, Issuer: issuer, Buyer: buyer, BuyerMSPID: buyerMSPID,
		AmountCents: amountCents, TaxCents: taxCents, TotalCents: totalCents,
		Currency: strings.ToUpper(strings.TrimSpace(currency)), DataHash: strings.ToLower(dataHash), HashVersion: "v2", ProjectID: projectID, CorrectionOf: correctionOf,
		CurrentHolder: initialHolder, HolderMSPID: initialHolderMSPID, IssuerMSPID: issuerMSPID, IssuerOrganizationID: issuerOrganizationID, HolderOrganizationID: issuerOrganizationID,
		Status: "ISSUED", CreatedAt: now, UpdatedAt: now,
	}
	if invoice.Currency == "" {
		invoice.Currency = "CNY"
	}
	if err := putJSON(ctx, invoiceKey(id), invoice); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(invoiceNumberKey(invoiceNo), []byte(id)); err != nil {
		return err
	}
	flowType := "ISSUE"
	if correctionOf != "" {
		flowType = "CORRECTION"
	}
	return s.createFlow(ctx, id, flowType, issuer, initialHolder, issuerMSPID, now)
}

// LinkInvoiceToProject fills a missing project association without altering the
// invoice's genuine business fields. It is allowed only before reimbursement.
func (s *InvoiceContract) LinkInvoiceToProject(ctx contractapi.TransactionContextInterface, invoiceID, projectID, operator string) error {
	invoice, err := s.ReadInvoice(ctx, invoiceID)
	if err != nil {
		return err
	}
	if invoice.Status == "VOIDED" {
		return fmt.Errorf("voided invoice cannot be linked to a project")
	}
	if invoice.ProjectID != "" {
		return fmt.Errorf("invoice %s is already linked to project %s", invoiceID, invoice.ProjectID)
	}
	if err := s.ensureInvoiceCanBeVoided(ctx, invoiceID); err != nil {
		return fmt.Errorf("invoice already has reimbursement activity: %w", err)
	}
	project, err := s.ReadProject(ctx, projectID)
	if err != nil {
		return err
	}
	if project.Status != "EXECUTING" && project.Status != "FINANCIAL_SETTLEMENT" && project.Status != "CLOSURE_APPROVED" {
		return fmt.Errorf("project %s is not open for invoice association", projectID)
	}
	if err := s.requireProjectMember(ctx, project, operator); err != nil {
		return err
	}
	if invoice.IssuerMSPID != project.ApplicantMSPID || (invoice.IssuerOrganizationID != "" && project.OrganizationID != "" && invoice.IssuerOrganizationID != project.OrganizationID) {
		return fmt.Errorf("invoice and project must belong to the same business organization")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	invoice.ProjectID, invoice.UpdatedAt = project.ID, now
	if err := putJSON(ctx, invoiceKey(invoice.ID), invoice); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, project.ID, "LINK_INVOICE", operator, invoice.ID)
}

// UnlinkInvoiceFromProject removes an accidental project association before the
// invoice enters any reimbursement activity. The LINK/UNLINK project events
// preserve an auditable relationship history without changing the invoice's
// genuine business fields.
func (s *InvoiceContract) UnlinkInvoiceFromProject(ctx contractapi.TransactionContextInterface, invoiceID, operator string) error {
	invoice, err := s.ReadInvoice(ctx, invoiceID)
	if err != nil {
		return err
	}
	if invoice.Status == "VOIDED" {
		return fmt.Errorf("voided invoice cannot have its project association changed")
	}
	if invoice.ProjectID == "" {
		return fmt.Errorf("invoice %s is not linked to a project", invoiceID)
	}
	if err := s.ensureInvoiceCanBeVoided(ctx, invoiceID); err != nil {
		return fmt.Errorf("invoice already has reimbursement activity: %w", err)
	}
	project, err := s.ReadProject(ctx, invoice.ProjectID)
	if err != nil {
		return err
	}
	if project.Status == "ARCHIVED" {
		return fmt.Errorf("archived project cannot change invoice associations")
	}
	if err := s.requireProjectMember(ctx, project, strings.TrimSpace(operator)); err != nil {
		return err
	}
	if invoice.IssuerMSPID != project.ApplicantMSPID || (invoice.IssuerOrganizationID != "" && project.OrganizationID != "" && invoice.IssuerOrganizationID != project.OrganizationID) {
		return fmt.Errorf("invoice and project must belong to the same business organization")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	projectID := invoice.ProjectID
	invoice.ProjectID, invoice.UpdatedAt = "", now
	if err := putJSON(ctx, invoiceKey(invoice.ID), invoice); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, projectID, "UNLINK_INVOICE", operator, invoice.ID)
}

// TransferInvoice changes the holder only when the certificate caller belongs
// to the organization that currently holds the invoice.
func (s *InvoiceContract) TransferInvoice(ctx contractapi.TransactionContextInterface, id, to, toMSPID, operator string) error {
	invoice, err := s.ReadInvoice(ctx, id)
	if err != nil {
		return err
	}
	to, operator = strings.TrimSpace(to), strings.TrimSpace(operator)
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
	if invoice.CurrentHolder != operator {
		return fmt.Errorf("only current holder %s can transfer this invoice", invoice.CurrentHolder)
	}
	if err := s.requireActiveBusinessUser(ctx, operator, callerMSP, "ISSUER", "HOLDER", "PROJECT_MEMBER"); err != nil {
		return fmt.Errorf("transfer operator: %w", err)
	}
	operatorOrganizationID, err := s.businessUserOrganization(ctx, operator)
	if err != nil {
		return err
	}
	if invoice.HolderOrganizationID != "" && operatorOrganizationID != "" && operatorOrganizationID != invoice.HolderOrganizationID {
		return fmt.Errorf("current holder does not belong to invoice holder organization")
	}
	targetOrganizationID, err := s.businessUserOrganization(ctx, to)
	if err != nil {
		return err
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	from := invoice.CurrentHolder
	invoice.CurrentHolder = to
	invoice.HolderMSPID = toMSPID
	invoice.HolderOrganizationID = targetOrganizationID
	invoice.Status = "IN_CIRCULATION"
	invoice.UpdatedAt = now
	if err := putJSON(ctx, invoiceKey(id), invoice); err != nil {
		return err
	}
	return s.createFlow(ctx, id, "TRANSFER", from, to, callerMSP, now)
}

// RequestInvoiceTransfer records a proposed material handoff. It intentionally
// does not change CurrentHolder; the recipient must accept it in a later tx.
func (s *InvoiceContract) RequestInvoiceTransfer(ctx contractapi.TransactionContextInterface, id, to, toMSPID, note, operator string) error {
	invoice, err := s.ReadInvoice(ctx, id)
	if err != nil {
		return err
	}
	to, note, operator = strings.TrimSpace(to), strings.TrimSpace(note), strings.TrimSpace(operator)
	if to == "" || to == invoice.CurrentHolder {
		return fmt.Errorf("transfer participants are invalid")
	}
	if invoice.Status == "VOIDED" {
		return fmt.Errorf("a voided invoice cannot be transferred")
	}
	caller, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if caller != invoice.HolderMSPID || operator != invoice.CurrentHolder {
		return fmt.Errorf("only current holder %s can request this transfer", invoice.CurrentHolder)
	}
	if err := s.requireActiveBusinessUser(ctx, operator, caller, "ISSUER", "HOLDER", "PROJECT_MEMBER"); err != nil {
		return err
	}
	if err := s.requireRegisteredRole(ctx, to, strings.TrimSpace(toMSPID), "ISSUER", "HOLDER", "PROJECT_MEMBER"); err != nil {
		return fmt.Errorf("transfer recipient: %w", err)
	}
	existing, err := s.ReadInvoiceTransfer(ctx, id)
	if err != nil && !isMissingTransfer(err) {
		return err
	}
	if existing != nil && existing.Status == "PENDING" {
		return fmt.Errorf("invoice already has a pending transfer")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	transfer := InvoiceTransfer{InvoiceID: id, From: operator, To: to, ToMSPID: strings.TrimSpace(toMSPID), Note: note, Status: "PENDING", CreatedAt: now, UpdatedAt: now}
	if err := putJSON(ctx, transferRequestKey(id), transfer); err != nil {
		return err
	}
	return s.createFlow(ctx, id, "TRANSFER_REQUEST", operator, to, caller, now)
}

func (s *InvoiceContract) RespondInvoiceTransfer(ctx contractapi.TransactionContextInterface, id, decision, response, recipient string) error {
	transfer, err := s.ReadInvoiceTransfer(ctx, id)
	if err != nil {
		return err
	}
	if transfer.Status != "PENDING" {
		return fmt.Errorf("transfer is not pending")
	}
	decision, response, recipient = strings.ToUpper(strings.TrimSpace(decision)), strings.TrimSpace(response), strings.TrimSpace(recipient)
	if decision != "ACCEPT" && decision != "REJECT" {
		return fmt.Errorf("transfer decision must be ACCEPT or REJECT")
	}
	if recipient != transfer.To {
		return fmt.Errorf("only selected recipient %s can respond", transfer.To)
	}
	if err := s.requireActiveBusinessUser(ctx, recipient, transfer.ToMSPID, "ISSUER", "HOLDER", "PROJECT_MEMBER"); err != nil {
		return err
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	transfer.UpdatedAt, transfer.Response = now, response
	if decision == "REJECT" {
		transfer.Status = "REJECTED"
		if err := putJSON(ctx, transferRequestKey(id), transfer); err != nil {
			return err
		}
		return s.createFlow(ctx, id, "TRANSFER_REJECTED", transfer.From, transfer.To, transfer.ToMSPID, now)
	}
	invoice, err := s.ReadInvoice(ctx, id)
	if err != nil {
		return err
	}
	if invoice.CurrentHolder != transfer.From || invoice.Status == "VOIDED" {
		return fmt.Errorf("invoice is no longer available for this transfer")
	}
	targetOrganizationID, err := s.businessUserOrganization(ctx, recipient)
	if err != nil {
		return err
	}
	invoice.CurrentHolder, invoice.HolderMSPID, invoice.HolderOrganizationID, invoice.Status, invoice.UpdatedAt = recipient, transfer.ToMSPID, targetOrganizationID, "IN_CIRCULATION", now
	transfer.Status = "ACCEPTED"
	if err := putJSON(ctx, invoiceKey(id), invoice); err != nil {
		return err
	}
	if err := putJSON(ctx, transferRequestKey(id), transfer); err != nil {
		return err
	}
	return s.createFlow(ctx, id, "TRANSFER_ACCEPTED", transfer.From, recipient, transfer.ToMSPID, now)
}

func (s *InvoiceContract) CancelInvoiceTransfer(ctx contractapi.TransactionContextInterface, id, operator string) error {
	transfer, err := s.ReadInvoiceTransfer(ctx, id)
	if err != nil {
		return err
	}
	if transfer.Status != "PENDING" {
		return fmt.Errorf("only a pending transfer can be cancelled")
	}
	if strings.TrimSpace(operator) != transfer.From {
		return fmt.Errorf("only transfer requester can cancel")
	}
	invoice, err := s.ReadInvoice(ctx, id)
	if err != nil {
		return err
	}
	if invoice.CurrentHolder != operator {
		return fmt.Errorf("only current holder can cancel this transfer")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	transfer.Status, transfer.UpdatedAt = "CANCELLED", now
	if err := putJSON(ctx, transferRequestKey(id), transfer); err != nil {
		return err
	}
	return s.createFlow(ctx, id, "TRANSFER_CANCELLED", transfer.From, transfer.To, invoice.HolderMSPID, now)
}

func (s *InvoiceContract) ReadInvoiceTransfer(ctx contractapi.TransactionContextInterface, id string) (*InvoiceTransfer, error) {
	data, err := ctx.GetStub().GetState(transferRequestKey(strings.TrimSpace(id)))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, fmt.Errorf("invoice transfer %s does not exist", id)
	}
	var transfer InvoiceTransfer
	if err := json.Unmarshal(data, &transfer); err != nil {
		return nil, err
	}
	return &transfer, nil
}

func isMissingTransfer(err error) bool {
	return err != nil && strings.Contains(err.Error(), "does not exist")
}

// VoidInvoice is an append-only cancellation action. It deliberately keeps the
// original invoice and its history instead of deleting on-chain evidence.
func (s *InvoiceContract) VoidInvoice(ctx contractapi.TransactionContextInterface, id, reason, operator string) error {
	invoice, err := s.ReadInvoice(ctx, id)
	if err != nil {
		return err
	}
	reason, operator = strings.TrimSpace(reason), strings.TrimSpace(operator)
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
	if err := s.requireActiveBusinessUser(ctx, operator, callerMSP, "ISSUER"); err != nil {
		return fmt.Errorf("void operator: %w", err)
	}
	operatorOrganizationID, err := s.businessUserOrganization(ctx, operator)
	if err != nil {
		return err
	}
	if invoice.IssuerOrganizationID != "" && operatorOrganizationID != "" && operatorOrganizationID != invoice.IssuerOrganizationID {
		return fmt.Errorf("only the invoice issuer business organization can void this invoice")
	}
	if err := s.ensureInvoiceCanBeVoided(ctx, id); err != nil {
		return err
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
	if err := s.ensureInvoiceCanBeVoided(ctx, id); err != nil {
		return err
	}
	if invoice.ProjectID == "" {
		return fmt.Errorf("only an invoice linked to a project can use the void-request process")
	}
	project, err := s.ReadProject(ctx, invoice.ProjectID)
	if err != nil {
		return fmt.Errorf("read linked project: %w", err)
	}
	if err := s.requireProjectMember(ctx, project, applicant); err != nil {
		return fmt.Errorf("void request applicant: %w", err)
	}
	callerMSP, err := callerMSPID(ctx)
	if err != nil {
		return err
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
	if err := s.requireActiveBusinessUser(ctx, reviewer, callerMSP, "ISSUER"); err != nil {
		return fmt.Errorf("void reviewer: %w", err)
	}
	reviewerOrganizationID, err := s.businessUserOrganization(ctx, reviewer)
	if err != nil {
		return err
	}
	if invoice.IssuerOrganizationID != "" && reviewerOrganizationID != "" && reviewerOrganizationID != invoice.IssuerOrganizationID {
		return fmt.Errorf("void reviewer must belong to issuer business organization")
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
	if err := s.ensureInvoiceCanBeVoided(ctx, id); err != nil {
		return err
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
	if role != "PROJECT_MEMBER" {
		return fmt.Errorf("public business-user registration may only create PROJECT_MEMBER; privileged roles require administrator provisioning")
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
	mspID, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if err := s.requireActiveBusinessUser(ctx, creator, mspID, "ORG_ADMIN"); err != nil {
		return fmt.Errorf("organization creator: %w", err)
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
func (s *InvoiceContract) CreateProject(ctx contractapi.TransactionContextInterface, id, name, content, budgetCentsText, expectedEndDate, applicant, organizationID string) error {
	id, name, content = strings.TrimSpace(id), strings.TrimSpace(name), strings.TrimSpace(content)
	expectedEndDate, applicant, organizationID = strings.TrimSpace(expectedEndDate), strings.TrimSpace(applicant), strings.TrimSpace(organizationID)
	if id == "" || name == "" || content == "" || expectedEndDate == "" || applicant == "" || organizationID == "" {
		return fmt.Errorf("project fields must not be empty")
	}
	if !validISODate(expectedEndDate) {
		return fmt.Errorf("expected end date must be a valid YYYY-MM-DD date")
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
	if err := s.requireActiveBusinessUser(ctx, applicant, mspID, "PROJECT_MEMBER", "ISSUER"); err != nil {
		return fmt.Errorf("project applicant: %w", err)
	}
	applicantUser, err := s.ReadBusinessUser(ctx, applicant)
	if err != nil {
		return err
	}
	if applicantUser.OrganizationID != organizationID {
		return fmt.Errorf("project applicant does not belong to organization %s", organizationID)
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project := Project{ID: id, Name: name, Content: content, BudgetCents: budgetCents, ExpectedEndDate: expectedEndDate, Applicant: applicant, ApplicantMSPID: mspID, OrganizationID: organizationID, Status: "DRAFT", CreatedAt: now, UpdatedAt: now}
	if err := putJSON(ctx, projectKey(id), project); err != nil {
		return err
	}
	if err := putJSON(ctx, projectMemberKey(id, applicant), ProjectMember{ProjectID: id, Username: applicant, Role: "LEADER", AddedAt: now, AddedBy: applicant}); err != nil {
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
	if !validISODate(expectedEndDate) {
		return fmt.Errorf("expected end date must be a valid YYYY-MM-DD date")
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

// ReviewProject approves the work plan only. Funding is intentionally not
// released here: acceptance and financial settlement are separate stages.
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
	if err := s.requireProjectManager(ctx, project, reviewer, "PROJECT_REVIEWER"); err != nil {
		return err
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project.Reviewer, project.ReviewOpinion, project.UpdatedAt = reviewer, opinion, now
	if decision == "APPROVE" {
		project.Status = "EXECUTING"
	} else {
		project.Status = "REVISION_REQUIRED"
	}
	if err := putJSON(ctx, projectKey(id), project); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, id, "PROJECT_"+decision, reviewer, opinion)
}

func (s *InvoiceContract) RequestProjectClosure(ctx contractapi.TransactionContextInterface, id, materials, materialsHash, applicant string) error {
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
	materials, materialsHash = strings.TrimSpace(materials), strings.ToLower(strings.TrimSpace(materialsHash))
	if materials == "" || len(materials) > 4000 {
		return fmt.Errorf("closure materials must be 1-4000 characters")
	}
	if len(materialsHash) != 64 {
		return fmt.Errorf("closure materials hash must be a 64-character SHA-256 hex string")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project.Status, project.ClosureMaterials, project.ClosureMaterialsHash, project.UpdatedAt = "CLOSURE_REVIEW", materials, materialsHash, now
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
	if err := s.requireProjectManager(ctx, project, reviewer, "PROJECT_REVIEWER"); err != nil {
		return err
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project.Reviewer, project.ReviewOpinion, project.UpdatedAt = reviewer, opinion, now
	if decision == "APPROVE" {
		project.Status = "FINANCIAL_SETTLEMENT"
		project.AvailableCents = project.BudgetCents
	} else {
		project.Status = "EXECUTING"
	}
	if err := putJSON(ctx, projectKey(id), project); err != nil {
		return err
	}
	if decision == "APPROVE" {
		return s.createProjectEvent(ctx, id, "CLOSURE_APPROVE_AND_RELEASE_FUND", reviewer, opinion)
	}
	return s.createProjectEvent(ctx, id, "CLOSURE_REVISION", reviewer, opinion)
}

// FinalizeProjectSettlement archives a project after all approved payments
// have been completed. Unused available money is recorded as recovered, not
// silently discarded; archived projects can no longer accept invoices or claims.
func (s *InvoiceContract) FinalizeProjectSettlement(ctx contractapi.TransactionContextInterface, id, financeOperator string) error {
	project, err := s.ReadProject(ctx, id)
	if err != nil {
		return err
	}
	if project.Status != "FINANCIAL_SETTLEMENT" && project.Status != "CLOSURE_APPROVED" {
		return fmt.Errorf("project %s is not in financial settlement", id)
	}
	if err := s.requireProjectManager(ctx, project, strings.TrimSpace(financeOperator), "FINANCE_ADMIN"); err != nil {
		return err
	}
	if project.ReservedCents != 0 {
		return fmt.Errorf("project still has %d cents frozen; complete payment or withdraw the reimbursement first", project.ReservedCents)
	}
	reimbursements, err := s.GetAllReimbursements(ctx)
	if err != nil {
		return err
	}
	for _, reimbursement := range reimbursements {
		if reimbursement.ProjectID == project.ID && (reimbursement.Status == "PENDING_REVIEW" || reimbursement.Status == "REVISION_REQUIRED") {
			return fmt.Errorf("project still has reimbursement %s awaiting action", reimbursement.ID)
		}
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	project.RecoveredCents += project.AvailableCents
	project.AvailableCents, project.Status, project.UpdatedAt = 0, "ARCHIVED", now
	if err := putJSON(ctx, projectKey(project.ID), project); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, project.ID, "FINALIZE_SETTLEMENT", financeOperator, "完成财务结算，回收剩余额度")
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

func (s *InvoiceContract) GetProjectMembers(ctx contractapi.TransactionContextInterface, projectID string) ([]*ProjectMember, error) {
	prefix := projectMemberPrefix + strings.TrimSpace(projectID) + "#"
	iterator, err := ctx.GetStub().GetStateByRange(prefix, prefix+"\uffff")
	if err != nil {
		return nil, err
	}
	defer iterator.Close()
	members := make([]*ProjectMember, 0)
	for iterator.HasNext() {
		item, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		var member ProjectMember
		if err := json.Unmarshal(item.Value, &member); err != nil {
			return nil, err
		}
		members = append(members, &member)
	}
	return members, nil
}

func (s *InvoiceContract) AddProjectMember(ctx contractapi.TransactionContextInterface, projectID, username, leader string) error {
	project, err := s.ReadProject(ctx, projectID)
	if err != nil {
		return err
	}
	if project.Status == "ARCHIVED" {
		return fmt.Errorf("archived project cannot add members")
	}
	if err := s.requireProjectApplicant(ctx, project, leader); err != nil {
		return err
	}
	username = strings.TrimSpace(username)
	member, err := s.ReadBusinessUser(ctx, username)
	if err != nil {
		return err
	}
	if member.Status != "ACTIVE" || (member.Role != "PROJECT_MEMBER" && member.Role != "ISSUER") {
		return fmt.Errorf("user %s cannot be a project member", username)
	}
	if member.MSPID != project.ApplicantMSPID || (project.OrganizationID != "" && member.OrganizationID != project.OrganizationID) {
		return fmt.Errorf("project member must belong to the same project organization")
	}
	existing, err := ctx.GetStub().GetState(projectMemberKey(projectID, username))
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("user %s is already a project member", username)
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	if err := putJSON(ctx, projectMemberKey(projectID, username), ProjectMember{ProjectID: projectID, Username: username, Role: "MEMBER", AddedAt: now, AddedBy: leader}); err != nil {
		return err
	}
	return s.createProjectEvent(ctx, projectID, "ADD_MEMBER", leader, username)
}

func (s *InvoiceContract) requireProjectMember(ctx contractapi.TransactionContextInterface, project *Project, username string) error {
	if err := s.requireProjectApplicant(ctx, project, username); err == nil {
		return nil
	}
	caller, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if caller != project.ApplicantMSPID {
		return fmt.Errorf("only project organization %s can perform this action", project.ApplicantMSPID)
	}
	if err := s.requireActiveBusinessUser(ctx, username, caller, "PROJECT_MEMBER", "ISSUER"); err != nil {
		return err
	}
	data, err := ctx.GetStub().GetState(projectMemberKey(project.ID, strings.TrimSpace(username)))
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("user %s is not on project %s member list", username, project.ID)
	}
	return nil
}

func (s *InvoiceContract) CreateReimbursement(ctx contractapi.TransactionContextInterface, id, projectID, invoiceID, applicant, evidence, evidenceHash string) error {
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
	if project.Status != "FINANCIAL_SETTLEMENT" && project.Status != "CLOSURE_APPROVED" {
		return fmt.Errorf("project %s is not open for reimbursement", projectID)
	}
	if err := s.requireProjectMember(ctx, project, applicant); err != nil {
		return err
	}
	invoice, err := s.ReadInvoice(ctx, invoiceID)
	if err != nil {
		return err
	}
	if invoice.Status == "VOIDED" {
		return fmt.Errorf("voided invoice %s cannot be reimbursed", invoiceID)
	}
	if invoice.Currency != "" && invoice.Currency != "CNY" {
		return fmt.Errorf("only CNY invoices can be reimbursed")
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
	evidence, evidenceHash = strings.TrimSpace(evidence), strings.ToLower(strings.TrimSpace(evidenceHash))
	if evidence == "" || len(evidence) > 4000 {
		return fmt.Errorf("reimbursement evidence must be 1-4000 characters")
	}
	if len(evidenceHash) != 64 {
		return fmt.Errorf("reimbursement evidence hash must be a 64-character SHA-256 hex string")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	if invoice.TotalCents <= 0 || invoice.TotalCents > maxMoneyCents {
		return fmt.Errorf("invoice amount is invalid for reimbursement")
	}
	reimbursement := Reimbursement{ID: id, ProjectID: projectID, InvoiceID: invoiceID, Applicant: applicant, AmountCents: invoice.TotalCents, Evidence: evidence, EvidenceHash: evidenceHash, Status: "PENDING_REVIEW", CreatedAt: now, UpdatedAt: now}
	if err := putJSON(ctx, reimbursementKey(id), reimbursement); err != nil {
		return err
	}
	if err := ctx.GetStub().PutState(reimbursementInvoiceKey(invoiceID), []byte(id)); err != nil {
		return err
	}
	return s.createProjectEventWithReference(ctx, projectID, "SUBMIT_REIMBURSEMENT", applicant, "提交报销申请", id)
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
	if err := s.requireProjectManager(ctx, project, reviewer, "PROJECT_REVIEWER"); err != nil {
		return err
	}
	invoice, err := s.ReadInvoice(ctx, reimbursement.InvoiceID)
	if err != nil {
		return err
	}
	if invoice.Status == "VOIDED" {
		return fmt.Errorf("voided invoice %s cannot be approved for reimbursement", invoice.ID)
	}
	if reimbursement.AmountCents <= 0 || reimbursement.AmountCents > maxMoneyCents {
		return fmt.Errorf("reimbursement amount is invalid")
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
	return s.createProjectEventWithReference(ctx, reimbursement.ProjectID, "REIMBURSEMENT_"+decision, reviewer, opinion, reimbursement.ID)
}

func (s *InvoiceContract) ResubmitReimbursement(ctx contractapi.TransactionContextInterface, id, evidence, evidenceHash, applicant string) error {
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
	if err := s.requireProjectMember(ctx, project, applicant); err != nil {
		return err
	}
	evidence, evidenceHash = strings.TrimSpace(evidence), strings.ToLower(strings.TrimSpace(evidenceHash))
	if evidence == "" || len(evidence) > 4000 {
		return fmt.Errorf("reimbursement evidence must be 1-4000 characters")
	}
	if len(evidenceHash) != 64 {
		return fmt.Errorf("reimbursement evidence hash must be a 64-character SHA-256 hex string")
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	reimbursement.Evidence, reimbursement.EvidenceHash, reimbursement.Status, reimbursement.UpdatedAt = evidence, evidenceHash, "PENDING_REVIEW", now
	reimbursement.Reviewer, reimbursement.ReviewOpinion = "", ""
	if err := putJSON(ctx, reimbursementKey(id), reimbursement); err != nil {
		return err
	}
	return s.createProjectEventWithReference(ctx, reimbursement.ProjectID, "RESUBMIT_REIMBURSEMENT", applicant, "重新提交报销申请", id)
}

// WithdrawReimbursement is the safe escape hatch before payment. It keeps the
// reimbursement record on-chain, releases a frozen amount when necessary, and
// removes only the active invoice-link guard so the invoice can be corrected.
func (s *InvoiceContract) WithdrawReimbursement(ctx contractapi.TransactionContextInterface, id, applicant string) error {
	reimbursement, err := s.ReadReimbursement(ctx, id)
	if err != nil {
		return err
	}
	if reimbursement.Status != "PENDING_REVIEW" && reimbursement.Status != "REVISION_REQUIRED" && reimbursement.Status != "APPROVED_RESERVED" {
		return fmt.Errorf("reimbursement %s cannot be withdrawn in status %s", id, reimbursement.Status)
	}
	project, err := s.ReadProject(ctx, reimbursement.ProjectID)
	if err != nil {
		return err
	}
	if err := s.requireProjectMember(ctx, project, applicant); err != nil {
		return err
	}
	now, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	if reimbursement.Status == "APPROVED_RESERVED" {
		if project.ReservedCents < reimbursement.AmountCents {
			return fmt.Errorf("project reserved balance is inconsistent")
		}
		available, err := checkedAddMoney(project.AvailableCents, reimbursement.AmountCents)
		if err != nil || available > project.BudgetCents-project.PaidCents {
			return fmt.Errorf("project balance is inconsistent and cannot release this reimbursement")
		}
		project.AvailableCents, project.ReservedCents, project.UpdatedAt = available, project.ReservedCents-reimbursement.AmountCents, now
		if err := putJSON(ctx, projectKey(project.ID), project); err != nil {
			return err
		}
	}
	reimbursement.Status, reimbursement.UpdatedAt = "WITHDRAWN", now
	if err := putJSON(ctx, reimbursementKey(id), reimbursement); err != nil {
		return err
	}
	if err := ctx.GetStub().DelState(reimbursementInvoiceKey(reimbursement.InvoiceID)); err != nil {
		return err
	}
	return s.createProjectEventWithReference(ctx, project.ID, "WITHDRAW_REIMBURSEMENT", applicant, "撤回报销申请", id)
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
	if err := s.requireProjectManager(ctx, project, financeOperator, "FINANCE_ADMIN"); err != nil {
		return err
	}
	invoice, err := s.ReadInvoice(ctx, reimbursement.InvoiceID)
	if err != nil {
		return err
	}
	if invoice.Status == "VOIDED" {
		return fmt.Errorf("voided invoice %s cannot be paid", invoice.ID)
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
	return s.createProjectEventWithReference(ctx, project.ID, "PAY_REIMBURSEMENT", financeOperator, "确认报销支付", id)
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
	if err := s.requireActiveBusinessUser(ctx, applicant, caller, "PROJECT_MEMBER", "ISSUER"); err != nil {
		return fmt.Errorf("project applicant: %w", err)
	}
	organizationID, err := s.businessUserOrganization(ctx, applicant)
	if err != nil {
		return err
	}
	if project.OrganizationID != "" && organizationID != "" && organizationID != project.OrganizationID {
		return fmt.Errorf("project applicant does not belong to project business organization")
	}
	return nil
}

// requireActiveBusinessUser makes the application-level identity explicit in
// chaincode. The course network still uses one representative certificate per
// MSP, so this complements (rather than replaces) the Fabric certificate check.
func (s *InvoiceContract) requireActiveBusinessUser(ctx contractapi.TransactionContextInterface, username, mspID string, roles ...string) error {
	caller, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if caller != mspID {
		return fmt.Errorf("certificate belongs to %s, not %s", caller, mspID)
	}
	user, err := s.ReadBusinessUser(ctx, username)
	if err != nil {
		if demoUserRole(username, mspID) != "" {
			for _, role := range roles {
				if demoUserRole(username, mspID) == role {
					return nil
				}
			}
		}
		return err
	}
	if user.Status != "ACTIVE" || user.MSPID != mspID {
		return fmt.Errorf("business user %s is not active in %s", username, mspID)
	}
	for _, role := range roles {
		if user.Role == role {
			return nil
		}
	}
	return fmt.Errorf("business user %s does not have the required role", username)
}

// requireRegisteredRole validates a proposed recipient without requiring the
// current caller's certificate to belong to that recipient's organization.
func (s *InvoiceContract) requireRegisteredRole(ctx contractapi.TransactionContextInterface, username, mspID string, roles ...string) error {
	user, err := s.ReadBusinessUser(ctx, username)
	if err != nil {
		if role := demoUserRole(username, mspID); role != "" {
			for _, allowed := range roles {
				if role == allowed {
					return nil
				}
			}
		}
		return err
	}
	if user.Status != "ACTIVE" || user.MSPID != mspID {
		return fmt.Errorf("business user %s is not active in %s", username, mspID)
	}
	for _, allowed := range roles {
		if user.Role == allowed {
			return nil
		}
	}
	return fmt.Errorf("business user %s does not have the required role", username)
}

// Built-in test accounts are intentionally a bootstrap exception: they keep a
// fresh course network usable before any business directory has been created.
// Newly registered users must always have a ledger record and are never covered
// by this fallback.
func demoUserRole(username, mspID string) string {
	roles := map[string]struct{ msp, role string }{
		"issuer-org1": {"Org1MSP", "ISSUER"}, "holder-org2": {"Org2MSP", "HOLDER"}, "auditor": {"Org1MSP", "AUDITOR"},
		"project-member": {"Org1MSP", "PROJECT_MEMBER"}, "project-reviewer": {"Org1MSP", "PROJECT_REVIEWER"},
		"finance-admin": {"Org2MSP", "FINANCE_ADMIN"}, "org-admin": {"Org1MSP", "ORG_ADMIN"},
		"org-admin-org2": {"Org2MSP", "ORG_ADMIN"},
	}
	if account, ok := roles[username]; ok && account.msp == mspID {
		return account.role
	}
	return ""
}

func (s *InvoiceContract) businessUserOrganization(ctx contractapi.TransactionContextInterface, username string) (string, error) {
	user, err := s.ReadBusinessUser(ctx, username)
	if err != nil {
		if demoUserRole(username, "Org1MSP") != "" || demoUserRole(username, "Org2MSP") != "" {
			return "", nil
		}
		return "", err
	}
	return user.OrganizationID, nil
}

// A primary organization may manage a project team directly under it; all
// other actions stay within the exact business organization.
func (s *InvoiceContract) requireProjectManager(ctx contractapi.TransactionContextInterface, project *Project, username, role string) error {
	caller, err := callerMSPID(ctx)
	if err != nil {
		return err
	}
	if err := s.requireActiveBusinessUser(ctx, username, caller, role); err != nil {
		return err
	}
	if project.OrganizationID == "" { // legacy record created before business-org scoping
		if caller != project.ApplicantMSPID {
			return fmt.Errorf("legacy project belongs to %s", project.ApplicantMSPID)
		}
		return nil
	}
	user, err := s.ReadBusinessUser(ctx, username)
	if err != nil {
		if demoUserRole(username, caller) != "" {
			return nil
		}
		return err
	}
	if user.OrganizationID == project.OrganizationID {
		return nil
	}
	projectOrg, err := s.ReadBusinessOrganization(ctx, project.OrganizationID)
	if err != nil {
		return err
	}
	if projectOrg.ParentID == user.OrganizationID {
		return nil
	}
	return fmt.Errorf("operator organization is outside this project's management scope")
}

func (s *InvoiceContract) ensureInvoiceCanBeVoided(ctx contractapi.TransactionContextInterface, invoiceID string) error {
	linked, err := ctx.GetStub().GetState(reimbursementInvoiceKey(invoiceID))
	if err != nil {
		return err
	}
	if linked == nil {
		return nil
	}
	reimbursement, err := s.ReadReimbursement(ctx, string(linked))
	if err != nil {
		return err
	}
	return fmt.Errorf("invoice %s is linked to reimbursement %s in status %s; resolve or withdraw the reimbursement before voiding", invoiceID, reimbursement.ID, reimbursement.Status)
}

func (s *InvoiceContract) createProjectEvent(ctx contractapi.TransactionContextInterface, projectID, eventType, actor, note string) error {
	return s.createProjectEventWithReference(ctx, projectID, eventType, actor, note, "")
}

func (s *InvoiceContract) createProjectEventWithReference(ctx contractapi.TransactionContextInterface, projectID, eventType, actor, note, referenceID string) error {
	timestamp, err := transactionTime(ctx)
	if err != nil {
		return err
	}
	txID := ctx.GetStub().GetTxID()
	event := ProjectEvent{ID: projectEventPrefix + projectID + "#" + txID, ProjectID: projectID, Type: eventType, Actor: actor, Note: note, ReferenceID: referenceID, TxID: txID, Timestamp: timestamp}
	return putJSON(ctx, event.ID, event)
}

// InvoiceNumberExists prevents the same business invoice number from being
// stored under different application IDs. It scans invoice state so that
// records created before this rule was introduced are covered as well.
func (s *InvoiceContract) InvoiceNumberExists(ctx contractapi.TransactionContextInterface, invoiceNo string) (bool, error) {
	indexed, err := ctx.GetStub().GetState(invoiceNumberKey(invoiceNo))
	if err != nil {
		return false, err
	}
	if indexed != nil {
		return true, nil
	}
	// Compatibility fallback for records written before the number index.
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

func (s *InvoiceContract) InvoiceNumberExistsExcept(ctx contractapi.TransactionContextInterface, invoiceNo, exceptID string) (bool, error) {
	indexed, err := ctx.GetStub().GetState(invoiceNumberKey(invoiceNo))
	if err != nil {
		return false, err
	}
	if indexed != nil && string(indexed) != exceptID {
		return true, nil
	}
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
		if invoice.ID != exceptID && invoice.InvoiceNo == invoiceNo {
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

// VerifyInvoice compares the server-calculated content hash with the hash stored
// on-chain. A voided invoice may still have matching original content, but is
// deliberately not reported as valid for current business use.
func (s *InvoiceContract) VerifyInvoice(ctx contractapi.TransactionContextInterface, id, presentedHash string) (*VerificationResult, error) {
	invoice, err := s.ReadInvoice(ctx, id)
	if err != nil {
		return nil, err
	}
	matched := strings.EqualFold(strings.TrimSpace(presentedHash), invoice.DataHash)
	result := &VerificationResult{Invoice: invoice, DataHashMatched: matched, Valid: matched && invoice.Status != "VOIDED"}
	if !matched {
		result.Reason = "content hash does not match the on-chain record"
	} else if invoice.Status == "VOIDED" {
		result.Reason = "content matches the original on-chain record, but this invoice has been voided"
	} else {
		result.Reason = "content hash matches the on-chain invoice record"
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
	if err != nil || result < 0 || result > maxMoneyCents {
		return 0, fmt.Errorf("%s must be a non-negative integer in cents", field)
	}
	return result, nil
}

func checkedAddMoney(left, right int64) (int64, error) {
	if left < 0 || right < 0 || left > math.MaxInt64-right || left+right > maxMoneyCents {
		return 0, fmt.Errorf("amount plus tax exceeds the allowed invoice limit")
	}
	return left + right, nil
}

func validISODate(value string) bool {
	_, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	return err == nil
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
