package api

import (
	"testing"

	"invoice-ledger-api/auth"
)

func TestMissingOrganizationDoesNotGrantProjectAccess(t *testing.T) {
	project := Project{ID: "p", OrganizationID: "missing-org"}
	principal := auth.Principal{Username: "unscoped", MSPID: "Org1MSP", Role: "PROJECT_REVIEWER"}
	if projectVisibleTo(principal, project, map[string]BusinessOrganization{}) {
		t.Fatal("empty parent ID must not grant access")
	}
	if hasBootstrapDirectoryAccess(principal) {
		t.Fatal("role alone must not grant bootstrap access")
	}
	principal.Username = "project-reviewer"
	principal.MSPID = "Org2MSP"
	if hasBootstrapDirectoryAccess(principal) {
		t.Fatal("bootstrap identity must match MSP")
	}
}

func TestBuiltInIssuerCanReadItsFabricOrganizationInvoicesOnly(t *testing.T) {
	issuer := auth.Principal{Username: "issuer-org1", MSPID: "Org1MSP", Role: "ISSUER"}
	org1Invoice := Invoice{IssuerMSPID: "Org1MSP", IssuerOrganizationID: "business-org-a"}
	org2Invoice := Invoice{IssuerMSPID: "Org2MSP", HolderMSPID: "Org2MSP", IssuerOrganizationID: "business-org-b"}
	if !invoiceVisibleTo(issuer, org1Invoice, nil, nil) {
		t.Fatal("built-in Org1 issuer must see invoices issued through Org1")
	}
	if invoiceVisibleTo(issuer, org2Invoice, nil, nil) {
		t.Fatal("built-in Org1 issuer must not see Org2-only invoices")
	}
	registeredIssuer := auth.Principal{Username: "alice", MSPID: "Org1MSP", OrganizationID: "business-org-c", Role: "ISSUER"}
	if invoiceVisibleTo(registeredIssuer, org1Invoice, nil, nil) {
		t.Fatal("registered issuer must stay scoped to its own business organization")
	}
}

func TestBuiltInRoleVisibilityScopes(t *testing.T) {
	org1Project := Project{ID: "p1", ApplicantMSPID: "Org1MSP", OrganizationID: "team-1"}
	org2Project := Project{ID: "p2", ApplicantMSPID: "Org2MSP", OrganizationID: "team-2"}
	org1Invoice := Invoice{IssuerMSPID: "Org1MSP", HolderMSPID: "Org1MSP", IssuerOrganizationID: "team-1"}
	transferredToOrg2 := Invoice{IssuerMSPID: "Org1MSP", HolderMSPID: "Org2MSP", IssuerOrganizationID: "team-1"}
	otherOrg2Invoice := Invoice{IssuerMSPID: "Org2MSP", HolderMSPID: "Org1MSP", IssuerOrganizationID: "team-2"}

	member := auth.Principal{Username: "project-member", MSPID: "Org1MSP", Role: "PROJECT_MEMBER"}
	if !projectVisibleTo(member, org1Project, nil) || projectVisibleTo(member, org2Project, nil) {
		t.Fatal("built-in project member must see Org1 projects only")
	}
	if !invoiceVisibleTo(member, org1Invoice, nil, nil) || invoiceVisibleTo(member, Invoice{IssuerMSPID: "Org2MSP", HolderMSPID: "Org2MSP"}, nil, nil) {
		t.Fatal("built-in project member must see Org1 invoices only")
	}

	holder := auth.Principal{Username: "holder-org2", MSPID: "Org2MSP", Role: "HOLDER"}
	if !invoiceVisibleTo(holder, transferredToOrg2, nil, nil) {
		t.Fatal("built-in holder must see invoices transferred to Org2")
	}
	if invoiceVisibleTo(holder, otherOrg2Invoice, nil, nil) || projectVisibleTo(holder, org2Project, nil) {
		t.Fatal("built-in holder must not receive Org2-wide project or invoice access")
	}

	for _, admin := range []auth.Principal{
		{Username: "auditor", MSPID: "Org1MSP", Role: "AUDITOR"},
		{Username: "project-reviewer", MSPID: "Org1MSP", Role: "PROJECT_REVIEWER"},
		{Username: "finance-admin", MSPID: "Org2MSP", Role: "FINANCE_ADMIN"},
		{Username: "org-admin", MSPID: "Org1MSP", Role: "ORG_ADMIN"},
		{Username: "org-admin-org2", MSPID: "Org2MSP", Role: "ORG_ADMIN"},
	} {
		if !projectVisibleTo(admin, org2Project, nil) || !invoiceVisibleTo(admin, otherOrg2Invoice, nil, nil) {
			t.Fatalf("%s must retain review visibility", admin.Username)
		}
	}
}

func TestProjectVisibleToBootstrapReviewer(t *testing.T) {
	project := Project{ID: "project-1", OrganizationID: "team-a", ApplicantMSPID: "Org1MSP"}
	invoice := Invoice{ID: "invoice-1", ProjectID: project.ID, IssuerOrganizationID: "team-a", HolderOrganizationID: "team-a"}
	organizations := map[string]BusinessOrganization{
		"team-a": {ID: "team-a", ParentID: "primary-a"},
	}

	if !projectVisibleTo(auth.Principal{Username: "project-reviewer", MSPID: "Org1MSP", Role: "PROJECT_REVIEWER"}, project, organizations) {
		t.Fatal("bootstrap project reviewer must see a pending project")
	}
	if !invoiceVisibleTo(auth.Principal{Username: "project-reviewer", MSPID: "Org1MSP", Role: "PROJECT_REVIEWER"}, invoice, map[string]Project{project.ID: project}, organizations) {
		t.Fatal("bootstrap project reviewer must see the invoice supporting a reimbursement")
	}
	if projectVisibleTo(auth.Principal{Username: "alice", MSPID: "Org1MSP", OrganizationID: "team-b", Role: "PROJECT_MEMBER"}, project, organizations) {
		t.Fatal("a registered member outside the project organization must remain scoped out")
	}
}
