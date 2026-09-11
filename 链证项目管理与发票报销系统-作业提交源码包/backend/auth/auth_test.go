package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAccountRecoveryMissingPrimaryAndPreservesValidBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	user, err := newAccount(Principal{Username: "alice", MSPID: "Org1MSP", Role: "PROJECT_MEMBER"}, "member123")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal([]account{user})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", data, 0600); err != nil {
		t.Fatal(err)
	}
	s := &Service{accounts: make(map[string]account), accountFile: path}
	if err := s.loadAccounts(); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.accounts["alice"]; !ok {
		t.Fatal("missing primary must recover backup")
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.saveRegisteredAccountsLocked(); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeAccounts(backup); err != nil {
		t.Fatalf("backup poisoned by corrupt primary: %v", err)
	}
}

func TestRejectInvalidAccountStore(t *testing.T) {
	for _, value := range []string{"null", "{}", "[{\"Username\":\"alice\"}]"} {
		if _, err := decodeAccounts([]byte(value)); err == nil {
			t.Fatalf("accepted invalid store: %s", value)
		}
	}
}

func TestAccountStoreRecoversFromLastAtomicBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	first, err := newAccount(Principal{Username: "alice", DisplayName: "Alice", MSPID: "Org1MSP", OrganizationID: "org-a", Role: "PROJECT_MEMBER"}, "member123")
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{accounts: map[string]account{"alice": first}, accountFile: path}
	if err := service.saveRegisteredAccountsLocked(); err != nil {
		t.Fatal(err)
	}
	second, err := newAccount(Principal{Username: "bob", DisplayName: "Bob", MSPID: "Org1MSP", OrganizationID: "org-a", Role: "PROJECT_MEMBER"}, "member123")
	if err != nil {
		t.Fatal(err)
	}
	service.accounts["bob"] = second
	if err := service.saveRegisteredAccountsLocked(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not valid json"), 0600); err != nil {
		t.Fatal(err)
	}
	recovered := &Service{accounts: make(map[string]account), accountFile: path}
	if err := recovered.loadAccounts(); err != nil {
		t.Fatalf("expected backup recovery, got %v", err)
	}
	if _, ok := recovered.accounts["alice"]; !ok {
		t.Fatal("last complete account generation was not recovered")
	}
}
