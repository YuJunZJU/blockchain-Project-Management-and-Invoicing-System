package fabric

import (
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/hash"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type profile struct {
	MSPID        string
	OrgDomain    string
	PeerEndpoint string
	PeerName     string
	UserName     string
}

type gatewayClient struct {
	connection *grpc.ClientConn
	gateway    *client.Gateway
	contract   *client.Contract
}

var (
	chaincodeName string
	channelName   string
	testNetwork   string
	clients       = make(map[string]*gatewayClient)
	mu            sync.Mutex
)

// Init validates shared Fabric configuration. Individual Gateway connections
// are opened lazily according to the authenticated user's organization.
func Init() error {
	configuredNetwork := os.Getenv("FABRIC_TEST_NETWORK")
	if configuredNetwork == "" {
		return fmt.Errorf("FABRIC_TEST_NETWORK is required; point it to fabric-samples/test-network")
	}
	absNetwork, err := filepath.Abs(configuredNetwork)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absNetwork); err != nil {
		return fmt.Errorf("Fabric Test Network is unavailable at %s: %w", absNetwork, err)
	}
	testNetwork = absNetwork
	channelName = envOr("CHANNEL_NAME", "mychannel")
	chaincodeName = envOr("CHAINCODE_NAME", "invoice")
	// Fail early when the default issuer organization is unavailable.
	_, err = ContractFor("Org1MSP")
	return err
}

func ContractFor(mspID string) (*client.Contract, error) {
	mu.Lock()
	defer mu.Unlock()
	if existing, ok := clients[mspID]; ok {
		return existing.contract, nil
	}
	configuredProfile, err := profileFor(mspID)
	if err != nil {
		return nil, err
	}
	created, err := connect(configuredProfile)
	if err != nil {
		return nil, err
	}
	clients[mspID] = created
	return created.contract, nil
}

func Close() {
	mu.Lock()
	defer mu.Unlock()
	for _, current := range clients {
		current.gateway.Close()
		_ = current.connection.Close()
	}
	clients = make(map[string]*gatewayClient)
}

func ChaincodeName() string { return chaincodeName }

func profileFor(mspID string) (profile, error) {
	switch mspID {
	case "Org1MSP":
		return profile{MSPID: "Org1MSP", OrgDomain: "org1.example.com", PeerEndpoint: "dns:///localhost:7051", PeerName: "peer0.org1.example.com", UserName: "User1@org1.example.com"}, nil
	case "Org2MSP":
		return profile{MSPID: "Org2MSP", OrgDomain: "org2.example.com", PeerEndpoint: "dns:///localhost:9051", PeerName: "peer0.org2.example.com", UserName: "User1@org2.example.com"}, nil
	default:
		return profile{}, fmt.Errorf("unsupported Fabric organization %s", mspID)
	}
}

func connect(configuredProfile profile) (*gatewayClient, error) {
	connection, err := newConnection(configuredProfile)
	if err != nil {
		return nil, err
	}
	id, err := newIdentity(configuredProfile)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	sign, err := newSign(configuredProfile)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	gateway, err := client.Connect(
		id, client.WithSign(sign), client.WithHash(hash.SHA256), client.WithClientConnection(connection),
		client.WithEvaluateTimeout(10*time.Second), client.WithEndorseTimeout(15*time.Second),
		client.WithSubmitTimeout(30*time.Second), client.WithCommitStatusTimeout(time.Minute),
	)
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("create Fabric gateway for %s: %w", configuredProfile.MSPID, err)
	}
	return &gatewayClient{connection: connection, gateway: gateway, contract: gateway.GetNetwork(channelName).GetContract(chaincodeName)}, nil
}

func newConnection(configuredProfile profile) (*grpc.ClientConn, error) {
	tlsPath := filepath.Join(testNetwork, "organizations", "peerOrganizations", configuredProfile.OrgDomain, "peers", configuredProfile.PeerName, "tls", "ca.crt")
	pem, err := os.ReadFile(tlsPath)
	if err != nil {
		return nil, fmt.Errorf("read %s peer TLS certificate: %w", configuredProfile.MSPID, err)
	}
	certificate, err := identity.CertificateFromPEM(pem)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(certificate)
	connection, err := grpc.NewClient(configuredProfile.PeerEndpoint, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(pool, configuredProfile.PeerName)))
	if err != nil {
		return nil, fmt.Errorf("create %s gRPC connection: %w", configuredProfile.MSPID, err)
	}
	return connection, nil
}

func newIdentity(configuredProfile profile) (*identity.X509Identity, error) {
	certDir := filepath.Join(testNetwork, "organizations", "peerOrganizations", configuredProfile.OrgDomain, "users", configuredProfile.UserName, "msp", "signcerts")
	pem, err := readFirstFile(certDir)
	if err != nil {
		return nil, fmt.Errorf("read %s client certificate: %w", configuredProfile.MSPID, err)
	}
	certificate, err := identity.CertificateFromPEM(pem)
	if err != nil {
		return nil, err
	}
	return identity.NewX509Identity(configuredProfile.MSPID, certificate)
}

func newSign(configuredProfile profile) (identity.Sign, error) {
	keyDir := filepath.Join(testNetwork, "organizations", "peerOrganizations", configuredProfile.OrgDomain, "users", configuredProfile.UserName, "msp", "keystore")
	pem, err := readFirstFile(keyDir)
	if err != nil {
		return nil, fmt.Errorf("read %s private key: %w", configuredProfile.MSPID, err)
	}
	privateKey, err := identity.PrivateKeyFromPEM(pem)
	if err != nil {
		return nil, err
	}
	return identity.NewPrivateKeySign(privateKey)
}

func readFirstFile(directory string) ([]byte, error) {
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) == 0 {
		if err == nil {
			err = fmt.Errorf("directory is empty")
		}
		return nil, err
	}
	return os.ReadFile(filepath.Join(directory, entries[0].Name()))
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
