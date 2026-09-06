package main

import (
	"log"
	"net/http"
	"os"

	"invoice-ledger-api/api"
	"invoice-ledger-api/auth"
	"invoice-ledger-api/fabric"
	"invoice-ledger-api/ocr"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := fabric.Init(); err != nil {
		log.Fatalf("connect to Fabric failed: %v", err)
	}
	defer fabric.Close()

	router := gin.Default()
	authService, err := auth.NewService()
	if err != nil {
		log.Fatalf("local account store check failed: %v", err)
	}
	router.GET("/health", func(c *gin.Context) {
		contract, err := fabric.ContractFor("Org1MSP")
		if err == nil {
			_, err = contract.EvaluateTransaction("GetAllInvoices")
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "backend": "ok", "fabric": "unavailable", "chaincode": fabric.ChaincodeName(), "error": "Fabric 网络或链码当前不可访问"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "backend": "ok", "fabric": "reachable", "chaincode": fabric.ChaincodeName()})
	})
	api.RegisterRoutes(router, authService, ocr.NewService())

	webDir := os.Getenv("WEB_DIR")
	if webDir == "" {
		webDir = "../web"
	}
	router.Static("/assets", webDir)
	router.GET("/", func(c *gin.Context) { c.File(webDir + "/index.html") })

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("链证发票服务已启动：http://localhost:%s", port)
	if err := router.Run("0.0.0.0:" + port); err != nil {
		log.Fatal(err)
	}
}
