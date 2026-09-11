package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/hyperledger/fabric-chaincode-go/v2/pkg/cid"
	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/queryresult"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// memoryStub deliberately has no connection to a Fabric network. It exercises
// the contract's business guards in memory so regressions are caught locally.
type memoryStub struct {
	shim.ChaincodeStubInterface
	state map[string][]byte
	seq   int
}

func (s *memoryStub) GetState(key string) ([]byte, error) { return s.state[key], nil }
func (s *memoryStub) PutState(key string, value []byte) error {
	s.state[key] = append([]byte(nil), value...)
	return nil
}
func (s *memoryStub) DelState(key string) error { delete(s.state, key); return nil }
func (s *memoryStub) GetTxID() string           { s.seq++; return fmt.Sprintf("test-%d", s.seq) }
func (s *memoryStub) GetTxTimestamp() (*timestamppb.Timestamp, error) {
	return &timestamppb.Timestamp{Seconds: 1_788_595_200}, nil
}

type memoryIterator struct {
	shim.StateQueryIteratorInterface
	values []*queryresult.KV
	index  int
}

func (i *memoryIterator) HasNext() bool { return i.index < len(i.values) }
func (i *memoryIterator) Next() (*queryresult.KV, error) {
	value := i.values[i.index]
	i.index++
	return value, nil
}
func (i *memoryIterator) Close() error { return nil }
func (s *memoryStub) GetStateByRange(start, end string) (shim.StateQueryIteratorInterface, error) {
	keys := make([]string, 0)
	for key := range s.state {
		if key >= start && key < end {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	iterator := &memoryIterator{}
	for _, key := range keys {
		iterator.values = append(iterator.values, &queryresult.KV{Key: key, Value: s.state[key]})
	}
	return iterator, nil
}

type memoryIdentity struct {
	cid.ClientIdentity
	msp string
}

func (i memoryIdentity) GetMSPID() (string, error) { return i.msp, nil }

func testContract() (*InvoiceContract, *contractapi.TransactionContext, *memoryStub) {
	stub := &memoryStub{state: map[string][]byte{}}
	context := new(contractapi.TransactionContext)
	context.SetStub(stub)
	context.SetClientIdentity(memoryIdentity{msp: "Org1MSP"})
	return new(InvoiceContract), context, stub
}

func putTestState(t *testing.T, stub *memoryStub, key string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	stub.state[key] = encoded
}

func activeUser(username, role, organizationID string) BusinessUser {
	return BusinessUser{Username: username, Role: role, MSPID: "Org1MSP", OrganizationID: organizationID, Status: "ACTIVE"}
}

func TestVoidInvoiceRejectsLinkedReimbursement(t *testing.T) {
	contract, context, stub := testContract()
	putTestState(t, stub, userKey("issuer"), activeUser("issuer", "ISSUER", "org-a"))
	putTestState(t, stub, invoiceKey("invoice-1"), Invoice{ID: "invoice-1", Status: "ISSUED", IssuerMSPID: "Org1MSP", IssuerOrganizationID: "org-a", CurrentHolder: "issuer"})
	putTestState(t, stub, reimbursementKey("reim-1"), Reimbursement{ID: "reim-1", Status: "PENDING_REVIEW"})
	stub.state[reimbursementInvoiceKey("invoice-1")] = []byte("reim-1")
	if err := contract.VoidInvoice(context, "invoice-1", "信息录入错误", "issuer"); err == nil {
		t.Fatal("voiding an invoice linked to reimbursement must be rejected")
	}
}

func TestUnlinkInvoiceFromProjectBeforeReimbursement(t *testing.T) {
	contract, context, stub := testContract()
	putTestState(t, stub, userKey("alice"), activeUser("alice", "PROJECT_MEMBER", "org-a"))
	putTestState(t, stub, projectKey("project-1"), Project{ID: "project-1", Applicant: "alice", ApplicantMSPID: "Org1MSP", OrganizationID: "org-a", Status: "FINANCIAL_SETTLEMENT"})
	putTestState(t, stub, projectMemberKey("project-1", "alice"), ProjectMember{ProjectID: "project-1", Username: "alice", Role: "LEADER"})
	putTestState(t, stub, invoiceKey("invoice-1"), Invoice{ID: "invoice-1", Status: "ISSUED", IssuerMSPID: "Org1MSP", IssuerOrganizationID: "org-a", ProjectID: "project-1"})

	if err := contract.UnlinkInvoiceFromProject(context, "invoice-1", "alice"); err != nil {
		t.Fatal(err)
	}
	invoice, err := contract.ReadInvoice(context, "invoice-1")
	if err != nil {
		t.Fatal(err)
	}
	if invoice.ProjectID != "" {
		t.Fatalf("project association was not removed: %#v", invoice)
	}
}

func TestUnlinkInvoiceRejectsReimbursementActivity(t *testing.T) {
	contract, context, stub := testContract()
	putTestState(t, stub, userKey("alice"), activeUser("alice", "PROJECT_MEMBER", "org-a"))
	putTestState(t, stub, projectKey("project-1"), Project{ID: "project-1", Applicant: "alice", ApplicantMSPID: "Org1MSP", OrganizationID: "org-a", Status: "FINANCIAL_SETTLEMENT"})
	putTestState(t, stub, projectMemberKey("project-1", "alice"), ProjectMember{ProjectID: "project-1", Username: "alice", Role: "LEADER"})
	putTestState(t, stub, invoiceKey("invoice-1"), Invoice{ID: "invoice-1", Status: "ISSUED", IssuerMSPID: "Org1MSP", IssuerOrganizationID: "org-a", ProjectID: "project-1"})
	stub.state[reimbursementInvoiceKey("invoice-1")] = []byte("reim-1")

	if err := contract.UnlinkInvoiceFromProject(context, "invoice-1", "alice"); err == nil {
		t.Fatal("invoice with reimbursement activity must remain linked to its project")
	}
}

func TestResubmitReimbursementPreservesNewEvidence(t *testing.T) {
	contract, context, stub := testContract()
	putTestState(t, stub, userKey("alice"), activeUser("alice", "PROJECT_MEMBER", "org-a"))
	putTestState(t, stub, projectKey("project-1"), Project{ID: "project-1", Applicant: "alice", ApplicantMSPID: "Org1MSP", OrganizationID: "org-a", Status: "EXECUTING"})
	putTestState(t, stub, reimbursementKey("reim-1"), Reimbursement{ID: "reim-1", ProjectID: "project-1", Status: "REVISION_REQUIRED"})
	evidence := "已补充支付截图和采购说明"
	if err := contract.ResubmitReimbursement(context, "reim-1", evidence, strings.Repeat("a", 64), "alice"); err != nil {
		t.Fatal(err)
	}
	reimbursement, err := contract.ReadReimbursement(context, "reim-1")
	if err != nil {
		t.Fatal(err)
	}
	if reimbursement.Status != "PENDING_REVIEW" || reimbursement.Evidence != evidence {
		t.Fatalf("unexpected reimbursement: %#v", reimbursement)
	}
}

func TestReviewProjectRejectsOutOfScopeReviewer(t *testing.T) {
	contract, context, stub := testContract()
	putTestState(t, stub, userKey("reviewer"), activeUser("reviewer", "PROJECT_REVIEWER", "unrelated-primary"))
	putTestState(t, stub, organizationKey("team"), BusinessOrganization{ID: "team", ParentID: "primary", MSPID: "Org1MSP", Status: "ACTIVE", Type: "PROJECT_TEAM"})
	putTestState(t, stub, projectKey("project-1"), Project{ID: "project-1", OrganizationID: "team", ApplicantMSPID: "Org1MSP", Status: "PENDING_REVIEW", BudgetCents: 100})
	if err := contract.ReviewProject(context, "project-1", "APPROVE", "同意", "reviewer"); err == nil {
		t.Fatal("reviewer outside project organization hierarchy must be rejected")
	}
}

func TestMoneyAndDateBounds(t *testing.T) {
	if _, err := checkedAddMoney(maxMoneyCents, 1); err == nil {
		t.Fatal("overflowing configured money limit must fail")
	}
	if validISODate("2026-02-31") {
		t.Fatal("invalid calendar date must fail")
	}
	if !validISODate("2026-09-06") {
		t.Fatal("valid ISO date rejected")
	}
}

func TestPublicBusinessRegistrationCannotCreateAdministrator(t *testing.T) {
	contract, context, _ := testContract()
	if err := contract.RegisterBusinessUser(context, "new-admin", "越权测试", "Org1MSP", "ORG_ADMIN", "any-org"); err == nil {
		t.Fatal("public registration must not create an organization administrator")
	}
}

func TestWithdrawReservedReimbursementReleasesBudget(t *testing.T) {
	contract, context, stub := testContract()
	putTestState(t, stub, userKey("alice"), activeUser("alice", "PROJECT_MEMBER", "org-a"))
	putTestState(t, stub, projectKey("project-1"), Project{ID: "project-1", Applicant: "alice", ApplicantMSPID: "Org1MSP", OrganizationID: "org-a", BudgetCents: 100, AvailableCents: 50, ReservedCents: 50, Status: "EXECUTING"})
	putTestState(t, stub, reimbursementKey("reim-1"), Reimbursement{ID: "reim-1", ProjectID: "project-1", InvoiceID: "invoice-1", Applicant: "alice", AmountCents: 50, Status: "APPROVED_RESERVED"})
	stub.state[reimbursementInvoiceKey("invoice-1")] = []byte("reim-1")
	if err := contract.WithdrawReimbursement(context, "reim-1", "alice"); err != nil {
		t.Fatal(err)
	}
	project, err := contract.ReadProject(context, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	reimbursement, err := contract.ReadReimbursement(context, "reim-1")
	if err != nil {
		t.Fatal(err)
	}
	if project.AvailableCents != 100 || project.ReservedCents != 0 || reimbursement.Status != "WITHDRAWN" {
		t.Fatalf("unexpected state: project=%#v reimbursement=%#v", project, reimbursement)
	}
	if stub.state[reimbursementInvoiceKey("invoice-1")] != nil {
		t.Fatal("invoice reimbursement link must be cleared after withdrawal")
	}
}

func TestVerifyInvoiceMarksVoidedInvoiceAsNotCurrentlyValid(t *testing.T) {
	contract, context, stub := testContract()
	putTestState(t, stub, invoiceKey("invoice-voided"), Invoice{ID: "invoice-voided", DataHash: strings.Repeat("a", 64), Status: "VOIDED"})
	result, err := contract.VerifyInvoice(context, "invoice-voided", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.DataHashMatched || result.Valid {
		t.Fatalf("voided invoice must retain content-match evidence but not be currently valid: %#v", result)
	}
}

func TestProjectEventKeepsReimbursementReference(t *testing.T) {
	contract, context, stub := testContract()
	if err := contract.createProjectEventWithReference(context, "project-1", "PAY_REIMBURSEMENT", "finance", "确认报销支付", "reim-1"); err != nil {
		t.Fatal(err)
	}
	for key, value := range stub.state {
		if !strings.HasPrefix(key, projectEventPrefix+"project-1#") {
			continue
		}
		var event ProjectEvent
		if err := json.Unmarshal(value, &event); err != nil {
			t.Fatal(err)
		}
		if event.ReferenceID != "reim-1" || event.Note != "确认报销支付" {
			t.Fatalf("event reference was not preserved: %#v", event)
		}
		return
	}
	t.Fatal("project event was not written")
}

func TestFinalizeSettlementRecoversAvailableBudget(t *testing.T) {
	contract, context, stub := testContract()
	putTestState(t, stub, userKey("finance"), activeUser("finance", "FINANCE_ADMIN", "org-a"))
	putTestState(t, stub, projectKey("project-settle"), Project{ID: "project-settle", ApplicantMSPID: "Org1MSP", OrganizationID: "org-a", BudgetCents: 1000, AvailableCents: 350, ReservedCents: 0, PaidCents: 650, Status: "FINANCIAL_SETTLEMENT"})
	if err := contract.FinalizeProjectSettlement(context, "project-settle", "finance"); err != nil {
		t.Fatal(err)
	}
	project, err := contract.ReadProject(context, "project-settle")
	if err != nil {
		t.Fatal(err)
	}
	if project.Status != "ARCHIVED" || project.AvailableCents != 0 || project.RecoveredCents != 350 {
		t.Fatalf("unexpected finalized project: %#v", project)
	}
}
