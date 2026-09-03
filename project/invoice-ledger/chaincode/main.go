package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

func main() {
	chaincode, err := contractapi.NewChaincode(&InvoiceContract{})
	if err != nil {
		log.Panicf("create invoice chaincode: %v", err)
	}
	if err := chaincode.Start(); err != nil {
		log.Panicf("start invoice chaincode: %v", err)
	}
}
