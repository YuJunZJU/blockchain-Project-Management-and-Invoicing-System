// Package ocr integrates Alibaba Cloud invoice OCR and an optional
// OpenAI-compatible model for transparent correction suggestions.
package ocr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ocrclient "github.com/alibabacloud-go/ocr-api-20210707/v3/client"
	"github.com/alibabacloud-go/tea/tea"
)

const maxFileSize = 10 << 20 // Alibaba Cloud's RecognizeAllText limit is 10 MB.

var allowedExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".bmp": true, ".gif": true,
	".tiff": true, ".webp": true, ".pdf": true,
}

// InvoiceFields contains only the fields the user may inspect and fill into
// the invoice form. The buyer is kept as buyerName: a chain transfer requires
// a registered business username and must never be guessed by an AI model.
type InvoiceFields struct {
	AmountCents int64  `json:"amountCents"`
	BuyerName   string `json:"buyerName"`
	InvoiceNo   string `json:"invoiceNo"`
	IssueDate   string `json:"issueDate"`
	Issuer      string `json:"issuer"`
	TaxCents    int64  `json:"taxCents"`
	TotalCents  int64  `json:"totalCents"`
}

type Correction struct {
	Field  string `json:"field"`
	From   string `json:"from"`
	Reason string `json:"reason"`
	To     string `json:"to"`
}

type Result struct {
	AIEnabled       bool           `json:"aiEnabled"`
	AIUsed          bool           `json:"aiUsed"`
	Corrections     []Correction   `json:"corrections"`
	Fields          InvoiceFields  `json:"fields"`
	OCRRequestID    string         `json:"ocrRequestId"`
	SuggestedFields *InvoiceFields `json:"suggestedFields,omitempty"`
	Warnings        []string       `json:"warnings"`
}

type Service struct {
	accessKeyID     string
	accessKeySecret string
	aiAPIKey        string
	aiBaseURL       string
	aiModel         string
	region          string
}

func NewService() *Service {
	region := strings.TrimSpace(os.Getenv("ALIYUN_OCR_REGION"))
	if region == "" {
		region = "cn-hangzhou"
	}
	return &Service{
		accessKeyID:     strings.TrimSpace(os.Getenv("ALIYUN_ACCESS_KEY_ID")),
		accessKeySecret: strings.TrimSpace(os.Getenv("ALIYUN_ACCESS_KEY_SECRET")),
		aiAPIKey:        strings.TrimSpace(os.Getenv("AI_CORRECTION_API_KEY")),
		aiBaseURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("AI_CORRECTION_BASE_URL")), "/"),
		aiModel:         strings.TrimSpace(os.Getenv("AI_CORRECTION_MODEL")),
		region:          region,
	}
}

func (s *Service) Enabled() bool { return s.accessKeyID != "" && s.accessKeySecret != "" }

func (s *Service) AIEnabled() bool {
	return s.aiAPIKey != "" && s.aiBaseURL != "" && s.aiModel != ""
}

func (s *Service) RecognizeInvoice(filename string, content []byte) (*Result, error) {
	if !s.Enabled() {
		return nil, errors.New("阿里云 OCR 尚未配置：请设置 ALIYUN_ACCESS_KEY_ID 和 ALIYUN_ACCESS_KEY_SECRET")
	}
	if len(content) == 0 {
		return nil, errors.New("上传文件为空")
	}
	if len(content) > maxFileSize {
		return nil, errors.New("文件不能超过 10MB")
	}
	if !allowedExtensions[strings.ToLower(fileExtension(filename))] {
		return nil, errors.New("仅支持 PNG、JPG、JPEG、BMP、GIF、TIFF、WebP 或 PDF 发票文件")
	}

	endpoint := fmt.Sprintf("ocr-api.%s.aliyuncs.com", s.region)
	client, err := ocrclient.NewClient(&openapi.Config{
		AccessKeyId:     tea.String(s.accessKeyID),
		AccessKeySecret: tea.String(s.accessKeySecret),
		Endpoint:        tea.String(endpoint),
		RegionId:        tea.String(s.region),
	})
	if err != nil {
		return nil, fmt.Errorf("初始化阿里云 OCR 客户端失败: %w", err)
	}
	response, err := client.RecognizeAllText(&ocrclient.RecognizeAllTextRequest{
		Body:   bytes.NewReader(content),
		PageNo: tea.Int32(1),
		Type:   tea.String("Invoice"),
	})
	if err != nil {
		return nil, fmt.Errorf("阿里云 OCR 调用失败: %w", err)
	}
	if response == nil || response.Body == nil {
		return nil, errors.New("阿里云 OCR 未返回识别结果")
	}
	if response.Body.Code != nil && *response.Body.Code != "" && *response.Body.Code != "200" {
		return nil, fmt.Errorf("阿里云 OCR 识别失败: %s", stringValue(response.Body.Message))
	}

	if response.Body.Data == nil || len(response.Body.Data.SubImages) == 0 {
		return nil, errors.New("阿里云统一识别未返回发票结构化字段，请确认上传的是清晰的增值税发票")
	}
	var payload struct {
		InvoiceAmountPreTax string `json:"invoiceAmountPreTax"`
		InvoiceDate         string `json:"invoiceDate"`
		InvoiceNumber       string `json:"invoiceNumber"`
		InvoiceTax          string `json:"invoiceTax"`
		PurchaserName       string `json:"purchaserName"`
		SellerName          string `json:"sellerName"`
		TotalAmount         string `json:"totalAmount"`
	}
	var parsed bool
	for _, image := range response.Body.Data.SubImages {
		if image == nil || image.KvInfo == nil || image.KvInfo.Data == nil {
			continue
		}
		if err := unmarshalUnifiedData(image.KvInfo.Data, &payload); err == nil {
			parsed = true
			break
		}
	}
	if !parsed {
		return nil, errors.New("无法解析阿里云统一识别的发票字段")
	}
	fields := InvoiceFields{
		AmountCents: parseCents(payload.InvoiceAmountPreTax),
		BuyerName:   strings.TrimSpace(payload.PurchaserName),
		InvoiceNo:   strings.TrimSpace(payload.InvoiceNumber),
		IssueDate:   normalizeDate(payload.InvoiceDate),
		Issuer:      strings.TrimSpace(payload.SellerName),
		TaxCents:    parseCents(payload.InvoiceTax),
		TotalCents:  parseCents(payload.TotalAmount),
	}
	result := &Result{AIEnabled: s.AIEnabled(), Fields: fields, OCRRequestID: stringValue(response.Body.RequestId)}
	result.Warnings = validate(fields)
	if !s.AIEnabled() {
		return result, nil
	}

	suggestion, corrections, err := s.correct(fields, result.Warnings)
	if err != nil {
		result.Warnings = append(result.Warnings, "AI 纠偏暂时不可用，已保留阿里云 OCR 原始识别结果。")
		return result, nil
	}
	result.AIUsed = true
	result.SuggestedFields = suggestion
	result.Corrections = corrections
	return result, nil
}

func unmarshalUnifiedData(value interface{}, target interface{}) error {
	if text, ok := value.(string); ok {
		return json.Unmarshal([]byte(text), target)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}

func (s *Service) correct(fields InvoiceFields, warnings []string) (*InvoiceFields, []Correction, error) {
	prompt, _ := json.Marshal(struct {
		Fields   InvoiceFields `json:"ocrFields"`
		Warnings []string      `json:"ruleWarnings"`
	}{fields, warnings})
	body, _ := json.Marshal(map[string]any{
		"model":           s.aiModel,
		"temperature":     0,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": "你是发票 OCR 纠偏助手。只能根据给定 OCR 字段和规则提示做保守修正，不能编造任何缺失信息。返回严格 JSON：{\"fields\":{\"invoiceNo\":\"\",\"issueDate\":\"YYYY-MM-DD 或空\",\"issuer\":\"\",\"buyerName\":\"\",\"amountCents\":0,\"taxCents\":0,\"totalCents\":0},\"corrections\":[{\"field\":\"\",\"from\":\"\",\"to\":\"\",\"reason\":\"\"}]}。无把握时维持原值。"},
			{"role": "user", "content": string(prompt)},
		},
	})
	request, err := http.NewRequest(http.MethodPost, s.aiBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Authorization", "Bearer "+s.aiAPIKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 25 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("AI 服务返回 HTTP %d", response.StatusCode)
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&completion); err != nil || len(completion.Choices) == 0 {
		return nil, nil, errors.New("AI 服务未返回有效纠偏结果")
	}
	var answer struct {
		Fields      InvoiceFields `json:"fields"`
		Corrections []Correction  `json:"corrections"`
	}
	if err := json.Unmarshal([]byte(stripCodeFence(completion.Choices[0].Message.Content)), &answer); err != nil {
		return nil, nil, errors.New("AI 纠偏结果不是有效 JSON")
	}
	if answer.Fields.InvoiceNo == "" {
		answer.Fields.InvoiceNo = fields.InvoiceNo
	}
	if answer.Fields.IssueDate == "" {
		answer.Fields.IssueDate = fields.IssueDate
	}
	if answer.Fields.Issuer == "" {
		answer.Fields.Issuer = fields.Issuer
	}
	if answer.Fields.BuyerName == "" {
		answer.Fields.BuyerName = fields.BuyerName
	}
	// Integer zero is also Go's value for a field omitted by an AI response.
	// An invoice amount must not be silently replaced by zero merely because the
	// model only suggested a text-field correction.
	if answer.Fields.AmountCents == 0 && fields.AmountCents > 0 {
		answer.Fields.AmountCents = fields.AmountCents
	}
	if answer.Fields.TaxCents == 0 && fields.TaxCents > 0 {
		answer.Fields.TaxCents = fields.TaxCents
	}
	if answer.Fields.TotalCents == 0 && fields.TotalCents > 0 {
		answer.Fields.TotalCents = fields.TotalCents
	}
	if answer.Fields.AmountCents < 0 || answer.Fields.TaxCents < 0 || answer.Fields.TotalCents < 0 {
		return nil, nil, errors.New("AI 返回了无效金额")
	}
	answer.Fields.IssueDate = normalizeDate(answer.Fields.IssueDate)
	return &answer.Fields, answer.Corrections, nil
}

func validate(fields InvoiceFields) []string {
	var warnings []string
	if fields.InvoiceNo == "" {
		warnings = append(warnings, "未识别到发票号码，请人工填写。")
	}
	if fields.IssueDate == "" {
		warnings = append(warnings, "未识别到有效开票日期，请人工确认。")
	}
	if fields.Issuer == "" {
		warnings = append(warnings, "未识别到销售方名称，请人工确认。")
	}
	if fields.TotalCents > 0 && fields.AmountCents+fields.TaxCents != fields.TotalCents {
		warnings = append(warnings, "金额、税额与价税合计不一致，请核对原始发票。")
	}
	if fields.TotalCents == 0 && fields.AmountCents == 0 {
		warnings = append(warnings, "未识别到有效金额，请人工填写。")
	}
	return warnings
}

func parseCents(value string) int64 {
	clean := regexp.MustCompile(`[^0-9.-]`).ReplaceAllString(strings.TrimSpace(value), "")
	if clean == "" {
		return 0
	}
	amount, err := strconv.ParseFloat(clean, 64)
	if err != nil || amount < 0 {
		return 0
	}
	return int64(amount*100 + 0.5)
}

func normalizeDate(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("年", "-", "月", "-", "日", "", "/", "-", ".", "-").Replace(value)
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, "")
	parts := strings.Split(value, "-")
	if len(parts) != 3 {
		return ""
	}
	y, errY := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	d, errD := strconv.Atoi(parts[2])
	if errY != nil || errM != nil || errD != nil || y < 2000 || m < 1 || m > 12 || d < 1 || d > 31 {
		return ""
	}
	normalized := fmt.Sprintf("%04d-%02d-%02d", y, m, d)
	if _, err := time.Parse("2006-01-02", normalized); err != nil {
		return ""
	}
	return normalized
}

func fileExtension(filename string) string {
	index := strings.LastIndex(filename, ".")
	if index < 0 {
		return ""
	}
	return filename[index:]
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func stripCodeFence(value string) string {
	return strings.Trim(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(value), "```json"), "```")), "\n")
}
