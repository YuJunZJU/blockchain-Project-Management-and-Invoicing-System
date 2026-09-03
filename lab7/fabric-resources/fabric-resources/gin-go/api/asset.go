package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"gin-app/fabric"

	"github.com/gin-gonic/gin"
)

func RegisterAssetRoutes(r *gin.Engine) {
	r.POST("/asset", CreateAsset)
	r.GET("/asset/:id", ReadAsset)
	r.GET("/assets", GetAllAssets)
}

type Asset struct {
	ID             string `json:"ID"`
	Color          string `json:"Color"`
	Size           int    `json:"Size"`
	Owner          string `json:"Owner"`
	AppraisedValue int    `json:"AppraisedValue"`
}

func CreateAsset(c *gin.Context) {
	var req Asset
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := fabric.Contract.SubmitTransaction("CreateAsset",
		req.ID, req.Color,
		strconv.Itoa(req.Size),
		req.Owner,
		strconv.Itoa(req.AppraisedValue),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create asset: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Asset created successfully"})
}

func ReadAsset(c *gin.Context) {
	id := c.Param("id")

	result, err := fabric.Contract.EvaluateTransaction("ReadAsset", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read asset: " + err.Error()})
		return
	}

	var asset Asset
	json.Unmarshal(result, &asset)

	c.JSON(http.StatusOK, asset)
}

func GetAllAssets(c *gin.Context) {
	result, err := fabric.Contract.EvaluateTransaction("GetAllAssets")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get all assets: " + err.Error()})
		return
	}

	var assets []Asset
	json.Unmarshal(result, &assets)

	c.JSON(http.StatusOK, assets)
}
