package main

import (
	"log"
	"net/http"
	"os"

	"invoice-ledger-api/api"
	"invoice-ledger-api/auth"
	"invoice-ledger-api/fabric"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := fabric.Init(); err != nil {
		log.Fatalf("connect to Fabric failed: %v", err)
	}
	defer fabric.Close()

	router := gin.Default()
	authService := auth.NewService()
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "chaincode": fabric.ChaincodeName()})
	})
	api.RegisterRoutes(router, authService)

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
